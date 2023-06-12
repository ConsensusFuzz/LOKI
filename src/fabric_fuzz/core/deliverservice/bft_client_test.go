/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

var endpoints = []comm.EndpointCriteria{
	{Endpoint: "localhost:5611", Organizations: []string{"org1"}},
	{Endpoint: "localhost:5612", Organizations: []string{"org2"}},
	{Endpoint: "localhost:5613", Organizations: []string{"org3"}},
	{Endpoint: "localhost:5614", Organizations: []string{"org4"}},
}

var endpoints3 = []comm.EndpointCriteria{
	{Endpoint: "localhost:5615", Organizations: []string{"org1"}},
	{Endpoint: "localhost:5616", Organizations: []string{"org2"}},
	{Endpoint: "localhost:5617", Organizations: []string{"org3"}},
}

var endpointMap = map[int]comm.EndpointCriteria{
	5611: {Endpoint: "localhost:5611", Organizations: []string{"org1"}},
	5612: {Endpoint: "localhost:5612", Organizations: []string{"org2"}},
	5613: {Endpoint: "localhost:5613", Organizations: []string{"org3"}},
	5614: {Endpoint: "localhost:5614", Organizations: []string{"org4"}},
}

var endpointMap3 = map[int]comm.EndpointCriteria{
	5615: {Endpoint: "localhost:5615", Organizations: []string{"org1"}},
	5616: {Endpoint: "localhost:5616", Organizations: []string{"org2"}},
	5617: {Endpoint: "localhost:5617", Organizations: []string{"org3"}},
}

// Scenario: create a client and close it.
func TestBFTDeliverClient_New(t *testing.T) {
	flogging.ActivateSpec("bftDeliveryClient=DEBUG")

	connFactory := func(endpoint comm.EndpointCriteria) (*grpc.ClientConn, error) {
		return &grpc.ClientConn{}, nil
	}

	abcClient := &abclient{}
	clFactory := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
		return abcClient
	}

	ledgerInfoMock := &mocks.LedgerInfo{}
	bc := NewBFTDeliveryClient("test-chain", connFactory, endpoints, clFactory, ledgerInfoMock, &mockMCS{})
	defer bc.Close()
}

// Scenario: create a client against a set of orderer mocks. Receive several blocks and check block & header reception.
func TestBFTDeliverClient_Recv(t *testing.T) {
	flogging.ActivateSpec("bftDeliveryClient=DEBUG")

	defer ensureNoGoroutineLeak(t)()

	osArray := make([]*mocks.Orderer, 0)
	for port := range endpointMap {
		osArray = append(osArray, mocks.NewOrderer(port, t))
	}
	for _, os := range osArray {
		os.SetNextExpectedSeek(5)
	}

	connFactory := func(endpoint comm.EndpointCriteria) (*grpc.ClientConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), getConnectionTimeout())
		defer cancel()
		return grpc.DialContext(ctx, endpoint.Endpoint, grpc.WithInsecure(), grpc.WithBlock())
		//return grpc.Dial(endpoint.Endpoint, grpc.WithInsecure(), grpc.WithBlock())
	}
	ledgerInfoMock := &mocks.LedgerInfo{}
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	bc := NewBFTDeliveryClient("test-chain", connFactory, endpoints, DefaultABCFactory, ledgerInfoMock, msgVerifierMock)
	ledgerInfoMock.On("LedgerHeight").Return(uint64(5), nil)

	go func() {
		for {
			resp, err := bc.Recv()
			if err != nil {
				assert.EqualError(t, err, errClientClosing.Error())
				return
			}
			block := resp.GetBlock()
			assert.NotNil(t, block)
			if block == nil {
				return
			}
			bc.UpdateReceived(block.Header.Number)
		}
	}()

	// all orderers send something: block/header
	beforeSend := time.Now()
	for seq := uint64(5); seq < uint64(10); seq++ {
		for _, os := range osArray {
			os.SendBlock(seq)
		}
	}

	time.Sleep(time.Second)
	bc.Close()

	lastN, lastT := bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(10), lastN)
	assert.True(t, lastT.After(beforeSend))

	headersNum, headerT, headerErr := bc.GetHeadersBlockNumTime()
	for i, n := range headersNum {
		assert.Equal(t, uint64(10), n)
		assert.True(t, headerT[i].After(beforeSend))
		assert.NoError(t, headerErr[i])
	}

	for _, os := range osArray {
		os.Shutdown()
	}
}

