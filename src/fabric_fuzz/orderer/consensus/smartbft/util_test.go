/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"io/ioutil"
	"testing"

	"encoding/base64"
	"path/filepath"

	"crypto/sha256"
	"encoding/hex"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/mocks/common/multichannel"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestNewBlockPuller(t *testing.T) {
	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)

	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	assert.NoError(t, err)

	goodConfigBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, goodConfigBlock))

	lastBlock := &common.Block{
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
				Value: utils.MarshalOrPanic(&common.LastConfig{Index: 42}),
			})},
		},
	}

	cs := &multichannel.ConsenterSupport{
		HeightVal: 100,
		BlockByIndex: map[uint64]*common.Block{
			42: goodConfigBlock,
			99: lastBlock,
		},
	}

	dialer := &cluster.PredicateDialer{
		ClientConfig: comm.ClientConfig{
			SecOpts: &comm.SecureOptions{
				Certificate: ca.CertBytes(),
			},
		},
	}

	blockPuller, err := newBlockPuller(cs, dialer,
		localconfig.Cluster{
			ReplicationMaxRetries: 2,
		})
	assert.NoError(t, err)
	assert.NotNil(t, blockPuller)
	blockPuller.Close()
}

func TestIsConsenterOfChannel(t *testing.T) {
	certInsideConfigBlock, err := base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNmekNDQWlhZ0F3SUJBZ0lSQUlMS" +
		"ThQL1ZwTXZIUWxJTTJkRWZ0aHd3Q2dZSUtvWkl6ajBFQXdJd2JERUwKTUFrR0ExVUVCaE1DVlZNeEV6QVJCZ05WQkFnVENrTmhiR2xtYjNKdWFXRXhGakFVQmdOVkJBY1R" +
		"EVk5oYmlCRwpjbUZ1WTJselkyOHhGREFTQmdOVkJBb1RDMlY0WVcxd2JHVXVZMjl0TVJvd0dBWURWUVFERXhGMGJITmpZUzVsCmVHRnRjR3hsTG1OdmJUQWVGdzB4T1RFe" +
		"U1UWXhNVEU0TURCYUZ3MHlPVEV5TVRNeE1URTRNREJhTUZreEN6QUoKQmdOVkJBWVRBbFZUTVJNd0VRWURWUVFJRXdwRFlXeHBabTl5Ym1saE1SWXdGQVlEVlFRSEV3MVR" +
		"ZVzRnUm5KaApibU5wYzJOdk1SMHdHd1lEVlFRREV4UnZjbVJsY21WeU5DNWxlR0Z0Y0d4bExtTnZiVEJaTUJNR0J5cUdTTTQ5CkFnRUdDQ3FHU000OUF3RUhBMElBQkF4Z" +
		"UJxaVRnN21TK1dURGpvd0c4V3pmeFI2L1FHNEFxdHJYaTJRNTBKMUwKY2xkdzF4bTdJejQ4THZqa3pTTjNRMm9peFRHVjB6ZFFzcUtuOWNyRXp0MmpnYnN3Z2Jnd0RnWUR" +
		"WUjBQQVFILwpCQVFEQWdXZ01CMEdBMVVkSlFRV01CUUdDQ3NHQVFVRkJ3TUJCZ2dyQmdFRkJRY0RBakFNQmdOVkhSTUJBZjhFCkFqQUFNQ3NHQTFVZEl3UWtNQ0tBSUUzY" +
		"VEwV2IrSXlSM0xFa2RmNWRyaUoyYlU0MXYvcUNTQkxMQlhXazZxSHMKTUV3R0ExVWRFUVJGTUVPQ0ZHOXlaR1Z5WlhJMExtVjRZVzF3YkdVdVkyOXRnZ2h2Y21SbGNtVnl" +
		"OSUlKYkc5agpZV3hvYjNOMGh3Ui9BQUFCaHhBQUFBQUFBQUFBQUFBQUFBQUFBQUFCTUFvR0NDcUdTTTQ5QkFNQ0EwY0FNRVFDCklFTGdpYzNSSU9QRjFucll2Rit2STBTc" +
		"mVPN1dEc2FTM0NOaDZqdUdkTGlOQWlCOHdSYVRXY3ZaKzg4Qkxwc3QKVkdnUE9PbVAzRTJPeVJVMEpTWFFMamlHeFE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==")
	assert.NoError(t, err)

	loadBlock := func(fileName string) *common.Block {
		b, err := ioutil.ReadFile(filepath.Join("testdata", fileName))
		assert.NoError(t, err)
		block := &common.Block{}
		err = proto.Unmarshal(b, block)
		assert.NoError(t, err)
		return block
	}
	for _, testCase := range []struct {
		name          string
		expectedError string
		configBlock   *common.Block
		certificate   []byte
	}{
		{
			name:          "nil block",
			expectedError: "nil block",
		},
		{
			name:          "no block data",
			expectedError: "block data is nil",
			configBlock:   &common.Block{},
		},
		{
			name: "invalid envelope inside block",
			expectedError: "failed to unmarshal payload from envelope:" +
				" error unmarshaling Payload: proto: common.Payload: illegal tag 0 (wire type 1)",
			configBlock: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:          "valid config block with cert mismatch",
			configBlock:   loadBlock("smartbft_genesis_block.pb"),
			certificate:   certInsideConfigBlock[2:],
			expectedError: cluster.ErrNotInChannel.Error(),
		},
		{
			name:          "etcdraft genesis block",
			configBlock:   loadBlock("etcdraftgenesis.block"),
			expectedError: "not a SmartBFT config block",
		},
		{
			name:        "valid config block with matching cert",
			configBlock: loadBlock("smartbft_genesis_block.pb"),
			certificate: certInsideConfigBlock,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := ConsenterCertificate(testCase.certificate).IsConsenterOfChannel(testCase.configBlock)
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func makeTx(nonce, creator []byte) []byte {
	return utils.MarshalOrPanic(&common.Envelope{
		Payload: utils.MarshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
					Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
					ChannelId: "test-chain",
				}),
				SignatureHeader: utils.MarshalOrPanic(&common.SignatureHeader{
					Creator: creator,
					Nonce:   nonce,
				}),
			},
		}),
	})
}

