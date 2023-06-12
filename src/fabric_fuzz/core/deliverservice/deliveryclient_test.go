/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/deliverservice/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	common2 "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type connectionProducer interface {
	comm.ConnectionProducer
}

func init() {
	msptesttools.LoadMSPSetupForTesting()
}

const (
	goRoutineTestWaitTimeout = time.Second * 15
)

var (
	lock = sync.Mutex{}
)

type mockBlocksDelivererFactory struct {
	mockCreate func() (blocksprovider.BlocksDeliverer, error)
}

func (mock *mockBlocksDelivererFactory) Create() (blocksprovider.BlocksDeliverer, error) {
	return mock.mockCreate()
}

type mockMCS struct {
}

func (*mockMCS) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

func (*mockMCS) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType("pkiID")
}

func (*mockMCS) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
	return nil
}

func (*mockMCS) VerifyHeader(chainID string, signedBlock *common2.Block) error {
	return nil
}

func (*mockMCS) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*mockMCS) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (*mockMCS) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

func TestNewDeliverService(t *testing.T) {
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			flogging.ActivateSpec("bftDeliveryClient=DEBUG")

			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			defer viper.Reset()
			defer ensureNoGoroutineLeak(t)()

			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64, 1)}
			factory := &struct{ mockBlocksDelivererFactory }{}

			blocksDeliverer := &mocks.MockBlocksDeliverer{}
			blocksDeliverer.MockRecv = mocks.MockRecv

			factory.mockCreate = func() (blocksprovider.BlocksDeliverer, error) {
				return blocksDeliverer, nil
			}
			abcf := func(*grpc.ClientConn) orderer.AtomicBroadcastClient {
				return &mocks.MockAtomicBroadcastClient{
					BD: blocksDeliverer,
				}
			}

			connFactory := func(string, map[string]*comm.OrdererEndpoint) func(comm.EndpointCriteria) (*grpc.ClientConn, error) {
				return func(endpoint comm.EndpointCriteria) (*grpc.ClientConn, error) {
					lock.Lock()
					defer lock.Unlock()
					return newConnection(), nil
				}
			}
			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  abcf,
				ConnFactory: connFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"a"},
				},
			})
			assert.NoError(t, err)
			assert.NoError(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{Height: 0}, func() {}))

			// Lets start deliver twice
			assert.Error(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{Height: 0}, func() {}), "can't start delivery")
			// Lets stop deliver that not started
			assert.Error(t, service.StopDeliverForChannel("TEST_CHAINID2"), "can't stop delivery")

			// Let it try to simulate a few recv -> gossip rounds
			time.Sleep(time.Second)
			assert.NoError(t, service.StopDeliverForChannel("TEST_CHAINID"))
			time.Sleep(time.Duration(10) * time.Millisecond)
			// Make sure to stop all blocks providers
			service.Stop()
			time.Sleep(time.Duration(500) * time.Millisecond)
			connWG.Wait()

			assertBlockDissemination(0, gossipServiceAdapter.GossipBlockDisseminations, t)
			assert.Equal(t, blocksDeliverer.RecvCount(), gossipServiceAdapter.AddPayloadCount())

			assert.Error(t, service.StartDeliverForChannel("TEST_CHAINID", &mocks.MockLedgerInfo{Height: 0}, func() {}), "Delivery service is stopping")
			assert.Error(t, service.StopDeliverForChannel("TEST_CHAINID"), "Delivery service is stopping")
		})
	}
}

func TestDeliverServiceRestart(t *testing.T) {
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			flogging.ActivateSpec("bftDeliveryClient=DEBUG")
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			defer viper.Reset()

			defer ensureNoGoroutineLeak(t)()
			// Scenario: bring up ordering service instance, then shut it down, and then resurrect it.
			// Client is expected to reconnect to it, and to ask for a block sequence that is the next block
			// after the last block it got from the previous incarnation of the ordering service.

			os := mocks.NewOrderer(5611, t)

			time.Sleep(time.Second)
			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"localhost:5611"},
				},
			})
			assert.NoError(t, err)

			li := &mocks.MockLedgerInfo{Height: uint64(100)}
			os.SetNextExpectedSeek(uint64(100))

			err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
			assert.NoError(t, err, "can't start delivery")
			// Check that delivery client requests blocks in order
			go os.SendBlock(uint64(100))
			assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
			go os.SendBlock(uint64(101))
			assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
			go os.SendBlock(uint64(102))
			assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
			os.Shutdown()
			time.Sleep(time.Second * 3)
			os = mocks.NewOrderer(5611, t)
			li.Set(103)
			os.SetNextExpectedSeek(uint64(103))
			go os.SendBlock(uint64(103))
			assertBlockDissemination(103, gossipServiceAdapter.GossipBlockDisseminations, t)
			service.Stop()
			os.Shutdown()
		})
	}
}

