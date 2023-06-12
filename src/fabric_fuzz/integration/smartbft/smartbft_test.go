/*
 *
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 * /
 *
 */

package smartbft

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tedsuo/ifrit/grouper"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/integration/nwo/commands"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/orderer/smartbft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var _ = Describe("EndToEnd Smart BFT configuration test", func() {
	var (
		testDir          string
		client           *docker.Client
		network          *nwo.Network
		networkProcess   ifrit.Process
		ordererProcesses []ifrit.Process
		peerProcesses    ifrit.Process
	)

	BeforeEach(func() {
		networkProcess = nil
		ordererProcesses = nil
		peerProcesses = nil
		var err error
		testDir, err = ioutil.TempDir("", "e2e-smartbft-test")
		Expect(err).NotTo(HaveOccurred())

		client, err = docker.NewClientFromEnv()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if networkProcess != nil {
			networkProcess.Signal(syscall.SIGTERM)
			Eventually(networkProcess.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if peerProcesses != nil {
			peerProcesses.Signal(syscall.SIGTERM)
			Eventually(peerProcesses.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		if network != nil {
			network.Cleanup()
		}
		for _, ordererInstance := range ordererProcesses {
			ordererInstance.Signal(syscall.SIGTERM)
			Eventually(ordererInstance.Wait(), network.EventuallyTimeout).Should(Receive())
		}
		os.RemoveAll(testDir)
	})

	Describe("smartbft network", func() {
		It("smartbft multiple nodes stop start all nodes", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerGroupRunner, peerRunners := peerGroupRunners(network)
			peerProcesses = ifrit.Invoke(peerGroupRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())
			peer := network.Peer("Org1", "peer0")

			assertBlockReception(map[string]int{"systemchannel": 0}, network.Orderers, peer, network)
			By("check block validation policy on system channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"
			By("Creating and joining  testchannel1")
			network.CreateAndJoinChannel(network.Orderers[0], channel)

			By("Deploying chaincode")
			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
			})

			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			By("check peers are using the BFT delivery client")
			for _, peerRunner := range peerRunners {
				Eventually(peerRunner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Created BFT Delivery Client"))
			}

			By("querying the chaincode")
			sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
				ChannelID: channel,
				Name:      "mycc",
				Ctor:      `{"Args":["query","a"]}`,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
			Expect(sess).To(gbytes.Say("100"))

			By("invoking the chaincode")
			invokeQuery(network, peer, network.Orderers[1], channel, 90)

			By("Taking down all the orderers")
			for _, proc := range ordererProcesses {
				proc.Signal(syscall.SIGTERM)
				Eventually(proc.Wait(), network.EventuallyTimeout).Should(Receive())
			}

			ordererRunners = nil
			ordererProcesses = nil
			By("Bringing up all the nodes")
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Waiting for followers to see the leader, again")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=testchannel1"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=testchannel1"))

			By("invoking the chaincode, again")
			invokeQuery(network, peer, network.Orderers[2], channel, 80)
		})

		It("smartbft assisted synchronization no rotation", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer := network.Peer("Org1", "peer0")

			assertBlockReception(map[string]int{"systemchannel": 0}, network.Orderers, peer, network)
			By("check block validation policy on sys channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"

			orderer := network.Orderers[0]
			network.CreateAndJoinChannel(orderer, channel)

			assertBlockReception(map[string]int{"systemchannel": 1}, network.Orderers, peer, network)

			// Disable leader rotation in application channel
			nwo.UpdateSmartBFTMetadata(network, peer, orderer, channel, func(md *smartbft.ConfigMetadata) {
				md.Options.LeaderRotation = smartbft.Options_OFF
				md.Options.DecisionsPerLeader = 0
			})

			assertBlockReception(map[string]int{"testchannel1": 1}, network.Orderers, peer, network)

			By("Restarting all nodes")
			for i := 0; i < 4; i++ {
				orderer := network.Orderers[i]
				By(fmt.Sprintf("Killing %s", orderer.Name))
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())

				By(fmt.Sprintf("Launching %s", orderer.Name))
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners[i] = runner
				proc := ifrit.Invoke(runner)
				ordererProcesses[i] = proc
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
			})

			assertBlockReception(map[string]int{"testchannel1": 2}, network.Orderers, peer, network)

			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			By("Taking down a follower node")
			ordererProcesses[3].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[3].Wait(), network.EventuallyTimeout).Should(Receive())

			invokeQuery(network, peer, orderer, channel, 90)
			invokeQuery(network, peer, orderer, channel, 80)
			invokeQuery(network, peer, orderer, channel, 70)
			invokeQuery(network, peer, orderer, channel, 60)

			By("Bringing up the follower node")
			runner := network.OrdererRunner(network.Orderers[3])
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
			proc := ifrit.Invoke(runner)
			ordererProcesses[3] = proc
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 0, sequence 2"))

			By("Waiting communication to be established from the leader")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			invokeQuery(network, peer, orderer, channel, 50)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 40)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 30)
			time.Sleep(time.Second * 2)
			invokeQuery(network, peer, orderer, channel, 20)
			time.Sleep(time.Second * 2)

			assertBlockReception(map[string]int{"testchannel1": 10}, network.Orderers, peer, network)

			invokeQuery(network, peer, orderer, channel, 10)
			assertBlockReception(map[string]int{"testchannel1": 11}, network.Orderers, peer, network)

			invokeQuery(network, peer, orderer, channel, 0)
			assertBlockReception(map[string]int{"testchannel1": 12}, network.Orderers, peer, network)

			By("Ensuring follower participates in consensus")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deciding on seq 12"))
		})

		It("smartbft autonomous synchronization", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()
			network.EventuallyTimeout = time.Minute * 2

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer := network.Peer("Org1", "peer0")

			assertBlockReception(map[string]int{"systemchannel": 0}, network.Orderers, peer, network)
			By("check block validation policy on sys channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"

			orderer := network.Orderers[0]
			network.CreateAndJoinChannel(orderer, channel)

			assertBlockReception(map[string]int{"systemchannel": 1}, network.Orderers, peer, network)

			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
			})

			assertBlockReception(map[string]int{"testchannel1": 1}, network.Orderers, peer, network)
			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			By("Taking down a follower node")
			ordererProcesses[3].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[3].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Invoking once")
			invokeQuery(network, peer, orderer, channel, 90)
			By("Invoking twice")
			invokeQuery(network, peer, orderer, channel, 80)
			By("Waiting for a view change to occur")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to leader role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=testchannel1"))
			By("Invoking three times")
			invokeQuery(network, peer, orderer, channel, 70)
			By("Invoking four times")
			invokeQuery(network, peer, orderer, channel, 60)

			By("Bringing up the follower node")
			runner := network.OrdererRunner(network.Orderers[3])
			runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
			proc := ifrit.Invoke(runner)
			ordererProcesses[3] = proc
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 0, sequence 2"))

			By("Waiting for follower to synchronize itself")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Finished synchronizing with cluster"))

			By("Waiting for follower to understand it synced a view change")
			Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Node 4 was informed of a new view 1 channel=testchannel1"))

			By("Waiting for all nodes to have the latest block sequence")
			assertBlockReception(map[string]int{"testchannel1": 5}, network.Orderers, peer, network)

			By("Ensuring the follower is functioning properly")
			invokeQuery(network, peer, orderer, channel, 50)
			invokeQuery(network, peer, orderer, channel, 40)
			assertBlockReception(map[string]int{"testchannel1": 7}, network.Orderers, peer, network)
		})

		It("smartbft node addition and removal", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			network.EventuallyTimeout *= 2

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer := network.Peer("Org1", "peer0")

			assertBlockReception(map[string]int{"systemchannel": 0}, network.Orderers, peer, network)
			By("check block validation policy on sys channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"

			orderer := network.Orderers[0]
			network.CreateAndJoinChannel(orderer, channel)

			assertBlockReception(map[string]int{"systemchannel": 1}, network.Orderers, peer, network)

			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
			})

			assertBlockReception(map[string]int{"testchannel1": 1}, network.Orderers, peer, network)

			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			By("Transacting on testchannel1")
			invokeQuery(network, peer, orderer, channel, 90)
			invokeQuery(network, peer, orderer, channel, 80)
			assertBlockReception(map[string]int{"testchannel1": 3}, network.Orderers, peer, network)

			By("Adding a new consenter")

			orderer5 := &nwo.Orderer{
				Name:         "orderer5",
				Organization: "OrdererOrg",
			}
			network.Orderers = append(network.Orderers, orderer5)

			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer5.ID()] = ports

			network.GenerateCryptoConfig()
			network.GenerateOrdererConfig(orderer5)

			sess, err := network.Cryptogen(commands.Extend{
				Config: network.CryptoConfigPath(),
				Input:  network.CryptoPath(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderer5), "server.crt")
			ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			ordererIdentity, err := ioutil.ReadFile(network.OrdererCert(orderer5))
			Expect(err).NotTo(HaveOccurred())

			for _, channel := range []string{"systemchannel", "testchannel1"} {
				nwo.UpdateSmartBFTMetadata(network, peer, orderer, channel, func(md *smartbft.ConfigMetadata) {
					md.Consenters = append(md.Consenters, &smartbft.Consenter{
						MspId:       "OrdererMSP",
						ConsenterId: 5,
						Identity: utils.MarshalOrPanic(&msp.SerializedIdentity{
							Mspid:   "OrdererMSP",
							IdBytes: ordererIdentity,
						}),
						ServerTlsCert: ordererCertificate,
						ClientTlsCert: ordererCertificate,
						Host:          "127.0.0.1",
						Port:          uint32(network.OrdererPort(orderer5, nwo.ClusterPort)),
					})
				})
			}

			assertBlockReception(map[string]int{
				"systemchannel": 2,
				"testchannel1":  4,
			}, network.Orderers[:4], peer, network)

			restart := func(i int) {
				orderer := network.Orderers[i]
				By(fmt.Sprintf("Killing %s", orderer.Name))
				ordererProcesses[i].Signal(syscall.SIGTERM)
				Eventually(ordererProcesses[i].Wait(), network.EventuallyTimeout).Should(Receive())

				By(fmt.Sprintf("Launching %s", orderer.Name))
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=grpc=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners[i] = runner
				proc := ifrit.Invoke(runner)
				ordererProcesses[i] = proc
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))

			By("Planting last config block in the orderer's file system")
			configBlock := nwo.GetConfigBlock(network, peer, orderer, "systemchannel")
			err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), utils.MarshalOrPanic(configBlock), 0644)
			Expect(err).NotTo(HaveOccurred())

			By("Launching the added orderer")
			orderer5Runner := network.OrdererRunner(orderer5)
			orderer5Runner.Command.Env = append(orderer5Runner.Command.Env, "FABRIC_LOGGING_SPEC=grpc=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
			ordererRunners = append(ordererRunners, orderer5Runner)
			proc := ifrit.Invoke(orderer5Runner)
			ordererProcesses = append(ordererProcesses, proc)
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for the added orderer to see the leader")
			Eventually(orderer5Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))

			By("Ensure all nodes are in sync")
			assertBlockReception(map[string]int{
				"systemchannel": 2,
				"testchannel1":  4,
			}, network.Orderers, peer, network)

			By("Make sure the peers get the config blocks, again")
			waitForBlockReceptionByPeer(peer, network, "testchannel1", 4)

			By("Killing the leader orderer")
			ordererProcesses[2].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[2].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for view change to occur")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=systemchannel"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to leader role, current view: 1, current leader: 2 channel=systemchannel"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=systemchannel"))
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout*2, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=systemchannel"))

			By("Bringing the previous leader back up")
			runner := network.OrdererRunner(network.Orderers[2])
			ordererRunners[2] = runner
			proc = ifrit.Invoke(runner)
			ordererProcesses[2] = proc
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Making sure previous leader abdicates")
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Changing to follower role, current view: 1, current leader: 2 channel=systemchannel"))

			By("Making sure previous leader sees the new leader")
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 2 channel=systemchannel"))

			By("Ensure all nodes are in sync")
			assertBlockReception(map[string]int{
				"systemchannel": 2,
				"testchannel1":  4,
			}, network.Orderers, peer, network)

			By("Transacting on testchannel1 a few times")
			invokeQuery(network, peer, network.Orderers[4], channel, 70)
			invokeQuery(network, peer, network.Orderers[4], channel, 60)
			By("Making sure the previous leader synchronizes")
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Synchronized to view 0 and sequence 6 with verification sequence 2 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 0, sequence 7, and decisions 6 channel=testchannel1"))
			By("Invoking again")
			invokeQuery(network, peer, network.Orderers[4], channel, 50)
			By("Ensure all nodes are in sync")
			assertBlockReception(map[string]int{"testchannel1": 7}, network.Orderers, peer, network)

			time.Sleep(time.Second * 5)
			invokeQuery(network, peer, network.Orderers[4], channel, 40)

			By("Ensuring added node participates in consensus")
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deciding on seq 8"))

			By("Ensure all nodes are in sync, again")
			assertBlockReception(map[string]int{"testchannel1": 8}, network.Orderers, peer, network)

			By("Removing the added node from the channels")
			nwo.UpdateSmartBFTMetadata(network, peer, network.Orderers[2], "testchannel1", func(md *smartbft.ConfigMetadata) {
				md.Consenters = md.Consenters[:4]
			})
			nwo.UpdateSmartBFTMetadata(network, peer, network.Orderers[1], "systemchannel", func(md *smartbft.ConfigMetadata) {
				md.Consenters = md.Consenters[:4]
			})

			assertBlockReception(map[string]int{
				"systemchannel": 3,
				"testchannel1":  9,
			}, network.Orderers, peer, network)

			By("Restarting the removed node")
			restart(4)

			By("Waiting for the removed node to say the channel is not serviced by it")
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("channel systemchannel is not serviced by me"))

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))

			By("Make sure the peers get the config blocks, again")
			waitForBlockReceptionByPeer(peer, network, "testchannel1", 9)

			By("Transact again")
			invokeQuery(network, peer, network.Orderers[2], channel, 30)

			By("Ensuring the existing nodes got the block")
			assertBlockReception(map[string]int{
				"testchannel1": 10,
			}, network.Orderers[:4], peer, network)

			// Drain the buffer
			n := len(orderer5Runner.Err().Contents())
			orderer5Runner.Err().Read(make([]byte, n))

			By("Adding back the node to the application channel")
			nwo.UpdateSmartBFTMetadata(network, peer, network.Orderers[2], channel, func(md *smartbft.ConfigMetadata) {
				md.Consenters = append(md.Consenters, &smartbft.Consenter{
					MspId:       "OrdererMSP",
					ConsenterId: 5,
					Identity: utils.MarshalOrPanic(&msp.SerializedIdentity{
						Mspid:   "OrdererMSP",
						IdBytes: ordererIdentity,
					}),
					ServerTlsCert: ordererCertificate,
					ClientTlsCert: ordererCertificate,
					Host:          "127.0.0.1",
					Port:          uint32(network.OrdererPort(orderer5, nwo.ClusterPort)),
				})
			})

			By("Ensuring all nodes got the block that adds the consenter to the application channel")
			assertBlockReception(map[string]int{
				"testchannel1": 11,
			}, network.Orderers, peer, network)

			By("Transact again")
			invokeQuery(network, peer, network.Orderers[2], channel, 20)

			assertBlockReception(map[string]int{
				"testchannel1": 12,
			}, network.Orderers, peer, network)

			By("Transact last time")
			invokeQuery(network, peer, network.Orderers[2], channel, 10)

			assertBlockReception(map[string]int{
				"testchannel1": 13,
			}, network.Orderers, peer, network)

			By("Ensuring re-added node participates in consensus")
			Eventually(ordererRunners[4].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Deciding on seq 13 channel=testchannel1"))
		})

		It("smartbft iterated addition and iterated removal", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer := network.Peer("Org1", "peer0")

			assertBlockReception(map[string]int{"systemchannel": 0}, network.Orderers, peer, network)
			By("check block validation policy on sys channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"

			orderer := network.Orderers[0]
			network.CreateAndJoinChannel(orderer, channel)

			assertBlockReception(map[string]int{"systemchannel": 1, channel: 0}, network.Orderers, peer, network)

			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			for i := 0; i < 6; i++ {
				fmt.Fprintf(GinkgoWriter, "adding orderer %d", i+5)

				name := fmt.Sprintf("orderer%d", i+5)

				newOrderer := &nwo.Orderer{
					Name:         name,
					Organization: "OrdererOrg",
				}
				network.Orderers = append(network.Orderers, newOrderer)

				ports := nwo.Ports{}
				for _, portName := range nwo.OrdererPortNames() {
					ports[portName] = network.ReservePort()
				}
				network.PortsByOrdererID[newOrderer.ID()] = ports

				network.GenerateCryptoConfig()
				network.GenerateOrdererConfig(newOrderer)

				sess, err := network.Cryptogen(commands.Extend{
					Config: network.CryptoConfigPath(),
					Input:  network.CryptoPath(),
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

				ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(newOrderer), "server.crt")
				ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
				Expect(err).NotTo(HaveOccurred())

				ordererIdentity, err := ioutil.ReadFile(network.OrdererCert(newOrderer))
				Expect(err).NotTo(HaveOccurred())

				for _, channel := range []string{"systemchannel", "testchannel1"} {
					nwo.UpdateSmartBFTMetadata(network, peer, orderer, channel, func(md *smartbft.ConfigMetadata) {
						md.Consenters = append(md.Consenters, &smartbft.Consenter{
							MspId:       "OrdererMSP",
							ConsenterId: uint64(5 + i),
							Identity: utils.MarshalOrPanic(&msp.SerializedIdentity{
								Mspid:   "OrdererMSP",
								IdBytes: ordererIdentity,
							}),
							ServerTlsCert: ordererCertificate,
							ClientTlsCert: ordererCertificate,
							Host:          "127.0.0.1",
							Port:          uint32(network.OrdererPort(newOrderer, nwo.ClusterPort)),
						})
					})
				}

				assertBlockReception(map[string]int{
					"systemchannel": 2 + i,
					"testchannel1":  1 + i,
				}, network.Orderers[:4], peer, network)

				By("Planting last config block in the orderer's file system")
				configBlock := nwo.GetConfigBlock(network, peer, orderer, "systemchannel")
				err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), utils.MarshalOrPanic(configBlock), 0644)
				Expect(err).NotTo(HaveOccurred())

				fmt.Fprintf(GinkgoWriter, "Launching orderer %d", 5+i)
				runner := network.OrdererRunner(newOrderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererRunners = append(ordererRunners, runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

				By("Waiting for the added orderer to see the leader")
				sysChanBlockHeight := 2 + i
				n := 4 + i + 1
				leaderIndex := sysChanBlockHeight % n
				Eventually(runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(fmt.Sprintf("Message from %d", leaderIndex+1)))

				By("Ensure all orderers are in sync")
				assertBlockReception(map[string]int{
					"systemchannel": 2 + i,
					"testchannel1":  1 + i,
				}, network.Orderers, peer, network)

				By(fmt.Sprintf("Make sure the peers are up to date with the orderers and have height of %d", 1+i))
				waitForBlockReceptionByPeer(peer, network, "testchannel1", uint64(1+i))

			} // for loop that adds orderers

			lastOrdererRunner := ordererRunners[len(ordererRunners)-1]
			lastOrderer := network.Orderers[len(network.Orderers)-1]
			// Put the endpoint of the last 4 orderers instead of the first 4
			var lastOrdererEndpoints []string
			for i := 1; i <= 4; i++ {
				o := network.Orderers[len(network.Orderers)-i]
				ordererEndpoint := fmt.Sprintf("127.0.0.1:%d", network.OrdererPort(o, nwo.ListenPort))
				lastOrdererEndpoints = append(lastOrdererEndpoints, ordererEndpoint)
			}

			By(fmt.Sprintf("Updating the addresses of the orderers to be %s", lastOrdererEndpoints))
			nwo.UpdateOrdererEndpoints(network, peer, lastOrderer, channel, lastOrdererEndpoints...)

			By("Shrinking the cluster back")
			for i := 0; i < 6; i++ {
				By(fmt.Sprintf("Waiting for the added orderer to see the leader %d", i+1))
				Eventually(lastOrdererRunner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say(fmt.Sprintf("Message from %d", 1+i)))
				By(fmt.Sprintf("Removing the added node from the application channel (block %d)", 8+i))
				nwo.UpdateSmartBFTMetadata(network, peer, lastOrderer, channel, func(md *smartbft.ConfigMetadata) {
					md.Consenters = md.Consenters[1:]
				})

				assertBlockReception(map[string]int{
					"systemchannel": 7,
					"testchannel1":  8 + i,
				}, network.Orderers[i+1:], peer, network)

				By(fmt.Sprintf("Make sure the peers are up to date with the orderers and have height of %d", 8+i))
				waitForBlockReceptionByPeer(peer, network, channel, uint64(8+i))
			}
		})

		It("smartbft multiple nodes view change", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			peer := network.Peer("Org1", "peer0")

			channel := "testchannel1"

			network.CreateAndJoinChannel(network.Orderers[0], channel)

			assertBlockReception(map[string]int{"systemchannel": 1}, network.Orderers, peer, network)

			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
			})

			assertBlockReception(map[string]int{"testchannel1": 1}, network.Orderers, peer, network)

			By("check block validation policy on sys channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)
			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			By("Taking down the leader node")
			ordererProcesses[1].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[1].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Submitting a request to all followers to force a view change")

			endpoints := fmt.Sprintf("%s,%s,%s",
				network.OrdererAddress(network.Orderers[0], nwo.ListenPort),
				network.OrdererAddress(network.Orderers[2], nwo.ListenPort),
				network.OrdererAddress(network.Orderers[3], nwo.ListenPort))

			sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
				ChannelID: channel,
				Orderer:   endpoints,
				Name:      "mycc",
				Ctor:      `{"Args":["issue","x1","100"]}`,
				PeerAddresses: []string{
					network.PeerAddress(network.Peer("Org1", "peer0"), nwo.ListenPort),
					network.PeerAddress(network.Peer("Org2", "peer1"), nwo.ListenPort),
				},
				WaitForEvent: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			By("Waiting for view change to occur")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("ViewChanged, the new view is 2"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("ViewChanged, the new view is 2"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("ViewChanged, the new view is 2"))

			By("Waiting for circulating transaction to be re-proposed")
			queryExpect(network, peer, channel, "x1", 100)

		})

		It("smartbft reconfiguration prevents blacklisting", func() {
			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)
			network.GenerateConfigTree()
			network.Bootstrap()

			network.EventuallyTimeout *= 2

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.common.cluster=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerRunner := network.PeerGroupRunner()
			peerProcesses = ifrit.Invoke(peerRunner)

			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			peer := network.Peer("Org1", "peer0")

			assertBlockReception(map[string]int{"systemchannel": 0}, network.Orderers, peer, network)
			By("check block validation policy on sys channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], "systemchannel", common.Policy_IMPLICIT_ORDERER)

			By("Waiting for followers to see the leader")
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 1"))

			channel := "testchannel1"

			orderer := network.Orderers[0]
			network.CreateAndJoinChannel(orderer, channel)

			assertBlockReception(map[string]int{"systemchannel": 1}, network.Orderers, peer, network)

			nwo.DeployChaincode(network, channel, network.Orderers[0], nwo.Chaincode{
				Name:    "mycc",
				Version: "0.0",
				Path:    "github.com/hyperledger/fabric/integration/chaincode/simple/cmd",
				Ctor:    `{"Args":["init","a","100","b","200"]}`,
				Policy:  `AND ('Org1MSP.member','Org2MSP.member')`,
			})

			assertBlockReception(map[string]int{"testchannel1": 1}, network.Orderers, peer, network)

			By("check block validation policy on app channel")
			assertBlockValidationPolicy(network, peer, network.Orderers[0], channel, common.Policy_IMPLICIT_ORDERER)

			By("Transacting on testchannel1")
			invokeQuery(network, peer, orderer, channel, 90)
			invokeQuery(network, peer, orderer, channel, 80)
			assertBlockReception(map[string]int{"testchannel1": 3}, network.Orderers, peer, network)

			By("Adding a new consenter")

			orderer5 := &nwo.Orderer{
				Name:         "orderer5",
				Organization: "OrdererOrg",
			}
			network.Orderers = append(network.Orderers, orderer5)

			ports := nwo.Ports{}
			for _, portName := range nwo.OrdererPortNames() {
				ports[portName] = network.ReservePort()
			}
			network.PortsByOrdererID[orderer5.ID()] = ports

			network.GenerateCryptoConfig()
			network.GenerateOrdererConfig(orderer5)

			sess, err := network.Cryptogen(commands.Extend{
				Config: network.CryptoConfigPath(),
				Input:  network.CryptoPath(),
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))

			ordererCertificatePath := filepath.Join(network.OrdererLocalTLSDir(orderer5), "server.crt")
			ordererCertificate, err := ioutil.ReadFile(ordererCertificatePath)
			Expect(err).NotTo(HaveOccurred())

			ordererIdentity, err := ioutil.ReadFile(network.OrdererCert(orderer5))
			Expect(err).NotTo(HaveOccurred())

			for _, channel := range []string{"systemchannel", "testchannel1"} {
				nwo.UpdateSmartBFTMetadata(network, peer, orderer, channel, func(md *smartbft.ConfigMetadata) {
					md.Consenters = append(md.Consenters, &smartbft.Consenter{
						MspId:       "OrdererMSP",
						ConsenterId: 5,
						Identity: utils.MarshalOrPanic(&msp.SerializedIdentity{
							Mspid:   "OrdererMSP",
							IdBytes: ordererIdentity,
						}),
						ServerTlsCert: ordererCertificate,
						ClientTlsCert: ordererCertificate,
						Host:          "127.0.0.1",
						Port:          uint32(network.OrdererPort(orderer5, nwo.ClusterPort)),
					})
				})
			}

			assertBlockReception(map[string]int{
				"systemchannel": 2,
				"testchannel1":  4,
			}, network.Orderers[:4], peer, network)

			By("Waiting for followers to see the leader of the system channel")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))

			By("Planting last config block in the orderer's file system")
			configBlock := nwo.GetConfigBlock(network, peer, orderer, "systemchannel")
			err = ioutil.WriteFile(filepath.Join(testDir, "systemchannel_block.pb"), utils.MarshalOrPanic(configBlock), 0644)
			Expect(err).NotTo(HaveOccurred())

			By("Launching the added orderer")
			orderer5Runner := network.OrdererRunner(orderer5)
			orderer5Runner.Command.Env = append(orderer5Runner.Command.Env, "FABRIC_LOGGING_SPEC=grpc=debug:orderer.consensus.smartbft=debug:policies.ImplicitOrderer=debug")
			ordererRunners = append(ordererRunners, orderer5Runner)
			proc := ifrit.Invoke(orderer5Runner)
			ordererProcesses = append(ordererProcesses, proc)
			Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())

			By("Waiting for the added orderer to see the leader")
			Eventually(orderer5Runner.Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 3 channel=systemchannel"))

			By("Waiting for followers to see the leader of the application channel")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 5 channel=testchannel1"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 5 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 5 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Message from 5 channel=testchannel1"))

			By("Killing the leader")
			orderer = network.Orderers[4]
			By(fmt.Sprintf("Killing %s", orderer.Name))
			ordererProcesses[4].Signal(syscall.SIGTERM)
			Eventually(ordererProcesses[4].Wait(), network.EventuallyTimeout).Should(Receive())

			By("Waiting for view change to occur")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 1, sequence 5, and decisions 0 channel=testchannel1"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 1, sequence 5, and decisions 0 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 1, sequence 5, and decisions 0 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Starting view with number 1, sequence 5, and decisions 0 channel=testchannel1"))

			orderer = network.Orderers[1]

			By("Transacting")
			invokeQuery(network, peer, orderer, channel, 70)

			By("Ensuring blacklisting is skipped due to reconfig")
			Eventually(ordererRunners[0].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commits due to verification sequence advancing from 1 to 2 channel=testchannel1"))
			Eventually(ordererRunners[1].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commits due to verification sequence advancing from 1 to 2 channel=testchannel1"))
			Eventually(ordererRunners[2].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commits due to verification sequence advancing from 1 to 2 channel=testchannel1"))
			Eventually(ordererRunners[3].Err(), network.EventuallyTimeout, time.Second).Should(gbytes.Say("Skipping verifying prev commits due to verification sequence advancing from 1 to 2 channel=testchannel1"))
		})

		It("smartbft upgrade BFT-2 to BFT-3", func() {
			wd, err := os.Getwd()
			Expect(err).To(Not(HaveOccurred()))

			archive, err := ioutil.ReadFile(filepath.Join(wd, "bft-pre-upgrade.tar.gz"))
			Expect(err).To(Not(HaveOccurred()))

			path := filepath.Join("/tmp", "e2e-smartbft-test254466452")
			if _, err := os.Stat(path); err == nil {
				os.RemoveAll(path)
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				err = os.Mkdir(path, 0755)
				Expect(err).To(Not(HaveOccurred()))
			}
			testDir = path

			err = extractTarGZ(archive, path)
			Expect(err).ToNot(HaveOccurred())

			network = nwo.New(nwo.MultiNodeSmartBFT(), testDir, client, StartPort(), components)

			var ordererRunners []*ginkgomon.Runner
			for _, orderer := range network.Orderers {
				runner := network.OrdererRunner(orderer)
				runner.Command.Env = append(runner.Command.Env, "FABRIC_LOGGING_SPEC=orderer.consensus.smartbft=debug:grpc=debug")
				ordererRunners = append(ordererRunners, runner)
				proc := ifrit.Invoke(runner)
				ordererProcesses = append(ordererProcesses, proc)
				Eventually(proc.Ready(), network.EventuallyTimeout).Should(BeClosed())
			}

			peerGroupRunner, _ := peerGroupRunners(network)
			peerProcesses = ifrit.Invoke(peerGroupRunner)
			Eventually(peerProcesses.Ready(), network.EventuallyTimeout).Should(BeClosed())

			network.PortsByOrdererID[network.Orderers[3].ID()] = map[nwo.PortName]uint16{
				nwo.ListenPort: 39012,
			}
			peer := network.Peer("Org1", "peer0")
			invokeQuery(network, peer, network.Orderers[3], "testchannel1", 70)
			invokeQuery(network, peer, network.Orderers[3], "testchannel1", 60)
			invokeQuery(network, peer, network.Orderers[3], "testchannel1", 50)
		})
	})
})

