package smartbft

import (
	smartbft "github.com/SmartBFT-Go/consensus/pkg/consensus"
	"github.com/hyperledger/fabric/common/flogging"
	//fuzzer "github.com/google/gofuzz"
	"math/rand"
	"time"

	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
)

type ProtocolFuzzer struct {
	consensus        *smartbft.Consensus
	Logger           *flogging.FabricLogger
}

func NewProtocolFuzzer (cons *smartbft.Consensus) *ProtocolFuzzer {

	logger := flogging.MustGetLogger("orderer.consensus.smartbft.protocolFuzzer")

	pf := &ProtocolFuzzer{
		consensus: cons,
		Logger: logger,
	}
	return pf
}

// for fuzz seeds
var PrePrePareMsgSeeds []*protos.Message
var PrePareMsgSeeds []*protos.Message
var CommitMsgSeeds []*protos.Message
var ViewChangeSeeds []*protos.Message
var ViewDataSeeds []*protos.Message
var NewViewSeeds []*protos.Message
var HeartBeatSeeds []*protos.Message
var HeartBeatResponseSeeds []*protos.Message
var StateTransferRequestSeeds []*protos.Message
var StateTransferResponseSeeds []*protos.Message


func AddPrePrePareMsgSeeds (m *protos.Message){
	PrePrePareMsgSeeds = append(PrePrePareMsgSeeds, m)
}

func AddPrePareMsgSeeds ( m *protos.Message) {
	PrePareMsgSeeds = append(PrePareMsgSeeds, m)
}

func AddCommitMsgSeeds ( m *protos.Message) {
	CommitMsgSeeds = append(CommitMsgSeeds, m)
}

func AddViewChangeSeeds ( m *protos.Message) {
	ViewChangeSeeds = append(ViewChangeSeeds, m)
}

func AddViewDataSeeds ( m *protos.Message) {
	ViewDataSeeds = append(ViewDataSeeds, m)
}

func AddNewViewSeeds ( m *protos.Message) {
	NewViewSeeds = append(NewViewSeeds, m)
}

func AddHeartBeatSeeds ( m *protos.Message) {
	HeartBeatSeeds = append(HeartBeatSeeds, m)
}

func AddHeartBeatResponseSeeds ( m *protos.Message) {
	HeartBeatResponseSeeds = append(HeartBeatResponseSeeds, m)
}


func AddStateTransferRequestSeeds ( m *protos.Message) {
	StateTransferRequestSeeds = append(StateTransferRequestSeeds, m)
}


func AddStateTransferResponseSeeds ( m *protos.Message) {
	StateTransferResponseSeeds = append(StateTransferResponseSeeds, m)
}



type MessageType int32

const (
	PrePrePareMsg      	MessageType = 0
	PrePareMsg      	MessageType = 1
	CommitMsg      		MessageType = 2
	ViewChangeMsg      	MessageType = 3
	ViewDataMsg      	MessageType = 4
	NewViewMsg      	MessageType = 5
	HeartBeatMsg      	MessageType = 6
	HeartBeatResponseMsg      		MessageType = 7
	StateTransferRequestMsg      	MessageType = 8
	StateTransferResponseMsg      	MessageType = 9
)

func (pf *ProtocolFuzzer) RunFuzz(){

	time.Sleep(time.Duration(60)*time.Second)
	pf.Logger.Errorf("Begin running consensus message fuzzing process!!!!!")


	c := pf.consensus.GerContoller()
	//if c.ID == c.GetLeaderID(){

		if c.ID == 1 || c.ID ==3 || c.ID == 5{
			pf.Logger.Errorf("I am a bad guy!!!!!")
		//if c.ID == 1 || c.ID == 2 || c.ID == 3	{   //only less than 1/3 nodes can be fuzzer nodes
			for {
				//if c.ID == c.GetLeaderID(){			//TODO only leader node send fuzzed messages
					msg := protocolFuzz()
					//pf.Logger.Errorf("Is next fuzzed message is nil? %b", (msg==nil))
					if msg != nil {
						pf.BroadcastConsensusFuzz(msg)
					}
				//}
				//TODO
				time.Sleep(time.Duration(5)*time.Second)
			}
		}else{
			pf.Logger.Errorf("I am a good guy!!!!!")
		}
	//}
}


