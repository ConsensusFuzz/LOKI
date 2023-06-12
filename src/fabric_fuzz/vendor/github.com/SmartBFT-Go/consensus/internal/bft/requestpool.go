// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
)

const (
	defaultRequestTimeout = 10 * time.Second // for unit tests only
)

//go:generate mockery -dir . -name RequestTimeoutHandler -case underscore -output ./mocks/

// RequestTimeoutHandler defines the methods called by request timeout timers created by time.AfterFunc.
// This interface is implemented by the bft.Controller.
type RequestTimeoutHandler interface {

	// OnRequestTimeout is called when a request timeout expires.
	OnRequestTimeout(request []byte, requestInfo types.RequestInfo)

	// OnLeaderFwdRequestTimeout is called when a leader forwarding timeout expires.
	OnLeaderFwdRequestTimeout(request []byte, requestInfo types.RequestInfo)

	// OnAutoRemoveTimeout is called when a auto-remove timeout expires.
	OnAutoRemoveTimeout(requestInfo types.RequestInfo)
}

// Pool implements requests pool, maintains pool of given size provided during
// construction. In case there are more incoming request than given size it will
// block during submit until there will be place to submit new ones.
type Pool struct {
	logger    api.Logger
	inspector api.RequestInspector
	options   PoolOptions

	lock           sync.Mutex
	fifo           *list.List
	semaphore      *semaphore.Weighted
	existMap       map[types.RequestInfo]*list.Element
	timeoutHandler RequestTimeoutHandler
	closed         bool
	stopped        bool
	submittedChan  chan struct{}
	sizeBytes      uint64
}

// requestItem captures request related information
type requestItem struct {
	request []byte
	timeout *time.Timer
}

// PoolOptions is the pool configuration
type PoolOptions struct {
	QueueSize         int64
	ForwardTimeout    time.Duration
	ComplainTimeout   time.Duration
	AutoRemoveTimeout time.Duration
}

// NewPool constructs new requests pool
func NewPool(log api.Logger, inspector api.RequestInspector, th RequestTimeoutHandler, options PoolOptions, submittedChan chan struct{}) *Pool {
	if options.ForwardTimeout == 0 {
		options.ForwardTimeout = defaultRequestTimeout
	}
	if options.ComplainTimeout == 0 {
		options.ComplainTimeout = defaultRequestTimeout
	}
	if options.AutoRemoveTimeout == 0 {
		options.AutoRemoveTimeout = defaultRequestTimeout
	}

	return &Pool{
		timeoutHandler: th,
		logger:         log,
		inspector:      inspector,
		fifo:           list.New(),
		semaphore:      semaphore.NewWeighted(options.QueueSize),
		existMap:       make(map[types.RequestInfo]*list.Element),
		options:        options,
		submittedChan:  submittedChan,
	}
}

// ChangeTimeouts changes the timeout of the pool
func (rp *Pool) ChangeTimeouts(th RequestTimeoutHandler, options PoolOptions) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	if !rp.stopped {
		rp.logger.Errorf("Trying to change timeouts but the pool is not stopped")
		return
	}

	if options.ForwardTimeout == 0 {
		options.ForwardTimeout = defaultRequestTimeout
	}
	if options.ComplainTimeout == 0 {
		options.ComplainTimeout = defaultRequestTimeout
	}
	if options.AutoRemoveTimeout == 0 {
		options.AutoRemoveTimeout = defaultRequestTimeout
	}

	rp.options.ForwardTimeout = options.ForwardTimeout
	rp.options.ComplainTimeout = options.ComplainTimeout
	rp.options.AutoRemoveTimeout = options.AutoRemoveTimeout

	rp.timeoutHandler = th

	rp.logger.Debugf("Changed pool timeouts")
}

func (rp *Pool) isClosed() bool {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	return rp.closed
}