// Scenario: block censorship by orderer. Create a client against a set of orderer mocks.
// Receive one block. Then, the orderer sending blocks stops sending but headers keep coming.
// The client should switch to another orderer and seek from the new height. Check block & header reception.
func TestBFTDeliverClient_Censorship(t *testing.T) {
	flogging.ActivateSpec("bftDeliveryClient,deliveryClient=DEBUG")
	viper.Set("peer.deliveryclient.bft.blockCensorshipTimeout", 2*time.Second)
	defer viper.Reset()
	defer ensureNoGoroutineLeak(t)()

	assert.Equal(t, util.GetDurationOrDefault("peer.deliveryclient.bft.blockCensorshipTimeout", bftBlockCensorshipTimeout), 2*time.Second)

	osMap := make(map[string]*mocks.Orderer, len(endpointMap))
	for port, ep := range endpointMap {
		osMap[ep.Endpoint] = mocks.NewOrderer(port, t)
	}
	for _, os := range osMap {
		os.SetNextExpectedSeek(5)
	}

	connFactory := func(endpoint comm.EndpointCriteria) (*grpc.ClientConn, error) {
		return grpc.Dial(endpoint.Endpoint, grpc.WithInsecure(), grpc.WithBlock())
	}
	var ledgerHeight uint64 = 5
	ledgerInfoMock := &mocks.LedgerInfo{}
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	bc := NewBFTDeliveryClient("test-chain", connFactory, endpoints, DefaultABCFactory, ledgerInfoMock, msgVerifierMock)
	ledgerInfoMock.On("LedgerHeight").Return(func() uint64 { return atomic.LoadUint64(&ledgerHeight) }, nil)
	msgVerifierMock.On("VerifyHeader", mock.AnythingOfType("string"), mock.AnythingOfType("*common.Block")).Return(nil)

	go func() {
		for {
			resp, err := bc.Recv()
			if err != nil {
				assert.EqualError(t, err, "client is closing")
				return
			}
			block := resp.GetBlock()
			assert.NotNil(t, block)
			if block == nil {
				return
			}
			atomic.StoreUint64(&ledgerHeight, block.Header.Number+1)
			bc.UpdateReceived(block.Header.Number)
		}
	}()

	blockEP, err := waitForBlockEP(bc)
	assert.NoError(t, err)
	osnMocks, err := detectOSNConnections(true, osnMapValues(osMap)...)
	assert.NoError(t, err)
	assert.Equal(t, strings.Split(blockEP, ":")[1], strings.Split(osnMocks[0].Addr().String(), ":")[1])

	// one normal block
	beforeSend := time.Now()
	for _, os := range osMap {
		os.SendBlock(5)
	}
	for _, os := range osMap {
		os.SetNextExpectedSeek(6)
	}
	time.Sleep(time.Second)
	lastN, lastT := bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(6), lastN)
	assert.True(t, lastT.After(beforeSend))
	verifyHeaderReceivers(t, bc, 3, 5, beforeSend, 0)

	// only headers
	beforeSend = time.Now()
	for seq := uint64(6); seq < uint64(10); seq++ {
		for ep, os := range osMap {
			if ep == blockEP { //censorship
				continue
			}
			os.SendBlock(seq)
		}
	}

	time.Sleep(3 * time.Second)

	// the client detected the censorship and switched
	blockEP2, err := waitForBlockEP(bc)
	assert.NoError(t, err)
	assert.True(t, blockEP != blockEP2)

	for seq := uint64(6); seq < uint64(10); seq++ {
		for ep, os := range osMap {
			if ep == blockEP2 {
				os.SendBlock(seq)
			}
		}
	}

	time.Sleep(1 * time.Second)

	lastN, lastT = bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(10), lastN)
	assert.True(t, lastT.After(beforeSend))
	verifyHeaderReceivers(t, bc, 3, 9, beforeSend, 1)

	bc.Close()

	for _, os := range osMap {
		os.Shutdown()
	}
}

