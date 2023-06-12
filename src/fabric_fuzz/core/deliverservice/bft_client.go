/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/spf13/viper"
)

var bftLogger = flogging.MustGetLogger("bftDeliveryClient")

const (
	bftMinBackoffDelay           = 10 * time.Millisecond
	bftMaxBackoffDelay           = 10 * time.Second
	bftBlockRcvTotalBackoffDelay = 20 * time.Second
	bftBlockCensorshipTimeout    = 20 * time.Second
)

var (
	errNoBlockReceiver        = errors.New("no block receiver")
	errDuplicateBlock         = errors.New("duplicate block")
	errOutOfOrderBlock        = errors.New("out-of-order block")
	errClientClosing          = errors.New("client is closing")
	errClientReconnectTimeout = errors.New("client reconnect timeout")
)

//go:generate mockery -dir . -name MessageCryptoVerifier -case underscore -output ./mocks/

// MessageCryptoVerifier defines message verification methods, and is subset of gossip/api/MessageCryptoService.
type MessageCryptoVerifier interface {
	// VerifyHeader returns nil when the header matches the metadata signature, but it does not compute the
	// block.Data.Hash() and compare it to the block.Header.DataHash, or otherwise inspect the block.Data.
	// This is used when the orderer delivers a block with header & metadata only (i.e. block.Data==nil).
	// See: gossip/api/MessageCryptoService
	VerifyHeader(chainID string, signedBlock *common.Block) error
}

type bftDeliveryClient struct {
	mutex     sync.Mutex
	startOnce sync.Once
	stopFlag  bool
	stopChan  chan struct{}

	chainID            string                    // a.k.a. Channel ID
	connectionFactory  comm.ConnectionFactory    // creates a gRPC connection
	createClient       clientFactory             // creates orderer.AtomicBroadcastClient
	ledgerInfoProvider blocksprovider.LedgerInfo // provides access to the ledger height
	msgCryptoVerifier  MessageCryptoVerifier     // verifies headers

	// The total time a bft client tries to connect (to all available endpoints) before giving up leadership
	reconnectTotalTimeThreshold time.Duration
	// the total time a block receiver tries to connect before giving up and letting the client to try another
	// endpoint as a block receiver
	reconnectBlockRcvTotalTimeThreshold time.Duration
	// The minimal time between connection retries. This interval grows exponentially until maxBackoffDelay, below.
	minBackoffDelay time.Duration
	// The maximal time between connection retries.
	maxBackoffDelay time.Duration
	// The block censorship timeout. A block censorship suspicion is declared if more than f header receivers are
	// ahead of the block receiver for a period larger than this timeout.
	blockCensorshipTimeout time.Duration

	endpoints          []comm.EndpointCriteria // a set of endpoints
	blockReceiverIndex int                     // index of the current block receiver endpoint

	blockReceiver   *broadcastClient
	nextBlockNumber uint64
	lastBlockTime   time.Time

	headerReceivers map[string]*bftHeaderReceiver
}

func NewBFTDeliveryClient(
	chainID string,
	connFactory comm.ConnectionFactory,
	endpoints []comm.EndpointCriteria,
	clFactory clientFactory,
	ledgerInfoProvider blocksprovider.LedgerInfo,
	msgVerifier MessageCryptoVerifier,
) *bftDeliveryClient {
	c := &bftDeliveryClient{
		stopChan:                            make(chan struct{}, 1),
		chainID:                             chainID,
		connectionFactory:                   connFactory,
		createClient:                        clFactory,
		ledgerInfoProvider:                  ledgerInfoProvider,
		msgCryptoVerifier:                   msgVerifier,
		reconnectTotalTimeThreshold:         getReConnectTotalTimeThreshold(),
		reconnectBlockRcvTotalTimeThreshold: util.GetDurationOrDefault("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", bftBlockRcvTotalBackoffDelay),
		minBackoffDelay:                     util.GetDurationOrDefault("peer.deliveryclient.bft.minBackoffDelay", bftMinBackoffDelay),
		maxBackoffDelay:                     util.GetDurationOrDefault("peer.deliveryclient.bft.maxBackoffDelay", bftMaxBackoffDelay),
		blockCensorshipTimeout:              util.GetDurationOrDefault("peer.deliveryclient.bft.blockCensorshipTimeout", bftBlockCensorshipTimeout),
		endpoints:                           comm.Shuffle(endpoints),
		blockReceiverIndex:                  -1,
		headerReceivers:                     make(map[string]*bftHeaderReceiver),
	}

	bftLogger.Infof("[%s] Created BFT Delivery Client", chainID)
	return c
}

