/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/orderer"
)

// TypeKey is the string with which this consensus implementation is identified across Fabric.
const TypeKey = "smartbft"

func init() {
	orderer.ConsensusTypeMetadataMap[TypeKey] = ConsensusTypeMetadataFactory{}
	orderer.DecodeHooks[TypeKey] = append(orderer.DecodeHooks[TypeKey], decodeConfig)
	orderer.EncodeHooks[TypeKey] = append(orderer.EncodeHooks[TypeKey], encodeConfig)
}

// ConsensusTypeMetadataFactory allows this implementation's proto messages to register
// their type with the orderer's proto messages. This is needed for protolator to work.
type ConsensusTypeMetadataFactory struct{}

// NewMessage implements the Orderer.ConsensusTypeMetadataFactory interface.
func (dogf ConsensusTypeMetadataFactory) NewMessage() proto.Message {
	return &ConfigMetadata{}
}

func encodeConfig(message proto.Message) proto.Message {
	config, isConfig := message.(*common.Config)
	_, isBlock := message.(*common.Block)
	if !isConfig && !isBlock {
		return message
	}
	if isBlock {
		config, _, _, _ = parseBlock(message.(*common.Block))
	}
	rawConsensusType := config.ChannelGroup.Groups["Orderer"].Values["ConsensusType"].Value
	consensusTypeValue := &orderer.ConsensusType{}
	err := proto.Unmarshal(rawConsensusType, consensusTypeValue)
	panicOnErr(err)
	metadata := &ConfigMetadata{}
	err = proto.Unmarshal(consensusTypeValue.Metadata, metadata)
	panicOnErr(err)
	for _, consenter := range metadata.Consenters {
		consenter.Identity, err = proto.Marshal(&msp.SerializedIdentity{
			Mspid:   consenter.MspId,
			IdBytes: consenter.Identity,
		})
		panicOnErr(err)
	}
	consensusTypeValue.Metadata, err = proto.Marshal(metadata)
	panicOnErr(err)
	config.ChannelGroup.Groups["Orderer"].Values["ConsensusType"].Value, _ = proto.Marshal(consensusTypeValue)

	if isBlock {
		block := message.(*common.Block)
		_, confEnv, pl, env := parseBlock(block)
		confEnv.Config = config
		pl.Data = marshalOrPanic(confEnv)
		env.Payload = marshalOrPanic(pl)
		block.Data.Data[0] = marshalOrPanic(env)
		return block
	}

	return config
}

func decodeConfig(message proto.Message) proto.Message {
	config, isConfig := message.(*common.Config)
	_, isBlock := message.(*common.Block)
	if !isConfig && !isBlock {
		return message
	}
	if isBlock {
		config, _, _, _ = parseBlock(message.(*common.Block))
	}
	rawConsensusType := config.ChannelGroup.Groups["Orderer"].Values["ConsensusType"].Value
	consensusTypeValue := &orderer.ConsensusType{}
	err := proto.Unmarshal(rawConsensusType, consensusTypeValue)
	panicOnErr(err)
	metadata := &ConfigMetadata{}
	err = proto.Unmarshal(consensusTypeValue.Metadata, metadata)
	panicOnErr(err)
	for _, consenter := range metadata.Consenters {
		sID := &msp.SerializedIdentity{}
		err := proto.Unmarshal(consenter.Identity, sID)
		if err != nil {
			fmt.Println(string(consenter.Identity))
			panicOnErr(err)
		}
		consenter.Identity = sID.IdBytes
	}
	consensusTypeValue.Metadata, _ = proto.Marshal(metadata)
	config.ChannelGroup.Groups["Orderer"].Values["ConsensusType"].Value, _ = proto.Marshal(consensusTypeValue)

	if isBlock {
		block := message.(*common.Block)
		_, confEnv, pl, env := parseBlock(block)
		confEnv.Config = config
		pl.Data = marshalOrPanic(confEnv)
		env.Payload = marshalOrPanic(pl)
		block.Data.Data[0] = marshalOrPanic(env)
		return block
	}

	return config
}

func marshalOrPanic(msg proto.Message) []byte {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return bytes
}

func panicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}

func parseBlock(block *common.Block) (*common.Config, *common.ConfigEnvelope, *common.Payload, *common.Envelope) {
	txn := block.Data.Data[0]
	env := &common.Envelope{}
	err := proto.Unmarshal(txn, env)
	if err != nil {
		panic(err)
	}
	pl := &common.Payload{}
	err = proto.Unmarshal(env.Payload, pl)
	if err != nil {
		panic(err)
	}
	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(pl.Data, configEnv)
	if err != nil {
		panic(err)
	}
	return configEnv.Config, configEnv, pl, env
}

// Marshal serializes this implementation's proto messages. It is called by the encoder package
// during the creation of the Orderer ConfigGroup.
func Marshal(md *ConfigMetadata) ([]byte, error) {
	copyMd := proto.Clone(md).(*ConfigMetadata)
	for _, c := range copyMd.Consenters {
		// Expect the user to set the config value for client/server certs to the
		// path where they are persisted locally, then load these files to memory.
		clientCert, err := ioutil.ReadFile(string(c.GetClientTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load client cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.ClientTlsCert = clientCert

		serverCert, err := ioutil.ReadFile(string(c.GetServerTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load server cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.ServerTlsCert = serverCert

		ecert, err := ioutil.ReadFile(string(c.GetIdentity()))
		if err != nil {
			return nil, fmt.Errorf("cannot load enrollment cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.Identity, err = proto.Marshal(&msp.SerializedIdentity{
			Mspid:   c.MspId,
			IdBytes: ecert,
		})
		if err != nil {
			panic(err)
		}
	}
	return proto.Marshal(copyMd)
}