func TestDeliverServiceFailover(t *testing.T) {
	// Scenario: bring up 2 ordering service instances,
	// and shut down the instance that the client has connected to.
	// Client is expected to connect to the other instance, and to ask for a block sequence that is the next block
	// after the last block it got from the ordering service that was shut down.
	// Then, shut down the other node, and bring back the first (that was shut down first).
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			viper.Set("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", time.Second)
			viper.Set("peer.deliveryclient.connTimeout", 100*time.Millisecond)
			defer viper.Reset()
			defer ensureNoGoroutineLeak(t)()

			os1 := mocks.NewOrderer(5612, t)
			os2 := mocks.NewOrderer(5613, t)

			time.Sleep(time.Second)
			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"localhost:5612", "localhost:5613"},
				},
			})
			assert.NoError(t, err)
			li := &mocks.MockLedgerInfo{Height: uint64(100)}
			os1.SetNextExpectedSeek(uint64(100))
			os2.SetNextExpectedSeek(uint64(100))

			err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
			assert.NoError(t, err, "can't start delivery")

			// We need to discover to which instance the client connected to
			inst, err := detectOSNConnections(isBFT, os1, os2)
			assert.NoError(t, err, "no connections")
			logger.Infof("TEST: first OSN : %s", inst[0].Addr().String())
			logger.Infof("TEST: second OSN: %s", inst[1].Addr().String())

			blockEP := service.GetEndpoint("TEST_CHAINID")
			logger.Infof("TEST: block receiver #1 endpoint=%s", blockEP)

			var instance2fail, instance2failSecond *mocks.Orderer
			var reincarnatedNodePort int
			switch blockEP {
			case "localhost:5612":
				instance2fail = os1
				instance2failSecond = os2
				reincarnatedNodePort = 5612
			case "localhost:5613":
				instance2fail = os2
				instance2failSecond = os1
				reincarnatedNodePort = 5613
			}

			assert.Equal(t, inst[0].Addr().String(), instance2fail.Addr().String())
			assert.Equal(t, inst[1].Addr().String(), instance2failSecond.Addr().String())

			go instance2fail.SendBlock(uint64(100))
			assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)

			li.Set(101)
			os1.SetNextExpectedSeek(101)
			os2.SetNextExpectedSeek(101)

			// Fail the orderer node the client is connected to
			instance2fail.Shutdown()
			time.Sleep(4 * time.Second)
			blockEP2 := service.GetEndpoint("TEST_CHAINID")
			assert.NotEqual(t, blockEP, blockEP2)
			logger.Infof("TEST: block receiver #2 endpoint=%s", blockEP2)

			// Ensure the client asks blocks from the other ordering service node
			go instance2failSecond.SendBlock(101)
			assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
			li.Set(102)

			// Now shut down the 2nd node
			instance2failSecond.Shutdown()
			time.Sleep(1 * time.Second)
			// Bring up the first one
			os := mocks.NewOrderer(reincarnatedNodePort, t)
			os.SetNextExpectedSeek(102)
			time.Sleep(4 * time.Second)

			blockEP3 := service.GetEndpoint("TEST_CHAINID")
			assert.NotEqual(t, blockEP3, blockEP2)
			logger.Infof("TEST: block receiver #3 endpoint=%s", blockEP3)
			go os.SendBlock(uint64(102))
			assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)

			os.Shutdown()
			service.Stop()
		})
	}
}

