/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBftHeaderReceiver_NoBlocks(t *testing.T) {
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	streamClientMock := &mocks.HeaderStreamClient{}
	hr := newBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, msgVerifierMock, 10*time.Millisecond, 10*time.Second)
	assert.NotNil(t, hr)

	_, _, err := hr.LastBlockNum()
	assert.EqualError(t, err, "Not found")

	streamClientMock.On("Recv").Return(nil, errors.New("oops"))
	streamClientMock.On("Close")
	hr.DeliverHeaders()
	_, _, err = hr.LastBlockNum()
	assert.EqualError(t, err, "Not found")
	msgVerifierMock.AssertNotCalled(t, "VerifyHeader")
}

func TestBftHeaderReceiver_WithBlocks(t *testing.T) {
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	streamClientMock := &mocks.HeaderStreamClient{}
	hr := newBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, msgVerifierMock, 10*time.Millisecond, 10*time.Second)

	seq := uint64(0)
	goodSig := uint32(1)
	streamClientMock.On("Recv").Return(
		func() *orderer.DeliverResponse {
			time.Sleep(time.Millisecond)
			seqNew := atomic.AddUint64(&seq, 1)
			return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, atomic.LoadUint32(&goodSig))
		},
		nil)
	streamClientMock.On("Close")
	streamClientMock.On("GetEndpoint").Return("test.com")

	msgVerifierMock.On("VerifyHeader", mock.Anything, mock.Anything).Return(
		func(_ string, signedBlock *common.Block) error {
			sigArray := signedBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES]
			sig := string(sigArray)
			if sig == "good" {
				return nil
			}
			return errors.New("test: bad signature")
		},
	)

	start := time.Now()
	go hr.DeliverHeaders()

	assert.True(t, waitForAtomicGreaterThan(&seq, 1))
	bNum, bTime, err := hr.LastBlockNum()
	assert.NoError(t, err)
	assert.True(t, uint64(0) < bNum, "expect bNum = %d > 0", bNum)
	assert.True(t, bTime.After(start))
	msgVerifierMock.AssertNumberOfCalls(t, "VerifyHeader", 1)

	bNumOld := bNum
	assert.True(t, waitForAtomicGreaterThan(&seq, bNumOld+2))
	bNum, bTime, err = hr.LastBlockNum()
	assert.NoError(t, err)
	assert.True(t, bNumOld < bNum, "expect bNum = %d > %d", bNum, bNumOld)
	assert.True(t, bTime.After(start))
	msgVerifierMock.AssertNumberOfCalls(t, "VerifyHeader", 2)

	//Invalid blocks
	bNumOld = bNum
	atomic.StoreUint32(&goodSig, 0)
	assert.True(t, waitForAtomicGreaterThan(&seq, bNumOld+3))
	bNum, bTime, err = hr.LastBlockNum()
	assert.EqualError(t, err, "Last block verification failed: test: bad signature")
	assert.True(t, bNumOld < bNum, "expect bNum = %d > %d", bNum, bNumOld)
	assert.True(t, bTime.After(start))
	msgVerifierMock.AssertNumberOfCalls(t, "VerifyHeader", 3)

	hr.Close()
	assert.True(t, hr.isStopped())
}

func TestBftHeaderReceiver_VerifyOnce(t *testing.T) {
	msgVerifierMock := &mocks.MessageCryptoVerifier{}
	streamClientMock := &mocks.HeaderStreamClient{}
	hr := newBFTHeaderReceiver("testchannel", "10.10.10.11:666", streamClientMock, msgVerifierMock, 10*time.Millisecond, 10*time.Second)

	seq := uint64(0)
	done := uint32(0)
	streamClientMock.On("Recv").Return(
		func() *orderer.DeliverResponse {
			for atomic.LoadUint64(&seq) > 0 && atomic.LoadUint32(&done) == 0 {
				time.Sleep(time.Millisecond)
			}
			seqNew := atomic.AddUint64(&seq, 1)
			return prepareBlock(seqNew, orderer.SeekInfo_HEADER_WITH_SIG, 1)
		},
		nil)
	streamClientMock.On("Close")
	streamClientMock.On("GetEndpoint").Return("test.com")

	msgVerifierMock.On("VerifyHeader", mock.Anything, mock.Anything).Return(
		func(_ string, signedBlock *common.Block) error {
			sigArray := signedBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES]
			sig := string(sigArray)
			if sig == "good" {
				return nil
			}
			return errors.New("test: bad signature")
		},
	)

	start := time.Now()
	go hr.DeliverHeaders()

	assert.True(t, waitForAtomicGreaterThan(&seq, 0))

	for i := 0; i < 10; {
		bNum, bTime, err := hr.LastBlockNum()
		if bNum > 0 {
			assert.NoError(t, err)
			assert.Equal(t, uint64(1), bNum)
			assert.True(t, bTime.After(start))
			i++
		} else {
			assert.EqualError(t, err, "Not found")
		}
	}
	msgVerifierMock.AssertNumberOfCalls(t, "VerifyHeader", 1)

	hr.Close()
	atomic.StoreUint32(&done, 1)
	assert.True(t, hr.isStopped())
}

func prepareBlock(seq uint64, contentType orderer.SeekInfo_SeekContentType, goodSignature uint32) *orderer.DeliverResponse {
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

	if goodSignature > 0 {
		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = []byte("good")
	} else {
		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = []byte("bad")
	}

	return &orderer.DeliverResponse{Type: &orderer.DeliverResponse_Block{Block: block}}
}

func waitForAtomicGreaterThan(addr *uint64, threshold uint64, timeoutOpt ...time.Duration) bool {
	to := 5 * time.Second
	if len(timeoutOpt) > 0 {
		to = timeoutOpt[0]
	}

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(to)

	for {
		select {
		case <-ticker.C:
		case <-timeout:
			return false
		}

		if atomic.LoadUint64(addr) > threshold {
			return true
		}
	}
}