// Scenario: server fail-over. Create a client against a set of orderer mocks.
// Receive one block. Then, the orderer sending blocks fails.
// The client should switch to another orderer and seek from the new height. Send a few blocks.
// Check block & header reception.
func TestBFTDeliverClient_Failover(t *testing.T) {
	flogging.ActivateSpec("bftDeliveryClient,deliveryClient=DEBUG")
	viper.Set("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", time.Second)
	viper.Set("peer.deliveryclient.bft.maxBackoffDelay", time.Second)
	viper.Set("peer.deliveryclient.connTimeout", 100*time.Millisecond)
	defer viper.Reset()
	defer ensureNoGoroutineLeak(t)()

	osMap := make(map[string]*mocks.Orderer, len(endpointMap))
	for port, ep := range endpointMap {
		osMap[ep.Endpoint] = mocks.NewOrderer(port, t)
	}
	for _, os := range osMap {
		os.SetNextExpectedSeek(5)
	}

	connFactory := func(endpoint comm.EndpointCriteria) (*grpc.ClientConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), getConnectionTimeout())
		defer cancel()
		return grpc.DialContext(ctx, endpoint.Endpoint, grpc.WithInsecure(), grpc.WithBlock())
	}
	var ledgerHeight uint64 = 5
	ledgerInfoMock := &mocks.LedgerInfo{}
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	bc := NewBFTDeliveryClient("test-chain", connFactory, endpoints, DefaultABCFactory, ledgerInfoMock, msgVerifierMock)
	ledgerInfoMock.On("LedgerHeight").Return(func() uint64 { return atomic.LoadUint64(&ledgerHeight) }, nil)
	msgVerifierMock.On("VerifyHeader", mock.AnythingOfType("string"), mock.AnythingOfType("*common.Block")).Return(nil)

	go func() {
		for {
			resp, err := bc.Recv()
			if err != nil {
				assert.EqualError(t, err, "client is closing")
				return
			}
			block := resp.GetBlock()
			assert.NotNil(t, block)
			if block == nil {
				return
			}
			atomic.StoreUint64(&ledgerHeight, block.Header.Number+1)
			bc.UpdateReceived(block.Header.Number)
		}
	}()

	time.Sleep(time.Second)
	blockEP, err := waitForBlockEP(bc)
	assert.NoError(t, err)

	osnMocks, err := detectOSNConnections(true, osnMapValues(osMap)...)
	assert.NoError(t, err)
	assert.Equal(t, strings.Split(blockEP, ":")[1], strings.Split(osnMocks[0].Addr().String(), ":")[1])

	// one normal block
	for _, os := range osMap {
		os.SendBlock(5)
	}
	for _, os := range osMap {
		os.SetNextExpectedSeek(6)
	}
	time.Sleep(time.Second)

	for ep, os := range osMap {
		if ep == blockEP {
			os.Shutdown()
			bftLogger.Infof("TEST: shutting down: %s", ep)
		}
	}

	time.Sleep(10 * time.Second)

	// the client detected the failure and switched
	blockEP2, err := waitForBlockEP(bc)
	assert.NoError(t, err)
	assert.True(t, blockEP != blockEP2)

	beforeSend := time.Now()
	for seq := uint64(6); seq < uint64(10); seq++ {
		for ep, os := range osMap {
			if ep == blockEP { // it is down
				continue
			}
			os.SendBlock(seq)
		}
	}

	time.Sleep(2 * time.Second)

	lastN, lastT := bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(10), lastN)
	assert.True(t, lastT.After(beforeSend))
	verifyHeaderReceivers(t, bc, 3, 9, beforeSend, 1)

	//restart the orderer
	for port, ep := range endpointMap {
		if ep.Endpoint == blockEP {
			os := mocks.NewOrderer(port, t)
			os.SetNextExpectedSeek(10)
			osMap[ep.Endpoint] = os
			bftLogger.Infof("TEST: restarting: %s", ep)
		}
	}

	time.Sleep(2 * time.Second)
	beforeSend = time.Now()
	for _, os := range osMap {
		os.SendBlock(10)
	}
	time.Sleep(1 * time.Second)

	lastN, lastT = bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(11), lastN)
	assert.True(t, lastT.After(beforeSend))
	verifyHeaderReceivers(t, bc, 3, 10, beforeSend, 0)

	bc.Close()

	for _, os := range osMap {
		os.Shutdown()
	}
}