func (c *bftDeliveryClient) Recv() (response *orderer.DeliverResponse, err error) {
	bftLogger.Debugf("[%s] Entry", c.chainID)

	c.startOnce.Do(func() {
		var num uint64
		num, err = c.ledgerInfoProvider.LedgerHeight()
		if err != nil {
			return
		}
		c.nextBlockNumber = num
		c.lastBlockTime = time.Now()
		bftLogger.Debugf("[%s] Starting monitor routine; Initial ledger height: %d", c.chainID, num)
		go c.monitor()
	})
	// can only happen once, after first invocation
	if err != nil {
		bftLogger.Debugf("[%s] Exit: error=%v", c.chainID, err)
		return nil, errors.New("cannot access ledger height")
	}

	var numEP int
	var numRetries int
	var stopRetries = time.Now().Add(c.reconnectTotalTimeThreshold)

	for !c.shouldStop() {
		if numEP, err = c.assignReceivers(); err != nil {
			bftLogger.Debugf("[%s] Exit: error=%s", c.chainID, err)
			return nil, err
		}

		c.launchHeaderReceivers()

		response, err = c.receiveBlock()
		if err == nil {
			bftLogger.Debugf("[%s] Exit: response=%v", c.chainID, response)
			return response, nil // the normal return path
		}

		if stopRetries.Before(time.Now()) {
			bftLogger.Debugf("[%s] Exit: reconnectTotalTimeThreshold: %s, expired; error: %s",
				c.chainID, c.reconnectTotalTimeThreshold, errClientReconnectTimeout)
			return nil, errClientReconnectTimeout
		}

		c.closeBlockReceiver(false)
		numRetries++
		if numRetries%numEP == 0 { //double the back-off delay on every round of attempts.
			dur := backOffDuration(2.0, uint(numRetries/numEP), c.minBackoffDelay, c.maxBackoffDelay)
			bftLogger.Debugf("[%s] Got receive error: %s; going to retry another endpoint in: %s", c.chainID, err, dur)
			backOffSleep(dur, c.stopChan)
		} else {
			bftLogger.Debugf("[%s] Got receive error: %s; going to retry another endpoint now", c.chainID, err)
		}
	}

	bftLogger.Debugf("[%s] Exit: %s", c.chainID, errClientClosing.Error())
	return nil, errClientClosing
}

func backOffDuration(base float64, exponent uint, minDur, maxDur time.Duration) time.Duration {
	if base < 1.0 {
		base = 1.0
	}
	if minDur <= 0 {
		minDur = bftMinBackoffDelay
	}
	if maxDur < minDur {
		maxDur = minDur
	}

	fDurNano := math.Pow(base, float64(exponent)) * float64(minDur.Nanoseconds())
	fDurNano = math.Min(fDurNano, float64(maxDur.Nanoseconds()))
	return time.Duration(fDurNano)
}

func backOffSleep(backOffDur time.Duration, stopChan <-chan struct{}) {
	select {
	case <-time.After(backOffDur):
	case <-stopChan:
	}
}

// Check block reception progress relative to header reception progress.
// If the orderer associated with the block receiver is suspected of censorship, replace it with another orderer.
func (c *bftDeliveryClient) monitor() {
	bftLogger.Debugf("[%s] Entry", c.chainID)

	ticker := time.NewTicker(bftBlockCensorshipTimeout / 100)
	for !c.shouldStop() {
		if suspicion := c.detectBlockCensorship(); suspicion {
			c.closeBlockReceiver(true)
		}

		select {
		case <-ticker.C:
		case <-c.stopChan:
		}
	}
	ticker.Stop()

	bftLogger.Debugf("[%s] Exit", c.chainID)
}

func (c *bftDeliveryClient) detectBlockCensorship() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.blockReceiver == nil {
		return false
	}

	now := time.Now()
	suspicionThreshold := c.lastBlockTime.Add(c.blockCensorshipTimeout)
	if now.Before(suspicionThreshold) {
		return false
	}

	var numAhead int
	for ep, hRcv := range c.headerReceivers {
		blockNum, _, err := hRcv.LastBlockNum()
		if err != nil {
			continue
		}
		if blockNum >= c.nextBlockNumber {
			bftLogger.Debugf("[%s] header receiver: %s, is ahead of block receiver, headr-rcv=%d, block-rcv=%d", c.chainID, ep, blockNum, c.nextBlockNumber-1)
			numAhead++
		}
	}

	numEP := uint64(len(c.endpoints))
	_, f := computeQuorum(numEP)
	if numAhead > f {
		bftLogger.Warnf("[%s] suspected block censorship: %d header receivers are ahead of block receiver, out of %d endpoints",
			c.chainID, numAhead, numEP)
		return true
	}

	return false
}

func computeQuorum(N uint64) (Q int, F int) {
	F = int((int(N) - 1) / 3)
	Q = int(math.Ceil((float64(N) + float64(F) + 1) / 2.0))
	return
}

