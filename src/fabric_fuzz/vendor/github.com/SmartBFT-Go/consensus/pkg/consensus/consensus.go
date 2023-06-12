// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package consensus

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"

	algorithm "github.com/SmartBFT-Go/consensus/internal/bft"
	bft "github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/pkg/errors"
)

// Consensus submits requests to be total ordered,
// and delivers to the application proposals by invoking Deliver() on it.
// The proposals contain batches of requests assembled together by the Assembler.
type Consensus struct {
	Config            types.Configuration
	Application       bft.Application
	Assembler         bft.Assembler
	WAL               bft.WriteAheadLog
	WALInitialContent [][]byte
	Comm              bft.Comm
	Signer            bft.Signer
	Verifier          bft.Verifier
	RequestInspector  bft.RequestInspector
	Synchronizer      bft.Synchronizer
	Logger            bft.Logger
	Metadata          protos.ViewMetadata
	LastProposal      types.Proposal
	LastSignatures    []types.Signature
	Scheduler         <-chan time.Time
	ViewChangerTicker <-chan time.Time

	submittedChan chan struct{}
	inFlight      *algorithm.InFlightData
	checkpoint    *types.Checkpoint
	pool          *algorithm.Pool
	viewChanger   *algorithm.ViewChanger
	controller    *algorithm.Controller
	collector     *algorithm.StateCollector
	state         *algorithm.PersistedState
	numberOfNodes uint64
	nodes         []uint64

	consensusDone sync.WaitGroup
	stopOnce      sync.Once
	stopChan      chan struct{}

	consensusLock sync.RWMutex

	reconfigChan chan types.Reconfig

	running uint64
}

func (c *Consensus) Complain(viewNum uint64, stopView bool) {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.viewChanger.StartViewChange(viewNum, stopView)
}

func (c *Consensus) Deliver(proposal types.Proposal, signatures []types.Signature) types.Reconfig {
	reconfig := c.Application.Deliver(proposal, signatures)
	if reconfig.InLatestDecision {
		c.Logger.Debugf("Detected a reconfig in deliver")
		c.reconfigChan <- reconfig
	}
	return reconfig
}

func (c *Consensus) Sync() types.SyncResponse {
	syncResponse := c.Synchronizer.Sync()
	if syncResponse.Reconfig.InReplicatedDecisions {
		c.Logger.Debugf("Detected a reconfig in sync")
		c.reconfigChan <- types.Reconfig{
			InLatestDecision: true,
			CurrentNodes:     syncResponse.Reconfig.CurrentNodes,
			CurrentConfig:    syncResponse.Reconfig.CurrentConfig,
		}
	}
	return syncResponse
}

// GetLeaderID returns the current leader ID or zero if Consensus is not running
func (c *Consensus) GetLeaderID() uint64 {
	if atomic.LoadUint64(&c.running) == 0 {
		return 0
	}
	return c.controller.GetLeaderID()
}

