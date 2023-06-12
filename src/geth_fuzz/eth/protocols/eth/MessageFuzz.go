package eth

import (
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	fuzz "github.com/google/gofuzz"
	"io"
	"math/rand"
	"os"
	"time"
)



type msgFuzzer func(p *Peer, f *fuzz.Fuzzer) error

var fuzzers = map[uint64]msgFuzzer{
	NewBlockHashesMsg:             getNewBlockHashesMsgFuzzed,
	NewBlockMsg:                   getNewBlockMsgFuzzed,
	TransactionsMsg:               getTransactionsMsgFuzzed,
	NewPooledTransactionHashesMsg: getNewPooledTransactionHashesMsgFuzzed,
	GetBlockHeadersMsg:            getGetBlockHeadersMsgFuzzed,
	BlockHeadersMsg:               getBlockHeadersMsgFuzzed,
	GetBlockBodiesMsg:             getGetBlockBodiesMsgFuzzed,
	BlockBodiesMsg:                getBlockBodiesMsgFuzzed,
	GetNodeDataMsg:                getGetNodeDataMsgFuzzed,
	NodeDataMsg:                   getNodeDataMsgFuzzed,
	GetReceiptsMsg:                getGetReceiptsMsgFuzzed,
	ReceiptsMsg:                   getReceiptsMsgFuzzed,
	GetPooledTransactionsMsg:      getGetPooledTransactionsMsgFuzzed,
	PooledTransactionsMsg:         getPooledTransactionsMsgFuzzed,
}

var fuzzcount = 0
var times = 0

func checkFileIsExist(filename string) bool {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false
	}
	return true
}

func (p *Peer) recordFuzzIteration() {

	p.Log().Error("Begin recordFuzzIteration !!!!!!!")

	time.Sleep(time.Duration(300)*time.Second)		//sleep 2 minutes

	for {

		var wireteString = "time,iteration\n"
		var filename = "./fuzzIteration.txt"
		var f *os.File
		//var err1 error
		if checkFileIsExist(filename) { //如果文件存在
			f, _ = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0666) //打开文件
			var count_string = fmt.Sprintf("%d", times) + "," + fmt.Sprintf("%d", fuzzcount) + "\n"
			times += 1
			fuzzcount = 0

			if _, err := f.WriteString(count_string); err != nil {
				p.Log().Error(err.Error())
			}

			//io.WriteString(f, count_string) //写入文件(字符串)
			//fmt.Println("文件存在")

			f.Close()
			//p.Log().Error("write string !!!!!!!" + count_string)

		} else {
			f, _ = os.Create(filename) //创建文件
			io.WriteString(f, wireteString)                 //写入文件(字符串)
			//fmt.Println("文件不存在")
			f.Close()
		}


		time.Sleep(time.Duration(60) * time.Second)
	}


}

func (p *Peer) FuzzMessages() {
	//TODO temp: send mutated msg here!

	time.Sleep(time.Duration(300)*time.Second)		//sleep 2 minutes

	p.Log().Error("Begin sending Fuzzed Message!!!!!!!")

	f := fuzz.New().NilChance(0.1)

	for{
		//p.Log().Warn("Sending Fuzzed Message!")
		fuzzing := fuzzers[getNextType()]
		if fuzzing!=nil{
			fuzzing(p,f)
			fuzzcount+=1
		}
		if fuzzcount%1000 == 0 {
			time.Sleep(2*time.Second)
		}

	}

}


func getNextType() uint64{
	rand.Seed(time.Now().Unix())
	return uint64(rand.Intn(15)+1)
}



func getNewBlockHashesMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.NewBlockHashesMsgs) == 0{
		return nil
	}else{
		msg := p.NewBlockHashesMsgs[rand.Intn(len(p.NewBlockHashesMsgs))]
		MutateNewBlockHashesMsg(f, msg)
		p2p.Send(p.rw, NewBlockHashesMsg, msg)
		return nil
	}
}

func getNewBlockMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.NewBlockMsgs) == 0{
		return nil
	}else{
		msg := p.NewBlockMsgs[rand.Intn(len(p.NewBlockMsgs))]
		MutateNewBlockMsg(f, msg)
		p2p.Send(p.rw, NewBlockMsg, msg)
		return nil
	}
}

func getTransactionsMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.TransactionsMsgs) == 0{
		return nil
	}else{
		msg := p.TransactionsMsgs[rand.Intn(len(p.TransactionsMsgs))]
		MutateTransactionsMsg(f, msg)
		p2p.Send(p.rw, TransactionsMsg, msg)
		return nil
	}
}

func getNewPooledTransactionHashesMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.NewPooledTransactionHashesMsgs) == 0{
		return nil
	}else{
		msg := p.NewPooledTransactionHashesMsgs[rand.Intn(len(p.NewPooledTransactionHashesMsgs))]
		MutateNewPooledTransactionHashesMsg(f, msg)
		p2p.Send(p.rw, NewPooledTransactionHashesMsg, msg)
		return nil
	}
}

func getBlockHeadersMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.BlockHeadersMsgs) == 0{
		return nil
	}else{
		msg := p.BlockHeadersMsgs[rand.Intn(len(p.BlockHeadersMsgs))]
		MutateBlockHeadersMsg(f, msg)
		p2p.Send(p.rw, BlockHeadersMsg,msg)
		return nil
	}
}

func getGetBlockHeadersMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error{
	if len(p.GetBlockHeadersMsg) == 0{
		return nil
	}else{
		msg := p.GetBlockHeadersMsg[rand.Intn(len(p.GetBlockHeadersMsg))]
		MutateGetBlockHeaderMsg(f, msg)
		p2p.Send(p.rw, GetBlockHeadersMsg,msg)
		return nil
	}
}

func getGetBlockBodiesMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.GetBlockBodiesMsgs) == 0{
		return nil
	}else{
		msg := p.GetBlockBodiesMsgs[rand.Intn(len(p.GetBlockBodiesMsgs))]
		MutateGetBlockBodiesMsg(f, msg)
		p2p.Send(p.rw, GetBlockBodiesMsg,msg)
		return nil
	}
}

func getBlockBodiesMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.BlockBodiesMsgs) == 0 {
		return nil
	}else{
		msg:= p.BlockBodiesMsgs[rand.Intn(len(p.BlockBodiesMsgs))]
		MutateBlockBodiesMsg(f, msg)
		p2p.Send(p.rw, BlockBodiesMsg,msg)
		return nil
	}
}

func getGetNodeDataMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.GetNodeDataMsgs) == 0 {
		return nil
	}else{
		msg:= p.GetNodeDataMsgs[rand.Intn(len(p.GetNodeDataMsgs))]
		MutateGetNodeDataMsg(f, msg)
		p2p.Send(p.rw, GetNodeDataMsg,msg)
		return nil
	}
}


func getNodeDataMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.NodeDataMsgs) == 0 {
		return nil
	}else{
		msg:= p.NodeDataMsgs[rand.Intn(len(p.NodeDataMsgs))]
		MutateNodeDataMsg(f, msg)
		p2p.Send(p.rw, NodeDataMsg,msg)
		return nil
	}
}

func getGetReceiptsMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.GetReceiptsMsgs) == 0 {
		return nil
	}else{
		msg:= p.GetReceiptsMsgs[rand.Intn(len(p.GetReceiptsMsgs))]
		MutateGetReceiptsMsg(f, msg)
		p2p.Send(p.rw, GetReceiptsMsg,msg)
		return nil
	}
}

func getReceiptsMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.ReceiptsMsgs) == 0 {
		return nil
	}else{
		msg:= p.ReceiptsMsgs[rand.Intn(len(p.ReceiptsMsgs))]
		MutateReceiptsMsg(f, msg)
		p2p.Send(p.rw, ReceiptsMsg,msg)
		return nil
	}
}

func getGetPooledTransactionsMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.GetPooledTransactionsMsgs) == 0 {
		return nil
	}else{
		msg:= p.GetPooledTransactionsMsgs[rand.Intn(len(p.GetPooledTransactionsMsgs))]
		MutateGetPooledTransactionsMsg(f, msg)
		p2p.Send(p.rw, GetPooledTransactionsMsg,msg)
		return nil
	}
}

func getPooledTransactionsMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.PooledTransactionsMsgs) == 0 {
		return nil
	}else{
		msg:= p.PooledTransactionsMsgs[rand.Intn(len(p.PooledTransactionsMsgs))]
		MutatePooledTransactionsMsg(f, msg)
		p2p.Send(p.rw, PooledTransactionsMsg,msg)
		return nil
	}
}



func getStatusMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error{
	if len(p.StatusMsgs) == 0{
		return nil
	}else{
		msg:= p.StatusMsgs[rand.Intn(len(p.StatusMsgs))]
		MutateStatusMsg(f, msg)
		p2p.Send(p.rw, StatusMsg,msg)
		return nil
	}
}




// message fuzzer

func MutateNewBlockHashesMsg(f *fuzz.Fuzzer, msg *NewBlockHashesPacket) {
	f.Fuzz(&msg)
}


func MutateNewBlockMsg(f *fuzz.Fuzzer, msg *NewBlockPacket) {
	f.Fuzz(msg.TD)
	f.Fuzz(msg.Block.Header())
	//f.Fuzz(msg.Block.Nonce())
	f.Fuzz(&msg.Block.ReceivedAt)
	//TODO
}

func MutateTransactionsMsg(f *fuzz.Fuzzer, msg types.Transactions){
	for _, m:= range ([]*types.Transaction)(msg){
		//innerdata:=m.GetInnerData()

		data := m.GetData()
		tmpStr :=string(*data)
		f.Fuzz(&tmpStr)
		tmpArr := []byte(tmpStr)
		data = &tmpArr

		f.Fuzz(m.GetGas())
		f.Fuzz(m.Gasprice())
		f.Fuzz(m.GastipCap())
		f.Fuzz(m.GasfeeCap())
		f.Fuzz(m.GetValue())
		f.Fuzz(m.GetNonce())
		//TODO need more mutating operation
	}


}

