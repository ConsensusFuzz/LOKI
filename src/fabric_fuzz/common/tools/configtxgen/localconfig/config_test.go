/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/orderer/smartbft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfile(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPSoloProfile,
		SampleDevModeSmartBFTProfile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName)
			assert.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadBFTProfile(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	p := Load(SampleDevModeSmartBFTProfile)
	assert.NotNil(t, p, "profile should not be nil")
	for polN, pol := range p.Orderer.Policies {
		fmt.Printf("%s %s \n", polN, pol)
	}
}

func TestLoadProfileWithPath(t *testing.T) {
	devConfigDir, err := configtest.GetDevConfigDir()
	assert.NoError(t, err, "failed to get dev config dir")

	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPSoloProfile,
		SampleDevModeSmartBFTProfile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName, devConfigDir)
			assert.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadTopLevel(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	topLevel := LoadTopLevel()
	assert.NotNil(t, topLevel.Application, "application should not be nil")
	assert.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	assert.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	assert.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	assert.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}

func TestLoadTopLevelWithPath(t *testing.T) {
	devConfigDir, err := configtest.GetDevConfigDir()
	require.NoError(t, err)

	topLevel := LoadTopLevel(devConfigDir)
	assert.NotNil(t, topLevel.Application, "application should not be nil")
	assert.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	assert.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	assert.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	assert.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}

