/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"encoding/asn1"
	"math/big"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

type Signature struct {
	Nonce                []byte
	SignatureHeader      []byte
	BlockHeader          []byte
	OrdererBlockMetadata []byte
	AuxiliaryInput       []byte
}

func (sig *Signature) Unmarshal(bytes []byte) error {
	_, err := asn1.Unmarshal(bytes, sig)
	return err
}

func (sig *Signature) Marshal() []byte {
	bytes, err := asn1.Marshal(*sig)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (sig Signature) AsBytes() []byte {
	msg2Sign := util.ConcatenateBytes(sig.OrdererBlockMetadata, sig.SignatureHeader, sig.BlockHeader, sig.AuxiliaryInput)
	return msg2Sign
}

func ProposalToBlock(proposal types.Proposal) (*common.Block, error) {
	// initialize block with empty fields
	block := &common.Block{
		Data:     &common.BlockData{},
		Metadata: &common.BlockMetadata{},
	}

	if len(proposal.Header) == 0 {
		return nil, errors.New("proposal header cannot be nil")
	}

	hdr := &asn1Header{}

	if _, err := asn1.Unmarshal(proposal.Header, hdr); err != nil {
		return nil, errors.Wrap(err, "bad header")
	}

	block.Header = &common.BlockHeader{
		Number:       hdr.Number.Uint64(),
		PreviousHash: hdr.PreviousHash,
		DataHash:     hdr.DataHash,
	}

	if len(proposal.Payload) == 0 {
		return nil, errors.New("proposal payload cannot be nil")
	}

	tuple := &ByteBufferTuple{}
	if err := tuple.FromBytes(proposal.Payload); err != nil {
		return nil, errors.Wrap(err, "bad payload and metadata tuple")
	}

	if err := proto.Unmarshal(tuple.A, block.Data); err != nil {
		return nil, errors.Wrap(err, "bad payload")
	}

	if err := proto.Unmarshal(tuple.B, block.Metadata); err != nil {
		return nil, errors.Wrap(err, "bad metadata")
	}
	return block, nil
}

type asn1Header struct {
	Number       *big.Int
	PreviousHash []byte
	DataHash     []byte
}