func (c *Consensus) Start() error {
	if err := c.ValidateConfiguration(c.Comm.Nodes()); err != nil {
		return errors.Wrapf(err, "configuration is invalid")
	}

	c.consensusDone.Add(1)
	c.stopOnce = sync.Once{}
	c.stopChan = make(chan struct{})
	c.reconfigChan = make(chan types.Reconfig)
	c.consensusLock.Lock()
	defer c.consensusLock.Unlock()

	c.setNodes(c.Comm.Nodes())

	c.inFlight = &algorithm.InFlightData{}

	c.state = &algorithm.PersistedState{
		InFlightProposal: c.inFlight,
		Entries:          c.WALInitialContent,
		Logger:           c.Logger,
		WAL:              c.WAL,
	}

	c.checkpoint = &types.Checkpoint{}
	c.checkpoint.Set(c.LastProposal, c.LastSignatures)

	c.createComponents()
	opts := algorithm.PoolOptions{
		QueueSize:         int64(c.Config.RequestPoolSize),
		ForwardTimeout:    c.Config.RequestForwardTimeout,
		ComplainTimeout:   c.Config.RequestComplainTimeout,
		AutoRemoveTimeout: c.Config.RequestAutoRemoveTimeout,
	}
	c.submittedChan = make(chan struct{}, 1)
	c.pool = algorithm.NewPool(c.Logger, c.RequestInspector, c.controller, opts, c.submittedChan)
	c.continueCreateComponents()

	c.Logger.Debugf("Application started with view %d, seq %d, and decisions %d", c.Metadata.ViewId, c.Metadata.LatestSequence, c.Metadata.DecisionsInView)
	view, seq, dec := c.setViewAndSeq(c.Metadata.ViewId, c.Metadata.LatestSequence, c.Metadata.DecisionsInView)

	c.waitForEachOther()

	go func() {
		defer c.consensusDone.Done()
		c.run()
	}()

	c.startComponents(view, seq, dec, true)

	atomic.StoreUint64(&c.running, 1)

	return nil
}

func (c *Consensus) run() {
	defer func() {
		c.Logger.Infof("Exiting")
	}()

	for {
		select {
		case reconfig := <-c.reconfigChan:
			c.reconfig(reconfig)
		case <-c.stopChan:
			return
		}
	}
}

func (c *Consensus) reconfig(reconfig types.Reconfig) {
	c.Logger.Debugf("Starting reconfig")
	c.consensusLock.Lock()
	defer c.consensusLock.Unlock()

	// make sure all components are stopped
	c.viewChanger.Stop()
	c.controller.StopWithPoolPause()
	c.collector.Stop()

	c.Config = reconfig.CurrentConfig
	if err := c.ValidateConfiguration(reconfig.CurrentNodes); err != nil {
		if strings.Contains(err.Error(), "nodes does not contain the SelfID") {
			c.close()
			c.Logger.Infof("Closing consensus since this node is not in the current set of nodes")
			return
		} else {
			c.Logger.Panicf("Configuration is invalid, error: %v", err)
		}
	}

	c.setNodes(reconfig.CurrentNodes)

	c.createComponents()
	opts := algorithm.PoolOptions{
		ForwardTimeout:    c.Config.RequestForwardTimeout,
		ComplainTimeout:   c.Config.RequestComplainTimeout,
		AutoRemoveTimeout: c.Config.RequestAutoRemoveTimeout,
	}
	c.pool.ChangeTimeouts(c.controller, opts) // TODO handle reconfiguration of queue size in the pool
	c.continueCreateComponents()

	proposal, _ := c.checkpoint.Get()
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(proposal.Metadata, md); err != nil {
		c.Logger.Panicf("Couldn't unmarshal the checkpoint metadata, error: %v", err)
	}
	c.Logger.Debugf("Checkpoint with view %d and seq %d", md.ViewId, md.LatestSequence)

	view, seq, dec := c.setViewAndSeq(md.ViewId, md.LatestSequence, md.DecisionsInView)

	c.waitForEachOther()

	c.startComponents(view, seq, dec, false)

	c.pool.RestartTimers()

	c.Logger.Debugf("Reconfig is done")
}

func (c *Consensus) close() {
	c.stopOnce.Do(
		func() {
			select {
			case <-c.stopChan:
				return
			default:
				close(c.stopChan)
			}
		},
	)
}

func (c *Consensus) Stop() {
	c.consensusLock.RLock()
	atomic.StoreUint64(&c.running, 0)
	c.viewChanger.Stop()
	c.controller.Stop()
	c.collector.Stop()
	c.consensusLock.RUnlock()
	c.close()
	c.consensusDone.Wait()
}

func (c *Consensus) HandleMessage(sender uint64, m *protos.Message) {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.controller.ProcessMessages(sender, m)
}

func (c *Consensus) HandleRequest(sender uint64, req []byte) {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.controller.HandleRequest(sender, req)
}