func invokeQuery(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, expectedBalance int) {
	sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: channel,
		Orderer:   network.OrdererAddress(orderer, nwo.ListenPort),
		Name:      "mycc",
		Ctor:      `{"Args":["invoke","a","b","10"]}`,
		PeerAddresses: []string{
			network.PeerAddress(network.Peer("Org1", "peer0"), nwo.ListenPort),
			network.PeerAddress(network.Peer("Org2", "peer1"), nwo.ListenPort),
		},
		WaitForEvent: true,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	queryExpect(network, peer, channel, "a", expectedBalance)
}

func queryExpect(network *nwo.Network, peer *nwo.Peer, channel string, key string, expectedBalance int) {
	Eventually(func() string {
		sess, err := network.PeerUserSession(peer, "User1", commands.ChaincodeQuery{
			ChannelID: channel,
			Name:      "mycc",
			Ctor:      fmt.Sprintf(`{"Args":["query","%s"]}`, key),
		})
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d: %s, %v", sess.ExitCode(), string(sess.Err.Contents()), err)
		}

		outStr := strings.TrimSpace(string(sess.Out.Contents()))
		if outStr != fmt.Sprintf("%d", expectedBalance) {
			return fmt.Sprintf("Error: expected: %d, received %s", expectedBalance, outStr)
		}
		return ""
	}, network.EventuallyTimeout, time.Second).Should(BeEmpty())
}