func TestDeliverServiceUpdateEndpoints(t *testing.T) {
	endpointUpdater := &mocks.EndpointUpdater{}
	testChannel := "test_channel"

	var ds = &deliverServiceImpl{
		connConfig: ConnectionCriteria{
			Organizations: []string{"org1"},
			OrdererEndpointsByOrg: map[string][]string{
				"org1": {"localhost:5612"},
			},
			OrdererEndpointOverrides: map[string]*comm.OrdererEndpoint{
				"localhost:5612": {
					Address: "localhost:5614",
				},
			},
		},
		deliverClients: map[string]*deliverClient{
			testChannel: {
				bclient: endpointUpdater, // can be either a broadcastClient or a bftDeliveryClient
			},
		},
	}
	expectedEC := []comm.EndpointCriteria{
		{Organizations: []string{"org1"}, Endpoint: "localhost:5614"},
		{Organizations: []string{"org2"}, Endpoint: "localhost:5613"},
	}
	endpointUpdater.On("UpdateEndpoints", expectedEC)

	err := ds.UpdateEndpoints(
		testChannel,
		ConnectionCriteria{
			Organizations: []string{"org1", "org2"},
			OrdererEndpointsByOrg: map[string][]string{
				"org1": {"localhost:5612"},
				"org2": {"localhost:5613"},
			},
		},
	)
	assert.NoError(t, err)

	err = ds.UpdateEndpoints(
		"bogus-channel",
		ConnectionCriteria{
			Organizations: []string{"org1", "org2"},
			OrdererEndpointsByOrg: map[string][]string{
				"org1": {"localhost:5612"},
				"org2": {"localhost:5613"},
			},
		},
	)
	assert.EqualError(t, err, "Channel with bogus-channel id was not found")
}

func TestDeliverServiceServiceUnavailable(t *testing.T) {
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			flogging.ActivateSpec("bftDeliveryClient=DEBUG")
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			viper.Set("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", time.Second)
			viper.Set("peer.deliveryclient.connTimeout", 100*time.Millisecond)

			defer viper.Reset()

			orgEndpointDisableInterval := comm.EndpointDisableInterval
			comm.EndpointDisableInterval = time.Millisecond * 1500
			defer func() { comm.EndpointDisableInterval = orgEndpointDisableInterval }()
			defer ensureNoGoroutineLeak(t)()
			// Scenario: bring up 2 ordering service instances,
			// Make the instance the client connects to fail after a delivery of a block and send SERVICE_UNAVAILABLE
			// whenever subsequent seeks are sent to it.
			// The client is expected to connect to the other instance, and to ask for a block sequence that is the next block
			// after the last block it got from the first ordering service node.
			// Wait endpoint disable interval
			// After that resurrect failed node (first node) and fail instance client currently connect - send SERVICE_UNAVAILABLE
			// The client should reconnect to original instance and ask for next block.

			os1 := mocks.NewOrderer(5615, t)
			os2 := mocks.NewOrderer(5616, t)

			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"localhost:5615", "localhost:5616"},
				},
			})
			assert.NoError(t, err)
			var base uint64 = 100
			li := &mocks.MockLedgerInfo{Height: base}
			os1.SetNextExpectedSeek(base)
			os2.SetNextExpectedSeek(base)

			err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
			assert.NoError(t, err, "can't start delivery")

			inst, err := detectOSNConnections(isBFT, os1, os2)
			assert.NoError(t, err, "no connections")
			activeInstance, backupInstance := inst[0], inst[1]

			logger.Infof("TEST: first OSN: %s", activeInstance.Addr().String())
			logger.Infof("TEST: second OSN: %s", backupInstance.Addr().String())

			blockEP := service.GetEndpoint("TEST_CHAINID")
			logger.Infof("TEST: block receiver #1 endpoint=%s", blockEP)

			assert.NotNil(t, activeInstance)
			assert.NotNil(t, backupInstance)

			// Send first block
			go activeInstance.SendBlock(base)

			assertBlockDissemination(base, gossipServiceAdapter.GossipBlockDisseminations, t)
			li.Set(base + 1)

			// Backup instance should expect a seek of 101 since we got 100
			backupInstance.SetNextExpectedSeek(base + 1)

			// Fail instance delivery client connected to
			activeInstance.Fail()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			wg := sync.WaitGroup{}
			wg.Add(1)

			go func(ctx context.Context) {
				defer wg.Done()
				for {
					select {
					case <-time.After(time.Millisecond * 100):
						cCount, cType := backupInstance.ConnCountType()
						if cCount > 0 && cType == orderer.SeekInfo_BLOCK {
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}(ctx)

			wg.Wait()
			assert.NoError(t, ctx.Err(), "Delivery client has not failed over to alive ordering service")
			// Check that delivery client was indeed connected
			assert.Equal(t, backupInstance.ConnCount(), 1)
			blockEP2 := service.GetEndpoint("TEST_CHAINID")
			logger.Infof("TEST: block receiver #2 endpoint=%s", blockEP2)
			assert.NotEqual(t, blockEP, blockEP2)
			// Have backup instance prepare to send a block
			backupInstance.SendBlock(base + 1)
			// Ensure the client asks blocks from the other ordering service node
			assertBlockDissemination(base+1, gossipServiceAdapter.GossipBlockDisseminations, t)

			// Wait until first endpoint enabled again
			time.Sleep(time.Millisecond * 1600)

			li.Set(base + 2)
			activeInstance.Resurrect()
			backupInstance.Fail()

			resurrectCtx, resCancel := context.WithTimeout(context.Background(), time.Second)
			defer resCancel()

			go func() {
				// Resurrected instance should expect a seek of 102 since we got 101
				activeInstance.SetNextExpectedSeek(base + 2)
				// Have resurrected instance prepare to send a block
				activeInstance.SendBlock(base + 2)

			}()

			reswg := sync.WaitGroup{}
			reswg.Add(1)

			go func() {
				defer reswg.Done()
				for {
					select {
					case <-time.After(time.Millisecond * 100):
						if activeInstance.ConnCount() > 0 {
							return
						}
					case <-resurrectCtx.Done():
						return
					}
				}
			}()

			reswg.Wait()

			assert.NoError(t, resurrectCtx.Err(), "Delivery client has not failed over to alive ordering service")
			// Check that delivery client was indeed connected
			assert.Equal(t, activeInstance.ConnCount(), 1)
			blockEP3 := service.GetEndpoint("TEST_CHAINID")
			logger.Infof("TEST: block receiver #3 endpoint=%s", blockEP3)
			assert.Equal(t, blockEP, blockEP3)
			// Ensure the client asks blocks from the other ordering service node
			assertBlockDissemination(base+2, gossipServiceAdapter.GossipBlockDisseminations, t)

			// Cleanup
			os1.Shutdown()
			os2.Shutdown()
			service.Stop()
		})
	}
}

