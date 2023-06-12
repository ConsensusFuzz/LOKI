/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type Orderer struct {
	net.Listener
	*grpc.Server
	nextExpectedSeek uint64
	t                *testing.T
	blockChannel     chan uint64
	stop             bool
	stopChan         chan struct{}
	failFlag         int32

	mutex       sync.Mutex
	connCount   uint32
	contentType orderer.SeekInfo_SeekContentType
}

func NewOrderer(port int, t *testing.T) *Orderer {
	srv := grpc.NewServer()
	lsnr, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(err)
	}
	o := &Orderer{
		Server:           srv,
		Listener:         lsnr,
		t:                t,
		nextExpectedSeek: uint64(1),
		blockChannel:     make(chan uint64, 1),
		stopChan:         make(chan struct{}, 1),
	}
	orderer.RegisterAtomicBroadcastServer(srv, o)
	go func() { _ = srv.Serve(lsnr) }()
	return o
}

func (o *Orderer) Shutdown() {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.stop {
		return
	}

	o.stop = true
	close(o.stopChan)
	o.Server.Stop()
	_ = o.Listener.Close()
}

func (o *Orderer) isStop() bool {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.stop {
		return true
	}
	return false
}
func (o *Orderer) Fail() {
	if o.isStop() {
		return
	}
	atomic.StoreInt32(&o.failFlag, int32(1))
	o.blockChannel <- 0
}

func (o *Orderer) Resurrect() {
	if o.isStop() {
		return
	}
	atomic.StoreInt32(&o.failFlag, int32(0))
	for {
		select {
		case <-o.blockChannel:
			continue
		default:
			return
		}
	}
}

func (o *Orderer) ConnCount() int {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	return int(o.connCount)
}

func (o *Orderer) ConnCountType() (int, orderer.SeekInfo_SeekContentType) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	return int(o.connCount), o.contentType
}

func (o *Orderer) hasFailed() bool {
	return atomic.LoadInt32(&o.failFlag) == int32(1)
}

func (*Orderer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("Should not have ben called")
}

func (o *Orderer) SetNextExpectedSeek(seq uint64) {
	atomic.StoreUint64(&o.nextExpectedSeek, uint64(seq))
}

func (o *Orderer) SendBlock(seq uint64) {
	if o.isStop() {
		return
	}
	o.blockChannel <- seq
}

func (o *Orderer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {

	envlp, err := stream.Recv()
	if err != nil {
		return nil
	}
	if o.hasFailed() {
		return stream.Send(statusUnavailable())
	}

	payload := &common.Payload{}
	_ = proto.Unmarshal(envlp.Payload, payload)
	seekInfo := &orderer.SeekInfo{}
	_ = proto.Unmarshal(payload.Data, seekInfo)
	assert.True(o.t, seekInfo.Behavior == orderer.SeekInfo_BLOCK_UNTIL_READY)
	assert.Equal(o.t, atomic.LoadUint64(&o.nextExpectedSeek), seekInfo.Start.GetSpecified().Number, "seekInfo=%v, Addr=%v", seekInfo, o.Addr())

	o.mutex.Lock()
	o.connCount++
	o.contentType = seekInfo.ContentType
	o.mutex.Unlock()

	defer func() {
		o.mutex.Lock()
		o.connCount--
		o.mutex.Unlock()
	}()

	for {
		select {
		case _ = <-o.stopChan:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		case seq := <-o.blockChannel:
			if o.hasFailed() {
				return stream.Send(statusUnavailable())
			}
			o.sendBlock(stream, seq, seekInfo.ContentType)
		}
	}
}

func statusUnavailable() *orderer.DeliverResponse {
	return &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Status{
			Status: common.Status_SERVICE_UNAVAILABLE,
		},
	}
}

func (o *Orderer) sendBlock(stream orderer.AtomicBroadcast_DeliverServer, seq uint64, contentType orderer.SeekInfo_SeekContentType) {
	const numTx = 10
	block := common.NewBlock(seq, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	data := &common.BlockData{
		Data: make([][]byte, numTx),
	}
	for i := 0; i < numTx; i++ {
		data.Data[i] = []byte{byte(i), byte(seq)}
	}
	block.Header.DataHash = data.Hash()
	if contentType == orderer.SeekInfo_BLOCK {
		block.Data = data
	}

	block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = []byte("good")

	_ = stream.Send(&orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: block},
	})
}