// (re)-assign a block delivery client and header delivery clients
func (c *bftDeliveryClient) assignReceivers() (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	numEP := len(c.endpoints)
	if numEP <= 0 {
		return numEP, errors.New("no endpoints")
	}

	if c.blockReceiver == nil {
		c.blockReceiverIndex = (c.blockReceiverIndex + 1) % numEP
		ep := c.endpoints[c.blockReceiverIndex]
		if headerReceiver, exists := c.headerReceivers[ep.Endpoint]; exists {
			headerReceiver.Close()
			delete(c.headerReceivers, ep.Endpoint)
			bftLogger.Debugf("[%s] Closed header receiver to: %s", c.chainID, ep.Endpoint)
		}
		c.blockReceiver = c.newBlockClient(ep)
		bftLogger.Debugf("[%s] Created block receiver to: %s", c.chainID, ep.Endpoint)
	}

	hRcvToCreate := make([]comm.EndpointCriteria, 0)
	for i, ep := range c.endpoints {
		if i == c.blockReceiverIndex {
			continue
		}

		if hRcv, exists := c.headerReceivers[ep.Endpoint]; exists {
			if hRcv.isStopped() {
				delete(c.headerReceivers, ep.Endpoint)
			} else {
				continue
			}
		}

		hRcvToCreate = append(hRcvToCreate, ep)
	}

	for _, ep := range hRcvToCreate {
		headerReceiver := newBFTHeaderReceiver(c.chainID, ep.Endpoint, c.newHeaderClient(ep), c.msgCryptoVerifier,
			c.minBackoffDelay, c.maxBackoffDelay)
		c.headerReceivers[ep.Endpoint] = headerReceiver
		bftLogger.Debugf("[%s] Created header receiver to: %s", c.chainID, ep.Endpoint)
	}

	bftLogger.Debugf("Exit: number of endpoints: %d", numEP)
	return numEP, nil
}

func (c *bftDeliveryClient) launchHeaderReceivers() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var launched int
	for ep, hRcv := range c.headerReceivers {
		if !hRcv.isStarted() {
			bftLogger.Debugf("[%s] launching a header receiver to endpoint: %s", c.chainID, ep)
			launched++
			go hRcv.DeliverHeaders()
		}
	}

	bftLogger.Debugf("[%s] header receivers: launched=%d, total running=%d ", c.chainID, launched, len(c.headerReceivers))
}

func (c *bftDeliveryClient) receiveBlock() (*orderer.DeliverResponse, error) {
	c.mutex.Lock()
	receiver := c.blockReceiver
	nextBlockNumber := c.nextBlockNumber
	c.mutex.Unlock()

	// call Recv() without a lock
	if receiver == nil {
		return nil, errNoBlockReceiver
	}
	response, err := receiver.Recv()

	if err != nil {
		return response, err
	}
	// ignore older blocks, filter out-of-order blocks
	switch t := response.Type.(type) {
	case *orderer.DeliverResponse_Block:
		if t.Block.Header.Number > nextBlockNumber {
			bftLogger.Warnf("[%s] Ignoring out-of-order block from orderer: %s; received block number: %d, expected: %d",
				c.chainID, receiver.GetEndpoint(), t.Block.Header.Number, nextBlockNumber)
			return nil, errOutOfOrderBlock
		}
		if t.Block.Header.Number < nextBlockNumber {
			bftLogger.Warnf("[%s] Ignoring duplicate block from orderer: %s; received block number: %d, expected: %d",
				c.chainID, receiver.GetEndpoint(), t.Block.Header.Number, nextBlockNumber)
			return nil, errDuplicateBlock
		}
	}

	return response, err
}

func (c *bftDeliveryClient) closeBlockReceiver(updateLastBlockTime bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if updateLastBlockTime {
		c.lastBlockTime = time.Now()
	}

	if c.blockReceiver != nil {
		c.blockReceiver.Close()
		c.blockReceiver = nil
	}
}

func (c *bftDeliveryClient) LedgerHeight() (uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.nextBlockNumber, nil
}