func TestDeliverServiceAbruptStop(t *testing.T) {
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			defer viper.Reset()
			defer ensureNoGoroutineLeak(t)()
			// Scenario: The deliver service is started and abruptly stopped.
			// The block provider instance is run in a separate goroutine, and thus
			// it might be scheduled after the deliver client is stopped.
			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}
			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"a"},
				},
			})
			assert.NoError(t, err)

			li := &mocks.MockLedgerInfo{Height: uint64(100)}
			service.StartDeliverForChannel("mychannel", li, func() {})
			service.StopDeliverForChannel("mychannel")
		})
	}
}

func TestDeliverServiceShutdown(t *testing.T) {
	// Scenario: Launch an ordering service node and let the client pull some blocks.
	// Then, shut down the client, and check that it is no longer fetching blocks.
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			defer viper.Reset()
			defer ensureNoGoroutineLeak(t)()

			os := mocks.NewOrderer(5614, t)

			time.Sleep(time.Second)
			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"localhost:5614"},
				},
			})
			assert.NoError(t, err)

			li := &mocks.MockLedgerInfo{Height: uint64(100)}
			os.SetNextExpectedSeek(uint64(100))
			err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
			assert.NoError(t, err, "can't start delivery")

			// Check that delivery service requests blocks in order
			go os.SendBlock(uint64(100))
			assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
			go os.SendBlock(uint64(101))
			assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
			li.Set(102)
			os.SetNextExpectedSeek(102)
			// Now stop the delivery service and make sure we don't disseminate a block
			service.Stop()
			go os.SendBlock(102)
			select {
			case <-gossipServiceAdapter.GossipBlockDisseminations:
				assert.Fail(t, "Disseminated a block after shutting down the delivery service")
			case <-time.After(time.Second * 2):
			}
			os.Shutdown()
			time.Sleep(time.Second)
		})
	}
}

