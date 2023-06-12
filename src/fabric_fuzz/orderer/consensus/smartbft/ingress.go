/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name MessageReceiver -case underscore -output mocks

// MessageReceiver receives messages
type MessageReceiver interface {
	HandleMessage(sender uint64, m *protos.Message)
	HandleRequest(sender uint64, req []byte)
}

//go:generate mockery -dir . -name ReceiverGetter -case underscore -output mocks

// ReceiverGetter obtains instances of MessageReceiver given a channel ID
type ReceiverGetter interface {
	// ReceiverByChain returns the MessageReceiver if it exists, or nil if it doesn't
	ReceiverByChain(channelID string) MessageReceiver
}

type WarningLogger interface {
	Warningf(template string, args ...interface{})
	Infof(template string, ars ...interface{})
}

// Ingreess dispatches Submit and Step requests to the designated per chain instances
type Ingreess struct {
	Logger        WarningLogger
	ChainSelector ReceiverGetter
}

// OnConsensus notifies the Ingreess for a reception of a StepRequest from a given sender on a given channel
func (in *Ingreess) OnConsensus(channel string, sender uint64, request *orderer.ConsensusRequest) error {

	receiver := in.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		in.Logger.Warningf("An attempt to send a consensus request to a non existing channel (%s) was made by %d", channel, sender)
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	msg := &protos.Message{}
	if err := proto.Unmarshal(request.Payload, msg); err != nil {
		in.Logger.Warningf("Malformed message: %v", err)
		return errors.Wrap(err, "malformed message")
	}

	//in.Logger.Warningf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! receive Consensus here !!!!!!!!!!!!!!!!!!!!!!!! type is: ", in.getType(msg))
	//in.Logger.Infof("Receive consensus, type is %s 		content is %s", in.getType(msg),msg.String())

	receiver.HandleMessage(sender, msg)
	return nil
}

// OnSubmit notifies the Ingreess for a reception of a SubmitRequest from a given sender on a given channel
func (in *Ingreess) OnSubmit(channel string, sender uint64, request *orderer.SubmitRequest) error {
	//in.Logger.Warningf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! receive Transaction here !!!!!!!!!!!!!!!!!!!!!!!!")


	receiver := in.ChainSelector.ReceiverByChain(channel)
	if receiver == nil {
		in.Logger.Warningf("An attempt to submit a transaction to a non existing channel (%s) was made by %d", channel, sender)
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	receiver.HandleRequest(sender, utils.MarshalOrPanic(request.Payload))
	return nil
}



func (in *Ingreess) getType( m *protos.Message) string{
	switch m.GetContent().(type) {
	case *protos.Message_PrePrepare:
		return "PrePrepare"
	case *protos.Message_Prepare:
		return "Prepare"
	case *protos.Message_Commit:
		return "Commit"
	case *protos.Message_ViewChange:
		return "ViewChange"
	case *protos.Message_ViewData:
		return "ViewData"
	case *protos.Message_NewView:
		return "NewView"
	case *protos.Message_HeartBeat:
		return "HeartBeat"
	case *protos.Message_HeartBeatResponse:
		return "HeartBeatResponse"
	case *protos.Message_StateTransferRequest:
		return "StateTransferRequest"
	case *protos.Message_StateTransferResponse:
		return "StateTransferResponse"
	default:
		in.Logger.Warningf("Unexpected message type, ignoring")
		return ""
	}

}