func protocolFuzz() *protos.Message{
	var msg *protos.Message

	nextType:=getNextType()

	switch nextType {
	case PrePrePareMsg:
		msg=getNextMsgSeed(PrePrePareMsgSeeds)
		if msg == nil {
			return nil
		}
		PrePrePareMsgSeeds = append(PrePrePareMsgSeeds, PrePrePareMsgFuzz(msg))

	case PrePareMsg:
		msg=getNextMsgSeed(PrePareMsgSeeds)
		if msg == nil {
			return nil
		}
		PrePareMsgSeeds = append(PrePareMsgSeeds, PrePareMsgFuzz(msg))

	case CommitMsg:
		msg=getNextMsgSeed(CommitMsgSeeds)
		if msg == nil {
			return nil
		}
		CommitMsgSeeds = append(CommitMsgSeeds, CommitFuzz(msg))

	case ViewChangeMsg:
		msg=getNextMsgSeed(ViewChangeSeeds)
		if msg == nil {
			return nil
		}
		ViewChangeSeeds = append(ViewChangeSeeds, ViewChangeFuzz(msg))

	case ViewDataMsg:
		msg=getNextMsgSeed(ViewDataSeeds)
		if msg == nil {
			return nil
		}
		ViewDataSeeds = append(ViewDataSeeds, ViewDataFuzz(msg))

	case NewViewMsg:
		msg=getNextMsgSeed(NewViewSeeds)
		if msg == nil {
			return nil
		}
		NewViewSeeds = append(NewViewSeeds, NewViewFuzz(msg))

	case HeartBeatMsg:
		msg=getNextMsgSeed(HeartBeatSeeds)
		if msg == nil {
			return nil
		}
		HeartBeatSeeds = append(HeartBeatSeeds, HeartBeatFuzz(msg))

	case HeartBeatResponseMsg:
		msg=getNextMsgSeed(HeartBeatResponseSeeds)
		if msg == nil {
			return nil
		}
		HeartBeatResponseSeeds = append(HeartBeatResponseSeeds, HeartBeatResponseFuzz(msg))

	case StateTransferRequestMsg:
		msg=getNextMsgSeed(StateTransferRequestSeeds)
		if msg == nil {
			return nil
		}
		StateTransferRequestSeeds = append(StateTransferRequestSeeds, StateTransferRequest(msg))

	case StateTransferResponseMsg:
		msg=getNextMsgSeed(StateTransferResponseSeeds)
		if msg == nil {
			return nil
		}
		StateTransferResponseSeeds = append(StateTransferResponseSeeds, StateTransferResponse(msg))

	}
	return msg

}

func getNextType() MessageType{
	rand.Seed(time.Now().Unix())
	return MessageType(rand.Intn(11))
}

func getNextMsgSeed(seeds []*protos.Message) *protos.Message {
	if len(seeds) == 0{
		return nil
	}else{
		return seeds[rand.Intn(len(seeds))]
	}
}



// TODO every receiver always get the same message
func (pf *ProtocolFuzzer) BroadcastConsensusFuzz(msg *protos.Message){
	c := pf.consensus.GerContoller()

	probability := percentageRandom()

	pf.Logger.Infof("Start broadcastConsensusFuzz, probability is: %f", probability)

	if probability < 0.7 {

		//1. first strategy: send to all nodes
		for _, node := range c.NodesList {
			// Do not send to yourself
			if c.ID == node {
				continue
			}
			c.Comm.SendConsensus(node, msg)
		}

	}else if probability < 0.85 {

		//2. second strategy: send to half nodes
		for i, node := range c.NodesList {
			// TODO should randomly choose nodes
			if i < 6{
				continue
			}
			c.Comm.SendConsensus(node, msg)
		}


	}else{

		//3. third strategy: send to 1/3 nodes
		for i, node := range c.NodesList {
			// TODO should randomly choose nodes
			if i < 8{
				continue
			}
			c.Comm.SendConsensus(node, msg)
		}

	}


}

