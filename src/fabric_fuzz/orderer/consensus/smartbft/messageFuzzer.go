package smartbft

import (
	//"github.com/hyperledger/fabric/protos/common"
	fuzzer "github.com/google/gofuzz"
	"math/rand"

	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
)

// ======================= funcs for Consensus message Fuzzer =======================
func ConsensusMessageFuzzed(m *protos.Message){

	switch m.GetContent().(type) {
	case *protos.Message_PrePrepare:
		PrePrePareMsgFuzz(m)
	case *protos.Message_Prepare:
		PrePareMsgFuzz(m)
	case *protos.Message_Commit:
		CommitFuzz(m)
	case *protos.Message_ViewChange:
		ViewChangeFuzz(m)
	case *protos.Message_ViewData:
		ViewDataFuzz(m)
	case *protos.Message_NewView:
		NewViewFuzz(m)
	case *protos.Message_HeartBeat:
		HeartBeatFuzz(m)
	case *protos.Message_HeartBeatResponse:
		HeartBeatResponseFuzz(m)
	case *protos.Message_StateTransferRequest:
		StateTransferRequest(m)
	case *protos.Message_StateTransferResponse:
		StateTransferResponse(m)
	default:
		//Logger.Warnf("Unexpected message type, ignoring")
	}
}


func StateTransferResponse(m *protos.Message) *protos.Message {
	f:=fuzzer.New().NilChance(0.1)
	stateTransferResponse := m.GetStateTransferResponse()
	f.Fuzz(&stateTransferResponse.Sequence)
	f.Fuzz(&stateTransferResponse.ViewNum)
	return m
}

func StateTransferRequest(m *protos.Message) *protos.Message {
	//f:=fuzzer.New().NilChance(0.1)
	//stateTransferRequest := m.GetStateTransferRequest()

	//f.Fuzz(stateTransferRequest.)
	return m
}

func HeartBeatResponseFuzz(m *protos.Message) *protos.Message {
	f:=fuzzer.New().NilChance(0.1)
	heartBeatResponse := m.GetHeartBeatResponse()

	f.Fuzz(&heartBeatResponse.View)

	return m
}

func HeartBeatFuzz(m *protos.Message) *protos.Message {
	f:=fuzzer.New().NilChance(0.1)
	heartBeat := m.GetHeartBeat()

	f.Fuzz(&heartBeat.View)

	heartBeat.Seq = seqFuzz(heartBeat.Seq)

	return m
}

func NewViewFuzz(m *protos.Message) *protos.Message {
	//f:=fuzzer.New().NilChance(0.1)
	//newView := m.GetNewView()
	//
	//f.Fuzz(newView.SignedViewData)
	return m
}

func ViewDataFuzz(m *protos.Message) *protos.Message {
	//f:=fuzzer.New().NilChance(0.1)
	//viewData := m.GetNewView()
	//
	//f.Fuzz(viewData.SignedViewData)
	return m
}

func ViewChangeFuzz(m *protos.Message) *protos.Message {
	f:=fuzzer.New().NilChance(0.1)
	viewChange := m.GetViewChange()

	f.Fuzz(&viewChange.Reason)
	f.Fuzz(&viewChange.NextView)

	return m
}

func CommitFuzz(m *protos.Message) *protos.Message {
	f:=fuzzer.New().NilChance(0.1)
	commit := m.GetCommit()

	f.Fuzz(&commit.View)
	commit.Seq = seqFuzz(commit.Seq)


	f.Fuzz(&commit.Assist)
	f.Fuzz(&commit.Digest)
	f.Fuzz(commit.Signature)

	return m
}



func PrePrePareMsgFuzz(m *protos.Message) *protos.Message{
	f:=fuzzer.New().NilChance(0.1)
	prePrepare := m.GetPrePrepare()

	f.Fuzz(&prePrepare.View)

	prePrepare.Seq = seqFuzz(prePrepare.Seq)

	//f.Fuzz(prePrepare.PrevCommitSignatures)
	f.Fuzz(prePrepare.Proposal)
	return m
}


func PrePareMsgFuzz(m *protos.Message) *protos.Message{
	f:=fuzzer.New().NilChance(0.1)
	prepare := m.GetPrepare()

	f.Fuzz(&prepare.View)

	prepare.Seq = seqFuzz(prepare.Seq)

	f.Fuzz(&prepare.Assist)
	f.Fuzz(&prepare.Digest)

	return m
}



// ======================= Funcs for Transaction message Fuzzer =======================
func TransactionMessageFuzzed(req []byte) []byte{
	f:=fuzzer.New().NilChance(0.1)
	f.Fuzz(&req)
	return req
}



// ======================= Funcs for common =======================

func getType(m *protos.Message) string{
	switch m.GetContent().(type) {
	case *protos.Message_PrePrepare:
		AddPrePrePareMsgSeeds(m)
		return "PrePrepare"
	case *protos.Message_Prepare:
		AddPrePareMsgSeeds(m)
		return "Prepare"
	case *protos.Message_Commit:
		AddCommitMsgSeeds(m)
		return "Commit"
	case *protos.Message_ViewChange:
		AddViewChangeSeeds(m)
		return "ViewChange"
	case *protos.Message_ViewData:
		AddViewDataSeeds(m)
		return "ViewData"
	case *protos.Message_NewView:
		AddNewViewSeeds(m)
		return "NewView"
	case *protos.Message_HeartBeat:
		AddHeartBeatSeeds(m)
		return "HeartBeat"
	case *protos.Message_HeartBeatResponse:
		AddHeartBeatResponseSeeds(m)
		return "HeartBeatResponse"
	case *protos.Message_StateTransferRequest:
		AddStateTransferRequestSeeds(m)
		return "StateTransferRequest"
	case *protos.Message_StateTransferResponse:
		AddStateTransferResponseSeeds(m)
		return "StateTransferResponse"
	default:
		return "Unexpected message type, ignoring"
	}
}

func addTx(req []byte){
	AddTransactionSeeds(req)
}

func seqFuzz(seq uint64) uint64 {
	if percentageRandom() >=0.1 {
		seq -= 1
	}else if percentageRandom()<=0.1 {
		seq +=1
	}else{
		seq = seq
	}
	return seq
}

func percentageRandom() float64 {
	return rand.Float64()
}