func (c *Consensus) SubmitRequest(req []byte) error {
	c.consensusLock.RLock()
	defer c.consensusLock.RUnlock()
	c.Logger.Debugf("Submit Request: %s", c.RequestInspector.RequestID(req))
	return c.controller.SubmitRequest(req)
}

func (c *Consensus) proposalMaker() *algorithm.ProposalMaker {
	return &algorithm.ProposalMaker{
		DecisionsPerLeader: c.Config.DecisionsPerLeader,
		Checkpoint:         c.checkpoint,
		State:              c.state,
		Comm:               c.controller,
		Decider:            c.controller,
		Logger:             c.Logger,
		Signer:             c.Signer,
		SelfID:             c.Config.SelfID,
		Sync:               c.controller,
		FailureDetector:    c,
		Verifier:           c.Verifier,
		N:                  c.numberOfNodes,
		InMsqQSize:         int(c.Config.IncomingMessageBufferSize),
		ViewSequences:      c.controller.ViewSequences,
	}
}

func (c *Consensus) ValidateConfiguration(nodes []uint64) error {
	if err := c.Config.Validate(); err != nil {
		return errors.Wrap(err, "bad configuration")
	}

	nodeSet := make(map[uint64]bool)
	for _, val := range nodes {
		if val == 0 {
			return errors.Errorf("nodes contains node id 0 which is not permitted, nodes: %v", nodes)
		}
		nodeSet[val] = true
	}

	if !nodeSet[c.Config.SelfID] {
		return errors.Errorf("nodes does not contain the SelfID: %d, nodes: %v", c.Config.SelfID, nodes)
	}

	if len(nodeSet) != len(nodes) {
		return errors.Errorf("nodes contains duplicate IDs, nodes: %v", nodes)
	}

	return nil
}

func (c *Consensus) setNodes(nodes []uint64) {
	c.numberOfNodes = uint64(len(nodes))
	c.nodes = sortNodes(nodes)
}