// create a broadcastClient that delivers blocks
func (c *bftDeliveryClient) newBlockClient(endpoint comm.EndpointCriteria) *broadcastClient {
	requester := &blocksRequester{
		tls:     viper.GetBool("peer.tls.enabled"),
		chainID: c.chainID,
	}

	//Update the nextBlockNumber from the ledger, and then make sure the client request the nextBlockNumber,
	// because that is what we expect in the Recv() loop.
	if height, err := c.ledgerInfoProvider.LedgerHeight(); err == nil {
		c.nextBlockNumber = height
	}
	broadcastSetup := func(bd blocksprovider.BlocksDeliverer) error {
		return requester.RequestBlocks(c) // Do not ask the ledger directly, ask the bftDeliveryClient
	}

	backoffPolicy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		//Let block receivers give-up early so we can replace them with a header receiver
		if elapsedTime >= c.reconnectBlockRcvTotalTimeThreshold {
			return 0, false
		}
		return backOffDuration(2.0, uint(attemptNum), c.minBackoffDelay, c.maxBackoffDelay), true
	}
	//Only a single endpoint
	connProd := comm.NewConnectionProducer(c.connectionFactory, []comm.EndpointCriteria{endpoint})
	blockClient := NewBroadcastClient(connProd, c.createClient, broadcastSetup, backoffPolicy)
	requester.client = blockClient
	return blockClient
}

// create a broadcastClient that delivers headers
func (c *bftDeliveryClient) newHeaderClient(endpoint comm.EndpointCriteria) *broadcastClient {
	requester := &blocksRequester{
		tls:     viper.GetBool("peer.tls.enabled"),
		chainID: c.chainID,
	}
	broadcastSetup := func(bd blocksprovider.BlocksDeliverer) error {
		return requester.RequestHeaders(c.ledgerInfoProvider)
	}
	backoffPolicy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		if elapsedTime >= c.reconnectTotalTimeThreshold { // Let header receivers continue to try until we close them
			return 0, false
		}
		return backOffDuration(2.0, uint(attemptNum), c.minBackoffDelay, c.maxBackoffDelay), true
	}
	//Only a single endpoint
	connProd := comm.NewConnectionProducer(c.connectionFactory, []comm.EndpointCriteria{endpoint})
	headerClient := NewBroadcastClient(connProd, c.createClient, broadcastSetup, backoffPolicy)
	requester.client = headerClient
	return headerClient
}

func (c *bftDeliveryClient) Send(*common.Envelope) error {
	return errors.New("should never be called")
}

func (c *bftDeliveryClient) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stopFlag {
		return
	}

	c.stopFlag = true
	close(c.stopChan)

	c.disconnectAll()

	bftLogger.Debugf("Exit")
}

func (c *bftDeliveryClient) disconnectAll() {
	if c.blockReceiver != nil {
		ep := c.blockReceiver.GetEndpoint()
		c.blockReceiver.Close()
		bftLogger.Debugf("[%s] closed block receiver to: %s", c.chainID, ep)
		c.blockReceiver = nil
	}

	for ep, hRcv := range c.headerReceivers {
		hRcv.Close()
		bftLogger.Debugf("[%s] closed header receiver to: %s", c.chainID, ep)
		delete(c.headerReceivers, ep)
	}
}

// Disconnect just the block receiver client, so that the next Recv() will choose a new one.
func (c *bftDeliveryClient) Disconnect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.blockReceiver != nil {
		c.blockReceiver.Disconnect()
		c.blockReceiver = nil
	}
}

// UpdateReceived allows the client to track the reception of valid blocks.
// This is needed because blocks are verified by the blockprovider, not here.
func (c *bftDeliveryClient) UpdateReceived(blockNumber uint64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	bftLogger.Debugf("[%s] received blockNumber=%d", c.chainID, blockNumber)
	c.nextBlockNumber = blockNumber + 1
	c.lastBlockTime = time.Now()
}

func (c *bftDeliveryClient) UpdateEndpoints(endpoints []comm.EndpointCriteria) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stopFlag {
		return
	}

	if equalEndpoints(c.endpoints, endpoints) {
		return
	}

	bftLogger.Debugf("[%s] updating endpoints: existing: %s, new: %s", c.chainID, c.endpoints, endpoints)
	c.endpoints = endpoints
	c.blockReceiverIndex = 0
	c.disconnectAll()
}

func (c *bftDeliveryClient) shouldStop() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.stopFlag
}

// GetEndpoint provides the endpoint of the ordering service server that delivers
// blocks (as opposed to headers) to this delivery client.
func (c *bftDeliveryClient) GetEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.blockReceiver == nil {
		return ""
	}

	return c.blockReceiver.GetEndpoint()
}

func (c *bftDeliveryClient) GetNextBlockNumTime() (uint64, time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.nextBlockNumber, c.lastBlockTime
}

func (c *bftDeliveryClient) GetHeadersBlockNumTime() ([]uint64, []time.Time, []error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	hNum := make([]uint64, 0, len(c.headerReceivers))
	hTime := make([]time.Time, 0, len(c.headerReceivers))
	hErr := make([]error, 0, len(c.headerReceivers))
	for _, hRcv := range c.headerReceivers {
		num, t, err := hRcv.LastBlockNum()
		hNum = append(hNum, num)
		hTime = append(hTime, t)
		hErr = append(hErr, err)
	}
	return hNum, hTime, hErr
}