// assertBlockReception asserts that the given orderers have expected heights for the given channel--> height mapping
func assertBlockReception(expectedSequencesPerChannel map[string]int, orderers []*nwo.Orderer, p *nwo.Peer, n *nwo.Network) {
	defer GinkgoRecover()
	assertReception := func(channelName string, blockSeq int) {
		for _, orderer := range orderers {
			waitForBlockReception(orderer, p, n, channelName, blockSeq)
		}
	}

	for channelName, blockSeq := range expectedSequencesPerChannel {
		assertReception(channelName, blockSeq)
	}
}

func waitForBlockReception(o *nwo.Orderer, submitter *nwo.Peer, network *nwo.Network, channelName string, blockSeq int) {
	c := commands.ChannelFetch{
		ChannelID:  channelName,
		Block:      "newest",
		OutputFile: "/dev/null",
		Orderer:    network.OrdererAddress(o, nwo.ListenPort),
	}
	Eventually(func() string {
		sess, err := network.OrdererAdminSession(o, submitter, c)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, network.EventuallyTimeout).Should(gexec.Exit())
		if sess.ExitCode() != 0 {
			return fmt.Sprintf("exit code is %d: %s", sess.ExitCode(), string(sess.Err.Contents()))
		}
		sessErr := string(sess.Err.Contents())
		expected := fmt.Sprintf("Received block: %d", blockSeq)
		if strings.Contains(sessErr, expected) {
			return ""
		}
		return sessErr
	}, network.EventuallyTimeout, time.Second).Should(BeEmpty())
}