func TestDeliverServiceShutdownRespawn(t *testing.T) {
	// Scenario: Launch an ordering service node and let the client pull some blocks.
	// Then, wait a few seconds, and don't send any blocks.
	// Afterwards - start a new instance and shut down the old instance.
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			flogging.ActivateSpec("bftDeliveryClient=DEBUG")
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			viper.Set("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", time.Second)
			viper.Set("peer.deliveryclient.connTimeout", 100*time.Millisecond)
			viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", 5*time.Second)
			defer viper.Reset()
			defer ensureNoGoroutineLeak(t)()

			osn1 := mocks.NewOrderer(5614, t)

			time.Sleep(time.Second)
			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"localhost:5614", "localhost:5615"},
				},
			})
			assert.NoError(t, err)

			li := &mocks.MockLedgerInfo{Height: uint64(100)}
			osn1.SetNextExpectedSeek(uint64(100))
			err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
			assert.NoError(t, err, "can't start delivery")

			// make sure we have a block receiver before we send a block
			_, err = detectOSNConnections(isBFT, osn1)
			assert.NoError(t, err)

			// Check that delivery service requests blocks in order
			go osn1.SendBlock(uint64(100))
			assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
			blockEP1 := service.GetEndpoint("TEST_CHAINID")

			go osn1.SendBlock(uint64(101))
			assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
			li.Set(uint64(102))
			// Now wait for a few seconds
			time.Sleep(time.Second * 2)
			// Now start the new instance
			osn2 := mocks.NewOrderer(5615, t)
			osn2.SetNextExpectedSeek(uint64(102))
			// Now stop the old instance
			osn1.Shutdown()
			// Send a block from osn2
			time.Sleep(time.Second)
			for {
				blockEP2 := service.GetEndpoint("TEST_CHAINID")
				if len(blockEP2) > 0 && blockEP2 != blockEP1 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}

			go osn2.SendBlock(uint64(102))
			// Ensure it is received
			assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
			service.Stop()
			osn2.Shutdown()
		})
	}
}

func TestDeliverServiceDisconnectReconnect(t *testing.T) {
	// Scenario: Launch an ordering service node and let the client pull some blocks.
	// Stop ordering service, wait for while - simulate disconnect and restart it back.
	// Wait for some time, without sending blocks - simulate recv wait on empty channel.
	// Repeat stop/start sequence multiple times, to make sure total retry time will pass
	// value returned by getReConnectTotalTimeThreshold - in test it set to 2 seconds
	// (0.5s + 1s + 2s + 4s) > 2s.
	// Send new block and check that delivery client got it.
	// So, we can see that waiting on recv in empty channel do reset total time spend in reconnection.
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			flogging.ActivateSpec("bftDeliveryClient=DEBUG")
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			viper.Set("peer.deliveryclient.bft.blockRcvTotalBackoffDelay", time.Second)
			viper.Set("peer.deliveryclient.connTimeout", 100*time.Millisecond)
			viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", 2*time.Second)
			defer viper.Reset()
			defer ensureNoGoroutineLeak(t)()

			osn := mocks.NewOrderer(5614, t)

			time.Sleep(time.Second)
			gossipServiceAdapter := &mocks.MockGossipServiceAdapter{GossipBlockDisseminations: make(chan uint64)}

			service, err := NewDeliverService(&Config{
				Gossip:      gossipServiceAdapter,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{
				Organizations: []string{"org"},
				OrdererEndpointsByOrg: map[string][]string{
					"org": {"localhost:5614"},
				},
			})
			assert.NoError(t, err)

			li := &mocks.MockLedgerInfo{Height: uint64(100)}
			osn.SetNextExpectedSeek(uint64(100))
			err = service.StartDeliverForChannel("TEST_CHAINID", li, func() {})
			assert.NoError(t, err, "can't start delivery")

			// Check that delivery service requests blocks in order
			go osn.SendBlock(uint64(100))
			assertBlockDissemination(100, gossipServiceAdapter.GossipBlockDisseminations, t)
			go osn.SendBlock(uint64(101))
			assertBlockDissemination(101, gossipServiceAdapter.GossipBlockDisseminations, t)
			li.Set(102)

			for i := 0; i < 5; i += 1 {
				// Shutdown orderer, simulate network disconnect
				osn.Shutdown()
				// Now wait for a disconnect to be discovered
				assert.True(t, waitForConnectionCount(osn, 0), "deliverService can't disconnect from orderer")
				// Recreate orderer, simulating network is back
				osn = mocks.NewOrderer(5614, t)
				osn.SetNextExpectedSeek(102)
				// Now wait for a while, to client connect back and simulate empty channel
				assert.True(t, waitForConnectionCount(osn, 1), "deliverService can't reconnect to orderer")
			}

			// Send a block from orderer
			go osn.SendBlock(uint64(102))
			// Ensure it is received
			assertBlockDissemination(102, gossipServiceAdapter.GossipBlockDisseminations, t)
			service.Stop()
			osn.Shutdown()
		})
	}
}