// Scenario: update endpoints. Create a client against a set of 4 orderer mocks.
// Receive one block. Then, increase the set to 7 orderers.
// The client should disconnect and re-connect to new set.
// Send a few blocks. Check block & header reception.
func TestBFTDeliverClient_UpdateEndpoints(t *testing.T) {
	flogging.ActivateSpec("bftDeliveryClient,deliveryClient=DEBUG")
	viper.Set("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", time.Second)
	viper.Set("peer.deliveryclient.bft.maxBackoffDelay", time.Second)
	viper.Set("peer.deliveryclient.connTimeout", 100*time.Millisecond)
	defer viper.Reset()
	defer ensureNoGoroutineLeak(t)()

	osMap := make(map[string]*mocks.Orderer, len(endpointMap))
	for port, ep := range endpointMap {
		osMap[ep.Endpoint] = mocks.NewOrderer(port, t)
	}
	for _, os := range osMap {
		os.SetNextExpectedSeek(5)
	}

	connFactory := func(endpoint comm.EndpointCriteria) (*grpc.ClientConn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), getConnectionTimeout())
		defer cancel()
		return grpc.DialContext(ctx, endpoint.Endpoint, grpc.WithInsecure(), grpc.WithBlock())
	}
	var ledgerHeight uint64 = 5
	ledgerInfoMock := &mocks.LedgerInfo{}
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	bc := NewBFTDeliveryClient("test-chain", connFactory, endpoints, DefaultABCFactory, ledgerInfoMock, msgVerifierMock)
	ledgerInfoMock.On("LedgerHeight").Return(func() uint64 { return atomic.LoadUint64(&ledgerHeight) }, nil)
	msgVerifierMock.On("VerifyHeader", mock.AnythingOfType("string"), mock.AnythingOfType("*common.Block")).Return(nil)

	go func() {
		for {
			resp, err := bc.Recv()
			if err != nil {
				assert.EqualError(t, err, "client is closing")
				return
			}
			block := resp.GetBlock()
			assert.NotNil(t, block)
			if block == nil {
				return
			}
			atomic.StoreUint64(&ledgerHeight, block.Header.Number+1)
			bc.UpdateReceived(block.Header.Number)
		}
	}()

	blockEP, err := waitForBlockEP(bc)
	assert.NoError(t, err)
	osnMocks, err := detectOSNConnections(true, osnMapValues(osMap)...)
	assert.NoError(t, err)
	assert.Equal(t, strings.Split(blockEP, ":")[1], strings.Split(osnMocks[0].Addr().String(), ":")[1])

	// one normal block
	beforeSend := time.Now()
	for _, os := range osMap {
		os.SendBlock(5)
	}
	for _, os := range osMap {
		os.SetNextExpectedSeek(6)
	}
	time.Sleep(time.Second)
	lastN, lastT := bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(6), lastN)
	assert.True(t, lastT.After(beforeSend))
	verifyHeaderReceivers(t, bc, 3, 5, beforeSend, 0)

	// start 3 additional orderers
	for port, ep := range endpointMap3 {
		os := mocks.NewOrderer(port, t)
		os.SetNextExpectedSeek(6)
		osMap[ep.Endpoint] = os
	}

	time.Sleep(time.Second)

	endpoints7 := append(make([]comm.EndpointCriteria, 0), endpoints...)
	endpoints7 = append(endpoints7, endpoints3...)
	bc.UpdateEndpoints(endpoints7)
	time.Sleep(time.Second)

	_, err = waitForBlockEP(bc)
	assert.NoError(t, err)

	beforeSend = time.Now()
	for seq := uint64(6); seq < uint64(10); seq++ {
		for _, os := range osMap {
			os.SendBlock(seq)
		}
	}

	time.Sleep(2 * time.Second)

	lastN, lastT = bc.GetNextBlockNumTime()
	assert.Equal(t, uint64(10), lastN)
	assert.True(t, lastT.After(beforeSend))
	verifyHeaderReceivers(t, bc, 6, 9, beforeSend, 0)

	bc.Close()

	for _, os := range osMap {
		os.Shutdown()
	}
}

func TestBFTDeliverClient_BackOffDuration(t *testing.T) {
	minDur := 50 * time.Millisecond
	maxDur := 10 * time.Second
	for exp := uint(0); exp < 100000000; exp = (exp + 1) * 2 {
		d := backOffDuration(2.0, 0, minDur, maxDur)
		assert.True(t, d >= minDur)
		assert.True(t, d <= maxDur)
	}

	maxDur = minDur / 2
	for exp := uint(0); exp < 100000000; exp = (exp + 1) * 2 {
		d := backOffDuration(2.0, 0, minDur, maxDur)
		assert.True(t, d >= minDur)
		assert.True(t, d <= minDur)
	}

	minDur = 0
	maxDur = 10 * time.Second
	for exp := uint(0); exp < 100000000; exp = (exp + 1) * 2 {
		d := backOffDuration(2.0, 0, minDur, maxDur)
		assert.True(t, d >= bftMinBackoffDelay)
		assert.True(t, d <= maxDur)
	}
}

func verifyHeaderReceivers(t *testing.T, client *bftDeliveryClient, expectedNumHdr int, expectedBlockNum uint64, expectedBlockTime time.Time, expectedNumErr int) {
	lastHdrN, lastHdrT, lastHdrErr := client.GetHeadersBlockNumTime()
	numErr := 0
	assert.Len(t, lastHdrN, expectedNumHdr)
	for i, n := range lastHdrN {
		if lastHdrErr[i] == nil {
			assert.Equal(t, uint64(expectedBlockNum), n)
			assert.True(t, lastHdrT[i].After(expectedBlockTime))
		} else {
			numErr++
		}
	}
	assert.Equal(t, expectedNumErr, numErr)
	return
}

func waitForBlockEP(bc *bftDeliveryClient) (string, error) {
	for i := int64(0); ; i++ {
		blockEP := bc.GetEndpoint()
		if len(blockEP) > 0 {
			return blockEP, nil
		}
		time.Sleep(10 * time.Millisecond)
		if time.Duration(i*10*time.Millisecond.Nanoseconds()) > 5*time.Second {
			return "", errors.New("timeout: no block receiver")
		}
	}
}

func osnMapValues(m map[string]*mocks.Orderer) []*mocks.Orderer {
	s := make([]*mocks.Orderer, 0)
	for _, v := range m {
		s = append(s, v)
	}
	return s
}