func TestConsensusSpecificInit(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	devConfigDir, err := configtest.GetDevConfigDir()
	require.NoError(t, err)

	t.Run("nil orderer type", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "",
			},
		}
		profile.completeInitialization(devConfigDir)

		assert.Equal(t, profile.Orderer.OrdererType, genesisDefaults.Orderer.OrdererType)
	})

	t.Run("unknown orderer type", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "unknown",
			},
		}

		assert.Panics(t, func() {
			profile.completeInitialization(devConfigDir)
		})
	})

	t.Run("solo", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "solo",
			},
		}
		profile.completeInitialization(devConfigDir)
		assert.Nil(t, profile.Orderer.Kafka.Brokers, "Kafka config settings should not be set")
	})

	t.Run("kafka", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "kafka",
			},
		}
		profile.completeInitialization(devConfigDir)
		assert.NotNil(t, profile.Orderer.Kafka.Brokers, "Kafka config settings should be set")
	})

	t.Run("raft", func(t *testing.T) {
		makeProfile := func(consenters []*etcdraft.Consenter, options *etcdraft.Options) *Profile {
			return &Profile{
				Orderer: &Orderer{
					OrdererType: "etcdraft",
					EtcdRaft: &etcdraft.ConfigMetadata{
						Consenters: consenters,
						Options:    options,
					},
				},
			}
		}
		t.Run("EtcdRaft section not specified in profile", func(t *testing.T) {
			profile := &Profile{
				Orderer: &Orderer{
					OrdererType: "etcdraft",
				},
			}

			assert.Panics(t, func() {
				profile.completeInitialization(devConfigDir)
			})
		})

		t.Run("nil consenter set", func(t *testing.T) { // should panic
			profile := makeProfile(nil, nil)

			assert.Panics(t, func() {
				profile.completeInitialization(devConfigDir)
			})
		})

		t.Run("single consenter", func(t *testing.T) {
			consenters := []*etcdraft.Consenter{
				{
					Host:          "node-1.example.com",
					Port:          7050,
					ClientTlsCert: []byte("path/to/client/cert"),
					ServerTlsCert: []byte("path/to/server/cert"),
				},
			}

			t.Run("invalid consenters specification", func(t *testing.T) {
				failingConsenterSpecifications := []*etcdraft.Consenter{
					{ // missing Host
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
					},
					{ // missing Port
						Host:          "node-1.example.com",
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
					},
					{ // missing ClientTlsCert
						Host:          "node-1.example.com",
						Port:          7050,
						ServerTlsCert: []byte("path/to/server/cert"),
					},
					{ // missing ServerTlsCert
						Host:          "node-1.example.com",
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
					},
				}

				for _, consenter := range failingConsenterSpecifications {
					profile := makeProfile([]*etcdraft.Consenter{consenter}, nil)

					assert.Panics(t, func() {
						profile.completeInitialization(devConfigDir)
					})
				}
			})

			t.Run("nil Options", func(t *testing.T) {
				profile := makeProfile(consenters, nil)
				profile.completeInitialization(devConfigDir)

				// need not be tested in subsequent tests
				assert.NotNil(t, profile.Orderer.EtcdRaft, "EtcdRaft config settings should be set")
				assert.Equal(t, profile.Orderer.EtcdRaft.Consenters[0].ClientTlsCert, consenters[0].ClientTlsCert,
					"Client TLS cert path should be correctly set")

				// specific assertion for this test context
				assert.Equal(t, profile.Orderer.EtcdRaft.Options, genesisDefaults.Orderer.EtcdRaft.Options,
					"Options should be set to the default value")
			})

			t.Run("heartbeat tick specified in Options", func(t *testing.T) {
				heartbeatTick := uint32(2)
				options := &etcdraft.Options{ // partially set so that we can check that the other members are set to defaults
					HeartbeatTick: heartbeatTick,
				}
				profile := makeProfile(consenters, options)
				profile.completeInitialization(devConfigDir)

				// specific assertions for this test context
				assert.Equal(t, profile.Orderer.EtcdRaft.Options.HeartbeatTick, heartbeatTick,
					"HeartbeatTick should be set to the specified value")
				assert.Equal(t, profile.Orderer.EtcdRaft.Options.ElectionTick, genesisDefaults.Orderer.EtcdRaft.Options.ElectionTick,
					"ElectionTick should be set to the default value")
			})

			t.Run("election tick specified in Options", func(t *testing.T) {
				electionTick := uint32(20)
				options := &etcdraft.Options{ // partially set so that we can check that the other members are set to defaults
					ElectionTick: electionTick,
				}
				profile := makeProfile(consenters, options)
				profile.completeInitialization(devConfigDir)

				// specific assertions for this test context
				assert.Equal(t, profile.Orderer.EtcdRaft.Options.ElectionTick, electionTick,
					"ElectionTick should be set to the specified value")
				assert.Equal(t, profile.Orderer.EtcdRaft.Options.HeartbeatTick, genesisDefaults.Orderer.EtcdRaft.Options.HeartbeatTick,
					"HeartbeatTick should be set to the default value")
			})

			t.Run("panic on invalid Heartbeat and Election tick", func(t *testing.T) {
				options := &etcdraft.Options{
					HeartbeatTick: 2,
					ElectionTick:  1,
				}
				profile := makeProfile(consenters, options)

				assert.Panics(t, func() {
					profile.completeInitialization(devConfigDir)
				})
			})

			t.Run("panic on invalid TickInterval", func(t *testing.T) {
				options := &etcdraft.Options{
					TickInterval: "500",
				}
				profile := makeProfile(consenters, options)

				assert.Panics(t, func() {
					profile.completeInitialization(devConfigDir)
				})
			})
		})
	})

	t.Run("smartbft", func(t *testing.T) {
		makeProfile := func(consenters []*smartbft.Consenter, options *smartbft.Options) *Profile {
			return &Profile{
				Orderer: &Orderer{
					OrdererType: "smartbft",
					SmartBFT: &smartbft.ConfigMetadata{
						Consenters: consenters,
						Options:    options,
					},
				},
			}
		}

		t.Run("No Smart BFT configuration provided", func(t *testing.T) {
			profile := makeProfile(nil, nil)
			profile.Orderer.SmartBFT = nil

			assert.Panics(t, func() {
				profile.completeInitialization(devConfigDir)
			})
		})

		t.Run("nil consenter set", func(t *testing.T) { // should panic
			profile := makeProfile(nil, nil)

			assert.Panics(t, func() {
				profile.completeInitialization(devConfigDir)
			})
		})

		t.Run("single consenter", func(t *testing.T) {
			consenters := []*smartbft.Consenter{
				{
					Host:          "node-1.example.com",
					Port:          7050,
					ClientTlsCert: []byte("path/to/client/cert"),
					ServerTlsCert: []byte("path/to/server/cert"),
					MspId:         "Org1",
					Identity:      []byte("path/to/indentity/cert"),
				},
			}

			t.Run("invalid consenters specification", func(t *testing.T) {
				failingConsenterSpecifications := []*smartbft.Consenter{
					{ // missing Host
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
						MspId:         "Org1",
						Identity:      []byte("path/to/indentity/cert"),
					},
					{ // missing Port
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
						MspId:         "Org1",
						Identity:      []byte("path/to/indentity/cert"),
					},
					{ // missing ClientTlsCert
						Host:          "node-1.example.com",
						Port:          7050,
						ServerTlsCert: []byte("path/to/server/cert"),
						MspId:         "Org1",
						Identity:      []byte("path/to/indentity/cert"),
					},
					{ // missing ServerTlsCert
						Host:          "node-1.example.com",
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						MspId:         "Org1",
						Identity:      []byte("path/to/indentity/cert"),
					},
					{ // missing identity cert
						Host:          "node-1.example.com",
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
						MspId:         "Org1",
					},
					{ // missing msp id
						Host:          "node-1.example.com",
						Port:          7050,
						ClientTlsCert: []byte("path/to/client/cert"),
						ServerTlsCert: []byte("path/to/server/cert"),
						Identity:      []byte("path/to/indentity/cert"),
					},
				}

				for _, consenter := range failingConsenterSpecifications {
					profile := makeProfile([]*smartbft.Consenter{consenter}, nil)

					assert.Panics(t, func() {
						profile.completeInitialization(devConfigDir)
					})
				}
			})

			t.Run("nil Options", func(t *testing.T) {
				profile := makeProfile(consenters, nil)
				profile.completeInitialization(devConfigDir)

				// need not be tested in subsequent tests
				assert.NotNil(t, profile.Orderer.SmartBFT, "SmartBFT config settings should be set")
				assert.Equal(t, profile.Orderer.SmartBFT.Consenters[0].ClientTlsCert, consenters[0].ClientTlsCert,
					"Client TLS cert path should be correctly set")
				assert.Equal(t, profile.Orderer.SmartBFT.Consenters[0].ServerTlsCert, consenters[0].ServerTlsCert,
					"Server TLS cert path should be correctly set")
				assert.Equal(t, profile.Orderer.SmartBFT.Consenters[0].Identity, consenters[0].Identity,
					"Consenter identity cert path should be correctly set")
				assert.Equal(t, profile.Orderer.SmartBFT.Consenters[0].MspId, consenters[0].MspId,
					"Consenter MSP ID should be correctly set")

				// specific assertion for this test context
				assert.Equal(t, profile.Orderer.SmartBFT.Options, genesisDefaults.Orderer.SmartBFT.Options,
					"Options should be set to the default value")
			})

		})

	})
}
