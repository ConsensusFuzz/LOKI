package eth

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	fuzz "github.com/google/gofuzz"
	"math/rand"
	"time"
)

func (p *Peer) FuzzTransactions() {
	//TODO temp: send mutated tx here!

	time.Sleep(time.Duration(2)*time.Second)		//sleep 2 minutes

	p.Log().Error("Begin sending Fuzzed Transactions!!!!!!!")

	f := fuzz.New().NilChance(0.1)
	for{
		//p.Log().Warn("Sending Fuzzed Message!")
		TransactionsMsgFuzzed(p,f)
		time.Sleep(time.Duration(1)*time.Second)
	}

}


func TransactionsMsgFuzzed(p *Peer, f *fuzz.Fuzzer) error {
	if len(p.TransactionsMsgs) == 0{
		return nil
	}else{
		msg := p.TransactionsMsgs[rand.Intn(len(p.TransactionsMsgs))]
		MutateTransactions(f, msg)
		p2p.Send(p.rw, TransactionsMsg, msg)
		return nil
	}
}


func MutateTransactions(f *fuzz.Fuzzer, msg types.Transactions){
	for _, m:= range ([]*types.Transaction)(msg){
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