func sortNodes(nodes []uint64) []uint64 {
	sorted := make([]uint64, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	return sorted
}

func (c *Consensus) createComponents() {
	c.viewChanger = &algorithm.ViewChanger{
		SelfID:             c.Config.SelfID,
		N:                  c.numberOfNodes,
		NodesList:          c.nodes,
		LeaderRotation:     c.Config.LeaderRotation,
		DecisionsPerLeader: c.Config.DecisionsPerLeader,
		SpeedUpViewChange:  c.Config.SpeedUpViewChange,
		Logger:             c.Logger,
		Signer:             c.Signer,
		Verifier:           c.Verifier,
		Application:        c,
		Checkpoint:         c.checkpoint,
		InFlight:           c.inFlight,
		State:              c.state,
		// Controller later
		// RequestsTimer later
		Ticker:            c.ViewChangerTicker,
		ResendTimeout:     c.Config.ViewChangeResendInterval,
		ViewChangeTimeout: c.Config.ViewChangeTimeout,
		InMsqQSize:        int(c.Config.IncomingMessageBufferSize),
	}

	c.collector = &algorithm.StateCollector{
		SelfID:         c.Config.SelfID,
		N:              c.numberOfNodes,
		Logger:         c.Logger,
		CollectTimeout: c.Config.CollectTimeout,
	}

	c.controller = &algorithm.Controller{
		Checkpoint:         c.checkpoint,
		WAL:                c.WAL,
		ID:                 c.Config.SelfID,
		N:                  c.numberOfNodes,
		NodesList:          c.nodes,
		LeaderRotation:     c.Config.LeaderRotation,
		DecisionsPerLeader: c.Config.DecisionsPerLeader,
		Verifier:           c.Verifier,
		Logger:             c.Logger,
		Assembler:          c.Assembler,
		Application:        c,
		FailureDetector:    c,
		Synchronizer:       c,
		Comm:               c.Comm,
		Signer:             c.Signer,
		RequestInspector:   c.RequestInspector,
		ViewChanger:        c.viewChanger,
		ViewSequences:      &atomic.Value{},
		Collector:          c.collector,
		State:              c.state,
	}

	c.viewChanger.Comm = c.controller
	c.viewChanger.Synchronizer = c.controller

	c.controller.ProposerBuilder = c.proposalMaker()
}

func (c *Consensus) continueCreateComponents() {
	batchBuilder := algorithm.NewBatchBuilder(c.pool, c.submittedChan, c.Config.RequestBatchMaxCount, c.Config.RequestBatchMaxBytes, c.Config.RequestBatchMaxInterval)
	leaderMonitor := algorithm.NewHeartbeatMonitor(c.Scheduler, c.Logger, c.Config.LeaderHeartbeatTimeout, c.Config.LeaderHeartbeatCount, c.controller, c.numberOfNodes, c.controller, c.controller.ViewSequences, c.Config.NumOfTicksBehindBeforeSyncing)
	c.controller.RequestPool = c.pool
	c.controller.Batcher = batchBuilder
	c.controller.LeaderMonitor = leaderMonitor

	c.viewChanger.Controller = c.controller
	c.viewChanger.Pruner = c.controller
	c.viewChanger.RequestsTimer = c.pool
	c.viewChanger.ViewSequences = c.controller.ViewSequences
}

func (c *Consensus) setViewAndSeq(view, seq, dec uint64) (newView, newSeq, newDec uint64) {
	newView = view
	newSeq = seq
	// decisions in view is incremented after delivery,
	// so if we delivered to the application proposal with decisions i,
	// then we are expecting to be proposed a proposal with decisions i+1,
	// unless this is the genesis block, or after a view change
	newDec = dec + 1
	if seq == 0 {
		newDec = 0
	}
	viewChange, err := c.state.LoadViewChangeIfApplicable()
	if err != nil {
		c.Logger.Panicf("Failed loading view change, error: %v", err)
	}
	if viewChange == nil {
		c.Logger.Debugf("No view change to restore")
	} else {
		// Check if the view change has a newer view
		if viewChange.NextView >= view {
			c.Logger.Debugf("Restoring from view change with view %d", viewChange.NextView)
			newView = viewChange.NextView
			restoreChan := make(chan struct{}, 1)
			restoreChan <- struct{}{}
			c.viewChanger.Restore = restoreChan
		}
	}

	viewSeq, err := c.state.LoadNewViewIfApplicable()
	if err != nil {
		c.Logger.Panicf("Failed loading new view, error: %v", err)
	}
	if viewSeq == nil {
		c.Logger.Debugf("No new view to restore")
	} else {
		// Check if metadata should be taken from the restored new view
		if viewSeq.Seq >= seq {
			c.Logger.Debugf("Restoring from new view with view %d and seq %d", viewSeq.View, viewSeq.Seq)
			newView = viewSeq.View
			newSeq = viewSeq.Seq
			newDec = 0
		}
	}
	return newView, newSeq, newDec
}

func (c *Consensus) waitForEachOther() {
	c.viewChanger.ControllerStartedWG = sync.WaitGroup{}
	c.viewChanger.ControllerStartedWG.Add(1)
	c.controller.StartedWG = &c.viewChanger.ControllerStartedWG
}

func (c *Consensus) startComponents(view, seq, dec uint64, configSync bool) {
	// If we delivered to the application proposal with sequence i,
	// then we are expecting to be proposed a proposal with sequence i+1.
	c.collector.Start()
	c.viewChanger.Start(view)
	if configSync {
		c.controller.Start(view, seq+1, dec, c.Config.SyncOnStart)
	} else {
		c.controller.Start(view, seq+1, dec, false)
	}
}


func (c *Consensus) GerContoller() *algorithm.Controller {
	return c.controller
}