func MutateNewPooledTransactionHashesMsg(f *fuzz.Fuzzer, msg *NewPooledTransactionHashesPacket) {
	//f.Fuzz(msg.)
	//TODO hashvalue, do not need to mutate
}

func MutateGetBlockHeaderMsg(f *fuzz.Fuzzer, msg *GetBlockHeadersPacket66) {
	f.Fuzz(&msg.Origin)
	f.Fuzz(&msg.Amount)
	f.Fuzz(&msg.Skip)
	f.Fuzz(&msg.Reverse)
}

func MutateBlockHeadersMsg(f *fuzz.Fuzzer, msg *BlockHeadersPacket66) {
	f.Fuzz(&msg.RequestId)
	for _, m:= range ([]*types.Header)(msg.BlockHeadersPacket){
		//f.Fuzz(m)
		f.Fuzz(&m.Bloom)
		f.Fuzz(m.Difficulty)
		f.Fuzz(m.Number)
		f.Fuzz(&m.GasLimit)
		f.Fuzz(&m.GasUsed)
		f.Fuzz(&m.Time)
		//f.Fuzz(m.Extra)
		f.Fuzz(&m.Nonce)
	}
}

func MutateGetBlockBodiesMsg(f *fuzz.Fuzzer, msg *GetBlockBodiesPacket66) {
	f.Fuzz(&msg.RequestId)
	//for _, m:= range ([]common.Hash)(msg.GetBlockBodiesPacket){
	//	f.Fuzz(&m)
	//}

}

func MutateBlockBodiesMsg(f *fuzz.Fuzzer, msg *BlockBodiesRLPPacket66) {
	f.Fuzz(&msg.RequestId)
	for _, m:= range ([]rlp.RawValue)(msg.BlockBodiesRLPPacket){
		f.Fuzz(&m)
	}
}

func MutateGetNodeDataMsg(f *fuzz.Fuzzer, msg *GetNodeDataPacket66) {
	f.Fuzz(&msg.RequestId)
	//for _, m:= range ([]common.Hash)(msg.GetNodeDataPacket){
	//	f.Fuzz(&m)
	//}
}

func MutateNodeDataMsg(f *fuzz.Fuzzer, msg *NodeDataPacket66) {
	f.Fuzz(&msg.RequestId)
	for _, m:= range ([][]byte)(msg.NodeDataPacket){
		f.Fuzz(&m)
	}
}

func MutateGetReceiptsMsg(f *fuzz.Fuzzer, msg *GetReceiptsPacket66) {
	f.Fuzz(&msg.RequestId)
	//for _, m:= range ([]common.Hash)(msg.GetReceiptsPacket){
	//	f.Fuzz(m)
	//}
}

func MutateReceiptsMsg(f *fuzz.Fuzzer, msg *ReceiptsRLPPacket66) {
	f.Fuzz(&msg.RequestId)
	for _, m:= range ([]rlp.RawValue)(msg.ReceiptsRLPPacket){
		f.Fuzz(&m)
	}
}

func MutateGetPooledTransactionsMsg(f *fuzz.Fuzzer, msg *GetPooledTransactionsPacket66) {
	f.Fuzz(&msg.RequestId)
	//for _, m:= range ([]common.Hash)(msg.GetPooledTransactionsPacket){
	//	f.Fuzz(&m)
	//}
}

func MutatePooledTransactionsMsg(f *fuzz.Fuzzer, msg *PooledTransactionsRLPPacket66) {
	f.Fuzz(&msg.RequestId)
	for _, m:= range ([]rlp.RawValue)(msg.PooledTransactionsRLPPacket){
		f.Fuzz(&m)
	}
}

func MutateStatusMsg(f *fuzz.Fuzzer, msg *StatusPacket) {
	f.Fuzz(&msg.ForkID)
	f.Fuzz(&msg.NetworkID)
	f.Fuzz(&msg.ProtocolVersion)
	f.Fuzz(msg.TD)
}













//func (p *Peer) getGetBlockHeadersMsgFuzzed(f *fuzz.Fuzzer) *GetBlockHeadersPacket66{
//	if len(p.GetBlockHeadersMsg) == 0{
//		return nil
//	}else{
//		msg := p.GetBlockHeadersMsg[rand.Intn(len(p.GetBlockHeadersMsg))]
//		MutateGetBlockHeaderMsg(f, msg)
//		return msg
//	}
//}
//
//
//
//func (p *Peer) getNewBlockMsgFuzzed(f *fuzz.Fuzzer) *NewBlockPacket{
//	if len(p.NewBlockMsgs) == 0{
//		return nil
//	}else{
//		msg := p.NewBlockMsgs[rand.Intn(len(p.NewBlockMsgs))]
//		MutateNewBlockMsg(f, msg)
//		return msg
//	}
//}
