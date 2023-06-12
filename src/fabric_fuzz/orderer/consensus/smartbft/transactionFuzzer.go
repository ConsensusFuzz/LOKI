package smartbft

import (
	smartbft "github.com/SmartBFT-Go/consensus/pkg/consensus"
	"github.com/hyperledger/fabric/common/flogging"
	"math/rand"
	"time"
)

type TransactionFuzzer struct {
	consensus        *smartbft.Consensus
	Logger           *flogging.FabricLogger
}

func NewTransactionFuzzer (cons *smartbft.Consensus) *TransactionFuzzer {

	logger := flogging.MustGetLogger("orderer.consensus.smartbft.protocolFuzzer")

	tf := &TransactionFuzzer{
		consensus: cons,
		Logger: logger,
	}
	return tf
}


var transactionSeeds [][]byte

func AddTransactionSeeds (seed []byte){
	transactionSeeds = append(transactionSeeds, seed)
}


func getNextTxSeed() []byte{
	if len(transactionSeeds) == 0{
		return nil
	}else{
		return transactionSeeds[rand.Intn(len(transactionSeeds))]
	}
}
func transactionFuzz() []byte{
	var req []byte
	req = getNextTxSeed()

	return req
	//return TransactionMessageFuzzed(req)
}

func (tf *TransactionFuzzer) RunTxFuzz(){
	time.Sleep(time.Duration(60)*time.Second)
	tf.Logger.Errorf("Begin running transaction fuzzing process!!!!!")

	c := tf.consensus.GerContoller()
	//if c.ID == c.GetLeaderID(){

	if c.ID == 1 || c.ID ==3 || c.ID == 5{

		//if c.ID == 1 || c.ID == 2 || c.ID == 3	{   //only less than 1/3 nodes can be fuzzer nodes
		for {
			//if c.ID == c.GetLeaderID(){			//TODO only leader node send fuzzed messages
			req := transactionFuzz()
			//pf.Logger.Errorf("Is next fuzzed message is nil? %b", (msg==nil))
			if req != nil {
				tf.BroadcastTransactionsFuzz(req)
			}
			//}
			//TODO
			time.Sleep(time.Duration(5)*time.Second)
		}
	}

}


func (tf *TransactionFuzzer) BroadcastTransactionsFuzz(req []byte){
	c := tf.consensus.GerContoller()

	probability := percentageRandom()

	tf.Logger.Infof("Start broadcastTransactionFuzz, probability is: %f", probability)

	if probability < 0.7 {

		//1. first strategy: send to all nodes
		for _, node := range c.NodesList {
			// Do not send to yourself
			if c.ID == node {
				continue
			}
			c.Comm.SendTransactionFuzz(node, req)
		}

	}else if probability < 0.85 {

		//2. second strategy: send to half nodes
		for i, node := range c.NodesList {
			// TODO should randomly choose nodes
			if i < 6{
				continue
			}
			c.Comm.SendTransactionFuzz(node, req)
		}


	}else{

		//3. third strategy: send to 1/3 nodes
		for i, node := range c.NodesList {
			// TODO should randomly choose nodes
			if i < 8{
				continue
			}
			c.Comm.SendTransaction(node, req)
		}

	}


}