// Submit a request into the pool, returns an error when request is already in the pool
func (rp *Pool) Submit(request []byte) error {
	reqInfo := rp.inspector.RequestID(request)
	if rp.isClosed() {
		return errors.Errorf("pool closed, request rejected: %s", reqInfo)
	}

	// do not wait for a semaphore with a lock, as it will prevent draining the pool.
	if err := rp.semaphore.Acquire(context.Background(), 1); err != nil {
		return errors.Wrapf(err, "acquiring semaphore for request: %s", reqInfo)
	}

	reqCopy := append(make([]byte, 0), request...)

	rp.lock.Lock()
	defer rp.lock.Unlock()

	if _, exist := rp.existMap[reqInfo]; exist {
		rp.semaphore.Release(1)
		errStr := fmt.Sprintf("request %s already exists in the pool", reqInfo)
		rp.logger.Errorf(errStr)
		return errors.New(errStr)
	}

	to := time.AfterFunc(
		rp.options.ForwardTimeout,
		func() { rp.onRequestTO(reqCopy, reqInfo) },
	)
	if rp.stopped {
		rp.logger.Debugf("pool stopped, submitting with a stopped timer, request: %s", reqInfo)
		to.Stop()
	}
	reqItem := &requestItem{
		request: reqCopy,
		timeout: to,
	}

	element := rp.fifo.PushBack(reqItem)
	rp.existMap[reqInfo] = element

	if len(rp.existMap) != rp.fifo.Len() {
		rp.logger.Panicf("RequestPool map and list are of different length: map=%d, list=%d", len(rp.existMap), rp.fifo.Len())
	}

	rp.logger.Debugf("Request %s submitted; started a timeout: %s", reqInfo, rp.options.ForwardTimeout)

	// notify that a request was submitted
	select {
	case rp.submittedChan <- struct{}{}:
	default:
	}

	rp.sizeBytes += uint64(len(element.Value.(*requestItem).request))

	return nil
}

// Size returns the number of requests currently residing the pool
func (rp *Pool) Size() int {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	return len(rp.existMap)
}

// NextRequests returns the next requests to be batched.
// It returns at most maxCount requests, and at most maxSizeBytes, in a newly allocated slice.
// Return variable full indicates that the batch cannot be increased further by calling again with the same arguments.
func (rp *Pool) NextRequests(maxCount int, maxSizeBytes uint64, check bool) (batch [][]byte, full bool) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	if check {
		if (len(rp.existMap) < maxCount) && (rp.sizeBytes < maxSizeBytes) {
			return nil, false
		}
	}

	count := minInt(rp.fifo.Len(), maxCount)
	var totalSize uint64
	batch = make([][]byte, 0, count)
	var element = rp.fifo.Front()
	for i := 0; i < count; i++ {
		req := element.Value.(*requestItem).request
		reqLen := uint64(len(req))
		if totalSize+reqLen > maxSizeBytes {
			rp.logger.Debugf("Returning batch of %d requests totalling %dB as it exceeds threshold of %dB",
				len(batch), totalSize, maxSizeBytes)
			return batch, true
		}
		batch = append(batch, req)
		totalSize = totalSize + reqLen
		element = element.Next()
	}

	fullS := totalSize >= maxSizeBytes
	fullC := len(batch) == maxCount
	full = fullS || fullC
	if len(batch) > 0 {
		rp.logger.Debugf("Returning batch of %d requests totalling %dB",
			len(batch), totalSize)
	}
	return batch, full
}

// Prune removes requests for which the given predicate returns error.
func (rp *Pool) Prune(predicate func([]byte) error) {
	reqVec, infoVec := rp.copyRequests()

	var numPruned int
	for i, req := range reqVec {
		err := predicate(req)
		if err == nil {
			continue
		}

		if remErr := rp.RemoveRequest(infoVec[i]); remErr != nil {
			rp.logger.Debugf("Failed to prune request: %s; predicate error: %s; remove error: %s", infoVec[i], err, remErr)
		} else {
			rp.logger.Debugf("Pruned request: %s; predicate error: %s", infoVec[i], err)
			numPruned++
		}
	}

	rp.logger.Debugf("Pruned %d requests", numPruned)
}

func (rp *Pool) copyRequests() (requestVec [][]byte, infoVec []types.RequestInfo) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	requestVec = make([][]byte, len(rp.existMap))
	infoVec = make([]types.RequestInfo, len(rp.existMap))

	var i int
	for info, item := range rp.existMap {
		infoVec[i] = info
		requestVec[i] = item.Value.(*requestItem).request
		i++
	}

	return
}

// RemoveRequest removes the given request from the pool
func (rp *Pool) RemoveRequest(requestInfo types.RequestInfo) error {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	element, exist := rp.existMap[requestInfo]
	if !exist {
		errStr := fmt.Sprintf("request %s is not in the pool at remove time", requestInfo)
		rp.logger.Debugf(errStr)
		return fmt.Errorf(errStr)
	}

	rp.deleteRequest(element, requestInfo)
	rp.sizeBytes -= uint64(len(element.Value.(*requestItem).request))
	return nil
}

func (rp *Pool) deleteRequest(element *list.Element, requestInfo types.RequestInfo) {
	item := element.Value.(*requestItem)
	item.timeout.Stop()

	rp.fifo.Remove(element)
	delete(rp.existMap, requestInfo)
	rp.logger.Infof("Removed request %s from request pool", requestInfo)
	rp.semaphore.Release(1)

	if len(rp.existMap) != rp.fifo.Len() {
		rp.logger.Panicf("RequestPool map and list are of different length: map=%d, list=%d", len(rp.existMap), rp.fifo.Len())
	}
}