func TestRequestID(t *testing.T) {
	ri := &RequestInspector{
		ValidateIdentityStructure: func(identity *msp.SerializedIdentity) error {
			return nil
		},
	}

	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	creator := utils.MarshalOrPanic(&msp.SerializedIdentity{
		Mspid:   "SampleOrg",
		IdBytes: []byte{1, 2, 3},
	})

	tx := makeTx(nonce, creator)

	var expectedTxID []byte
	expectedTxID = append(expectedTxID, nonce...)
	expectedTxID = append(expectedTxID, creator...)

	txID := sha256.Sum256(expectedTxID)
	expectedTxString := hex.EncodeToString(txID[:])

	expectedClient := sha256.Sum256(creator)

	info := ri.RequestID(tx)
	assert.Equal(t, expectedTxString, info.ID)
	assert.Equal(t, hex.EncodeToString(expectedClient[:]), info.ClientID)
}

func TestRemoteNodesFromConfigBlock(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("testdata", "smartbft_genesis_block.pb"))
	assert.NoError(t, err)

	block := &common.Block{}
	err = proto.Unmarshal(b, block)
	assert.NoError(t, err)

	config, err := RemoteNodesFromConfigBlock(block, 1, flogging.MustGetLogger("test"))
	assert.NoError(t, err)

	assert.NotNil(t, config)
}

func TestRuntimeConfig(t *testing.T) {
	b, err := ioutil.ReadFile(filepath.Join("testdata", "smartbft_genesis_block.pb"))
	assert.NoError(t, err)

	genesisBlock := &common.Block{}
	err = proto.Unmarshal(b, genesisBlock)
	assert.NoError(t, err)

	notConfigBlock := &common.Block{
		Header: &common.BlockHeader{Number: 10},
		Data: &common.BlockData{
			Data: [][]byte{
				makeTx(nil, nil),
			},
		}}

	prevRTC := RuntimeConfig{
		logger: flogging.MustGetLogger("test"),
		id:     1,
	}

	// Commit a config block
	prevRTC, err = prevRTC.BlockCommitted(genesisBlock)
	assert.NoError(t, err)
	assert.True(t, prevRTC.isConfig)

	// Commit a non config block
	newRTC, err := prevRTC.BlockCommitted(notConfigBlock)
	assert.NoError(t, err)
	assert.False(t, newRTC.isConfig)

	assert.Equal(t, prevRTC.id, newRTC.id)
	assert.Equal(t, prevRTC.logger, newRTC.logger)
	assert.Equal(t, prevRTC.Nodes, newRTC.Nodes)
	assert.Equal(t, prevRTC.RemoteNodes, newRTC.RemoteNodes)
	assert.Equal(t, prevRTC.LastConfigBlock, newRTC.LastConfigBlock)
	assert.Equal(t, prevRTC.ID2Identities, newRTC.ID2Identities)
	assert.NotEqual(t, prevRTC.LastBlock, newRTC.LastBlock)
	assert.NotEqual(t, prevRTC.LastCommittedBlockHash, newRTC.LastCommittedBlockHash)

	b, err = ioutil.ReadFile(filepath.Join("testdata", "systemchannel_block.pb"))
	assert.NoError(t, err)

	configBlock := &common.Block{}
	err = proto.Unmarshal(b, configBlock)
	assert.NoError(t, err)

	// Ensure it's not the genesis block by mistake
	assert.Equal(t, uint64(2), configBlock.Header.Number)

	prevRTC = newRTC
	newRTC, err = newRTC.BlockCommitted(configBlock)
	assert.NoError(t, err)
	assert.True(t, newRTC.isConfig)

	assert.Equal(t, prevRTC.id, newRTC.id)
	assert.Equal(t, prevRTC.logger, newRTC.logger)
	assert.NotEqual(t, prevRTC.Nodes, newRTC.Nodes)
	assert.NotEqual(t, prevRTC.RemoteNodes, newRTC.RemoteNodes)
	assert.NotEqual(t, prevRTC.LastConfigBlock, newRTC.LastConfigBlock)
	assert.NotEqual(t, prevRTC.ID2Identities, newRTC.ID2Identities)
	assert.NotEqual(t, prevRTC.LastBlock, newRTC.LastBlock)
	assert.NotEqual(t, prevRTC.LastCommittedBlockHash, newRTC.LastCommittedBlockHash)
}