func waitForBlockReceptionByPeer(peer *nwo.Peer, network *nwo.Network, channelName string, blockSeq uint64) {
	Eventually(func() bool {
		blockNumFromPeer := nwo.CurrentConfigBlockNumber(network, peer, nil, channelName)
		return blockNumFromPeer == blockSeq
	}, network.EventuallyTimeout, time.Second).Should(BeTrue())
}

func assertBlockValidationPolicy(network *nwo.Network, peer *nwo.Peer, orderer *nwo.Orderer, channel string, policyType common.Policy_PolicyType) {
	config := nwo.GetConfig(network, peer, orderer, channel)
	blockValidationPolicyValue, ok := config.ChannelGroup.Groups["Orderer"].Policies["BlockValidation"]
	Expect(ok).To(BeTrue())
	Expect(common.Policy_PolicyType(blockValidationPolicyValue.Policy.Type)).To(Equal(policyType))
}

func peerGroupRunners(n *nwo.Network) (ifrit.Runner, []*ginkgomon.Runner) {
	runners := []*ginkgomon.Runner{}
	members := grouper.Members{}
	for _, p := range n.Peers {
		runner := n.PeerRunner(p)
		members = append(members, grouper.Member{Name: p.ID(), Runner: runner})
		runners = append(runners, runner)
	}
	return grouper.NewParallel(syscall.SIGTERM, members), runners
}

func extractTarGZ(archive []byte, baseDir string) error {
	gzReader, err := gzip.NewReader(bytes.NewBuffer(archive))
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		filePath := filepath.Join(baseDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(filePath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			fd, err := os.Create(filePath)
			if err != nil {
				return err
			}
			_, err = io.Copy(fd, tarReader)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