// Close removes all the requests, stops all the timeout timers.
func (rp *Pool) Close() {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	rp.closed = true

	for requestInfo, element := range rp.existMap {
		rp.deleteRequest(element, requestInfo)
	}
}

// StopTimers stops all the timeout timers attached to the pending requests, and marks the pool as "stopped".
// This which prevents submission of new requests, and renewal of timeouts by timer go-routines that where running
// at the time of the call to StopTimers().
func (rp *Pool) StopTimers() {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	rp.stopped = true

	for _, element := range rp.existMap {
		item := element.Value.(*requestItem)
		item.timeout.Stop()
	}

	rp.logger.Debugf("Stopped all timers: size=%d", len(rp.existMap))
}

// RestartTimers restarts all the timeout timers attached to the pending requests, as RequestForwardTimeout, and re-allows
// submission of new requests.
func (rp *Pool) RestartTimers() {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	rp.stopped = false

	for reqInfo, element := range rp.existMap {
		item := element.Value.(*requestItem)
		item.timeout.Stop()
		to := time.AfterFunc(
			rp.options.ForwardTimeout,
			func() { rp.onRequestTO(item.request, reqInfo) },
		)
		item.timeout = to
	}

	rp.logger.Debugf("Restarted all timers: size=%d", len(rp.existMap))
}

func (rp *Pool) contains(reqInfo types.RequestInfo) bool {
	rp.lock.Lock()
	defer rp.lock.Unlock()
	_, contains := rp.existMap[reqInfo]
	return contains
}

// called by the goroutine spawned by time.AfterFunc
func (rp *Pool) onRequestTO(request []byte, reqInfo types.RequestInfo) {
	if !rp.contains(reqInfo) {
		return
	}
	// may take time, in case Comm channel to leader is full; hence w/o the lock.
	rp.logger.Debugf("Request %s timeout expired, going to send to leader", reqInfo)
	rp.timeoutHandler.OnRequestTimeout(request, reqInfo)

	rp.lock.Lock()
	defer rp.lock.Unlock()

	element, contains := rp.existMap[reqInfo]
	if !contains {
		rp.logger.Debugf("Request %s no longer in pool", reqInfo)
		return
	}

	if rp.closed || rp.stopped {
		rp.logger.Debugf("Pool stopped, will NOT start a leader-forwarding timeout")
		return
	}

	//start a second timeout
	item := element.Value.(*requestItem)
	item.timeout = time.AfterFunc(
		rp.options.ComplainTimeout,
		func() { rp.onLeaderFwdRequestTO(request, reqInfo) },
	)
	rp.logger.Debugf("Request %s; started a leader-forwarding timeout: %s", reqInfo, rp.options.ComplainTimeout)
}

// called by the goroutine spawned by time.AfterFunc
func (rp *Pool) onLeaderFwdRequestTO(request []byte, reqInfo types.RequestInfo) {
	if !rp.contains(reqInfo) {
		return
	}
	// may take time, in case Comm channel is full; hence w/o the lock.
	rp.logger.Debugf("Request %s leader-forwarding timeout expired, going to complain on leader", reqInfo)
	rp.timeoutHandler.OnLeaderFwdRequestTimeout(request, reqInfo)

	rp.lock.Lock()
	defer rp.lock.Unlock()

	element, contains := rp.existMap[reqInfo]
	if !contains {
		rp.logger.Debugf("Request %s no longer in pool", reqInfo)
		return
	}

	if rp.closed || rp.stopped {
		rp.logger.Debugf("Pool stopped, will NOT start auto-remove timeout")
		return
	}

	//start a third timeout
	item := element.Value.(*requestItem)
	item.timeout = time.AfterFunc(
		rp.options.AutoRemoveTimeout,
		func() { rp.onAutoRemoveTO(reqInfo) },
	)
	rp.logger.Debugf("Request %s; started auto-remove timeout: %s", reqInfo, rp.options.AutoRemoveTimeout)
}

// called by the goroutine spawned by time.AfterFunc
func (rp *Pool) onAutoRemoveTO(reqInfo types.RequestInfo) {
	rp.logger.Debugf("Request %s auto-remove timeout expired, going to remove from pool", reqInfo)
	if err := rp.RemoveRequest(reqInfo); err != nil {
		rp.logger.Errorf("Removal of request %s failed; error: %s", reqInfo, err)
		return
	}
	rp.timeoutHandler.OnAutoRemoveTimeout(reqInfo)
	return
}