func TestDeliverServiceBadConfig(t *testing.T) {
	bftOpt := []bool{false, true}
	for _, isBFT := range bftOpt {
		t.Run(fmt.Sprintf("BFT=%v", isBFT), func(t *testing.T) {
			flogging.ActivateSpec("bftDeliveryClient=DEBUG")
			viper.Set("peer.deliveryclient.bft.enabled", isBFT)
			defer viper.Reset()

			notEmptyConnectionCriteria := ConnectionCriteria{
				Organizations:         []string{"foo"},
				OrdererEndpointsByOrg: map[string][]string{"foo": {"bar", "baz"}},
			}
			// Empty endpoints
			service, err := NewDeliverService(&Config{
				Gossip:      &mocks.MockGossipServiceAdapter{},
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, ConnectionCriteria{})
			assert.EqualError(t, err, "no endpoints specified")
			assert.Nil(t, service)

			// Nil gossip adapter
			service, err = NewDeliverService(&Config{
				Gossip:      nil,
				CryptoSvc:   &mockMCS{},
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, notEmptyConnectionCriteria)
			assert.EqualError(t, err, "no gossip provider specified")
			assert.Nil(t, service)

			// Nil crypto service
			service, err = NewDeliverService(&Config{
				Gossip:      &mocks.MockGossipServiceAdapter{},
				CryptoSvc:   nil,
				ABCFactory:  DefaultABCFactory,
				ConnFactory: DefaultConnectionFactory,
			}, notEmptyConnectionCriteria)
			assert.EqualError(t, err, "no crypto service specified")
			assert.Nil(t, service)

			// Nil ABCFactory
			service, err = NewDeliverService(&Config{
				Gossip:      &mocks.MockGossipServiceAdapter{},
				CryptoSvc:   &mockMCS{},
				ABCFactory:  nil,
				ConnFactory: DefaultConnectionFactory,
			}, notEmptyConnectionCriteria)
			assert.EqualError(t, err, "no AtomicBroadcast factory specified")
			assert.Nil(t, service)

			// Nil connFactory
			service, err = NewDeliverService(&Config{
				Gossip:     &mocks.MockGossipServiceAdapter{},
				CryptoSvc:  &mockMCS{},
				ABCFactory: DefaultABCFactory,
			}, notEmptyConnectionCriteria)
			assert.EqualError(t, err, "no connection factory specified")
			assert.Nil(t, service)
		})
	}
}

func TestRetryPolicyOverflow(t *testing.T) {
	connFactory := func(channelID string, _ map[string]*comm.OrdererEndpoint) func(comm.EndpointCriteria) (*grpc.ClientConn, error) {
		return func(_ comm.EndpointCriteria) (*grpc.ClientConn, error) {
			return nil, errors.New("")
		}
	}
	client := (&deliverServiceImpl{
		conf: &Config{ConnFactory: connFactory}}).newClient("TEST", &mocks.MockLedgerInfo{Height: uint64(100)})
	assert.NotNil(t, client.shouldRetry)
	for i := 0; i < 100; i++ {
		retryTime, _ := client.shouldRetry(i, time.Second)
		assert.True(t, retryTime <= time.Hour && retryTime > 0)
	}
}

func assertBlockDissemination(expectedSeq uint64, ch chan uint64, t *testing.T) {
	select {
	case seq := <-ch:
		assert.Equal(t, expectedSeq, seq)
	case <-time.After(time.Second * 5):
		assert.FailNow(t, fmt.Sprintf("Didn't gossip a new block with seq num %d within a timely manner", expectedSeq))
		t.Fatal()
	}
}

func ensureNoGoroutineLeak(t *testing.T) func() {
	goroutineCountAtStart := runtime.NumGoroutine()
	return func() {
		start := time.Now()
		timeLimit := start.Add(goRoutineTestWaitTimeout)
		for time.Now().Before(timeLimit) {
			time.Sleep(time.Millisecond * 500)
			if goroutineCountAtStart >= runtime.NumGoroutine() {
				return
			}
		}
		assert.Fail(t, "Some goroutine(s) didn't finish: %s", getStackTrace())
	}
}

func getStackTrace() string {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	return string(buf)
}

func waitForConnectionCount(orderer *mocks.Orderer, connCount int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	for {
		select {
		case <-time.After(time.Millisecond * 100):
			if orderer.ConnCount() == connCount {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
}

func TestToEndpointCriteria(t *testing.T) {
	for _, testCase := range []struct {
		description string
		input       ConnectionCriteria
		expectedOut []comm.EndpointCriteria
	}{
		{
			description: "globally defined endpoints",
			input: ConnectionCriteria{
				Organizations:    []string{"foo", "bar"},
				OrdererEndpoints: []string{"a", "b", "c"},
			},
			expectedOut: []comm.EndpointCriteria{
				{Organizations: []string{"foo", "bar"}, Endpoint: "a"},
				{Organizations: []string{"foo", "bar"}, Endpoint: "b"},
				{Organizations: []string{"foo", "bar"}, Endpoint: "c"},
			},
		},
		{
			description: "globally defined endpoints with overrides",
			input: ConnectionCriteria{
				Organizations:    []string{"foo", "bar"},
				OrdererEndpoints: []string{"a", "b", "c"},
				OrdererEndpointOverrides: map[string]*comm.OrdererEndpoint{
					"b": {
						Address: "d",
					},
				},
			},
			expectedOut: []comm.EndpointCriteria{
				{Organizations: []string{"foo", "bar"}, Endpoint: "a"},
				{Organizations: []string{"foo", "bar"}, Endpoint: "d"},
				{Organizations: []string{"foo", "bar"}, Endpoint: "c"},
			},
		},
		{
			description: "per org defined endpoints",
			input: ConnectionCriteria{
				Organizations: []string{"foo", "bar"},
				// Even if OrdererEndpoints are defined, the OrdererEndpointsByOrg take precedence.
				OrdererEndpoints: []string{"a", "b", "c"},
				OrdererEndpointsByOrg: map[string][]string{
					"foo": {"a", "b"},
					"bar": {"c"},
				},
			},
			expectedOut: []comm.EndpointCriteria{
				{Organizations: []string{"foo"}, Endpoint: "a"},
				{Organizations: []string{"foo"}, Endpoint: "b"},
				{Organizations: []string{"bar"}, Endpoint: "c"},
			},
		},
		{
			description: "per org defined endpoints with overrides",
			input: ConnectionCriteria{
				Organizations: []string{"foo", "bar"},
				// Even if OrdererEndpoints are defined, the OrdererEndpointsByOrg take precedence.
				OrdererEndpoints: []string{"a", "b", "c"},
				OrdererEndpointsByOrg: map[string][]string{
					"foo": {"a", "b"},
					"bar": {"c"},
				},
				OrdererEndpointOverrides: map[string]*comm.OrdererEndpoint{
					"b": {
						Address: "d",
					},
				},
			},
			expectedOut: []comm.EndpointCriteria{
				{Organizations: []string{"foo"}, Endpoint: "a"},
				{Organizations: []string{"foo"}, Endpoint: "d"},
				{Organizations: []string{"bar"}, Endpoint: "c"},
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			assert.Equal(t, testCase.expectedOut, testCase.input.toEndpointCriteria())
		})
	}
}

func detectOSNConnections(isBFT bool, osSet ...*mocks.Orderer) ([]*mocks.Orderer, error) {
	var first int
	r := make([]*mocks.Orderer, len(osSet))

	if !isBFT {
		for i := int64(0); ; i++ {
			first = -1
			for j, os := range osSet {
				if os.ConnCount() > 0 {
					first = j
					break
				}
			}

			if first >= 0 {
				break //found
			}

			time.Sleep(time.Millisecond * 10)
			if time.Duration(i*10*time.Millisecond.Nanoseconds()) > 5*time.Second {
				return nil, errors.New("timeout: no connections")
			}
		}
	} else {
		cType := make([]orderer.SeekInfo_SeekContentType, len(osSet))
		for i := int64(0); ; i++ {
			var cCnt int
			first = -1
			for j, os := range osSet {
				c1, t1 := os.ConnCountType()
				if c1 > 0 {
					cCnt++
				}
				cType[j] = t1
				if t1 == orderer.SeekInfo_BLOCK {
					first = j
				}
			}

			if cCnt == len(osSet) && first >= 0 {
				break //found
			}

			time.Sleep(time.Millisecond * 10)
			if time.Duration(i*10*time.Millisecond.Nanoseconds()) > 5*time.Second {
				return nil, errors.New("timeout: no connections")
			}
		}
	}

	r[0] = osSet[first]
	n := 1
	for j, os := range osSet {
		if j == first {
			continue
		}
		r[n] = os
		n++
	}
	return r, nil
}
