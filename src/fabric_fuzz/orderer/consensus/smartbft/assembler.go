/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"encoding/asn1"

	"sync/atomic"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name Ledger -case underscore -output mocks

type Ledger interface {
	// Height returns the number of blocks in the ledger this channel is associated with.
	Height() uint64

	// Block returns a block with the given number,
	// or nil if such a block doesn't exist.
	Block(number uint64) *common.Block
}

type Assembler struct {
	RuntimeConfig   *atomic.Value
	Logger          *flogging.FabricLogger
	VerificationSeq func() uint64
}

func (a *Assembler) AssembleProposal(metadata []byte, requests [][]byte) (nextProp types.Proposal) {
	rtc := a.RuntimeConfig.Load().(RuntimeConfig)

	lastConfigBlockNum := rtc.LastConfigBlock.Header.Number
	lastBlock := rtc.LastBlock

	if len(requests) == 0 {
		a.Logger.Panicf("Programming error, no requests in proposal")
	}
	batchedRequests := singleConfigTxOrSeveralNonConfigTx(requests, a.Logger)

	block := common.NewBlock(lastBlock.Header.Number+1, lastBlock.Header.Hash())
	block.Data = &common.BlockData{Data: batchedRequests}
	block.Header.DataHash = block.Data.Hash()

	if isConfigBlock(block) {
		lastConfigBlockNum = block.Header.Number
	}

	block.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: lastConfigBlockNum}),
	})
	block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.OrdererBlockMetadata{
			ConsenterMetadata: metadata,
			LastConfig: &common.LastConfig{
				Index: lastConfigBlockNum,
			},
		}),
	})

	tuple := &ByteBufferTuple{
		A: utils.MarshalOrPanic(block.Data),
		B: utils.MarshalOrPanic(block.Metadata),
	}

	prop := types.Proposal{
		Header:               block.Header.Bytes(),
		Payload:              tuple.ToBytes(),
		Metadata:             metadata,
		VerificationSequence: int64(a.VerificationSeq()),
	}

	return prop
}

func singleConfigTxOrSeveralNonConfigTx(requests [][]byte, logger Logger) [][]byte {
	// Scan until a config transaction is found
	var batchedRequests [][]byte
	var i int
	for i < len(requests) {
		currentRequest := requests[i]
		envelope, err := utils.UnmarshalEnvelope(currentRequest)
		if err != nil {
			logger.Panicf("Programming error, received bad envelope but should have validated it: %v", err)
			continue
		}

		// If we saw a config transaction, we cannot add any more transactions to the batch.
		if utils.IsConfigTransaction(envelope) {
			break
		}

		// Else, it's not a config transaction, so add it to the batch.
		batchedRequests = append(batchedRequests, currentRequest)
		i++
	}

	// If we don't have any transaction in the batch, it is safe to assume we only
	// saw a single transaction which is a config transaction.
	if len(batchedRequests) == 0 {
		batchedRequests = [][]byte{requests[0]}
	}

	// At this point, batchedRequests contains either a single config transaction, or a few non config transactions.
	return batchedRequests
}

func LastConfigBlockFromLedgerOrPanic(ledger Ledger, logger Logger) *common.Block {
	block, err := lastConfigBlockFromLedger(ledger)
	if err != nil {
		logger.Panicf("Failed retrieving last config block: %v", err)
	}
	return block
}

func lastConfigBlockFromLedger(ledger Ledger) (*common.Block, error) {
	lastBlockSeq := ledger.Height() - 1
	lastBlock := ledger.Block(lastBlockSeq)
	if lastBlock == nil {
		return nil, errors.Errorf("unable to retrieve block [%d]", lastBlockSeq)
	}
	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, ledger)
	if err != nil {
		return nil, err
	}
	return lastConfigBlock, nil
}

func PreviousConfigBlockFromLedgerOrPanic(ledger Ledger, logger Logger) *common.Block {
	block, err := previousConfigBlockFromLedger(ledger)
	if err != nil {
		logger.Panicf("Failed retrieving previous config block: %v", err)
	}
	return block
}

func previousConfigBlockFromLedger(ledger Ledger) (*common.Block, error) {
	previousBlockSeq := ledger.Height() - 2
	if ledger.Height() == 1 {
		previousBlockSeq = 0
	}
	previousBlock := ledger.Block(previousBlockSeq)
	if previousBlock == nil {
		return nil, errors.Errorf("unable to retrieve block [%d]", previousBlockSeq)
	}
	previousConfigBlock, err := cluster.LastConfigBlock(previousBlock, ledger)
	if err != nil {
		return nil, err
	}
	return previousConfigBlock, nil
}

func LastBlockFromLedgerOrPanic(ledger Ledger, logger Logger) *common.Block {
	lastBlockSeq := ledger.Height() - 1
	lastBlock := ledger.Block(lastBlockSeq)
	if lastBlock == nil {
		logger.Panicf("Failed retrieving last block")
	}
	return lastBlock
}

type ByteBufferTuple struct {
	A []byte
	B []byte
}

func (bbt *ByteBufferTuple) ToBytes() []byte {
	bytes, err := asn1.Marshal(*bbt)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (bbt *ByteBufferTuple) FromBytes(bytes []byte) error {
	_, err := asn1.Unmarshal(bytes, bbt)
	return err
}
