/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sort"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/orderer/smartbft"
	utils2 "github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mocks/mock_blockpuller.go . BlockPuller

// newBlockPuller creates a new block puller
func newBlockPuller(
	support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster) (BlockPuller, error) {

	verifyBlockSequence := func(blocks []*common.Block, _ string) error {
		return cluster.VerifyBlocksBFT(blocks, support)
	}

	stdDialer := &cluster.StandardDialer{
		ClientConfig: baseDialer.ClientConfig.Clone(),
	}
	stdDialer.ClientConfig.AsyncConnect = false
	stdDialer.ClientConfig.SecOpts.VerifyCertificate = nil

	// Extract the TLS CA certs and endpoints from the configuration,
	endpoints, err := etcdraft.EndpointconfigFromFromSupport(support)
	if err != nil {
		return nil, err
	}

	der, _ := pem.Decode(stdDialer.ClientConfig.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(stdDialer.ClientConfig.SecOpts.Certificate))
	}

	bp := &cluster.BlockPuller{
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller"),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChainID(),
		Dialer:              stdDialer,
	}

	return bp, nil
}

func getViewMetadataFromBlock(block *common.Block) (smartbftprotos.ViewMetadata, error) {
	if block.Header.Number == 0 {
		// Genesis block has no prior metadata so we just return an un-initialized metadata
		return smartbftprotos.ViewMetadata{}, nil
	}

	signatureMetadata := utils2.GetMetadataFromBlockOrPanic(block, common.BlockMetadataIndex_SIGNATURES)
	ordererMD := &common.OrdererBlockMetadata{}
	if err := proto.Unmarshal(signatureMetadata.Value, ordererMD); err != nil {
		return smartbftprotos.ViewMetadata{}, errors.Wrap(err, "failed unmarshaling OrdererBlockMetadata")
	}

	var viewMetadata smartbftprotos.ViewMetadata
	if err := proto.Unmarshal(ordererMD.ConsenterMetadata, &viewMetadata); err != nil {
		return smartbftprotos.ViewMetadata{}, err
	}

	return viewMetadata, nil
}

func configFromMetadataOptions(selfID uint64, options *smartbft.Options) (types.Configuration, error) {
	var err error

	config := types.DefaultConfig
	config.SelfID = selfID

	if options == nil {
		return config, errors.New("config metadata options field is nil")
	}

	config.RequestBatchMaxCount = options.RequestBatchMaxCount
	config.RequestBatchMaxBytes = options.RequestBatchMaxBytes
	if config.RequestBatchMaxInterval, err = time.ParseDuration(options.RequestBatchMaxInterval); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestBatchMaxInterval")
	}
	config.IncomingMessageBufferSize = options.IncomingMessageBufferSize
	config.RequestPoolSize = options.RequestPoolSize
	if config.RequestForwardTimeout, err = time.ParseDuration(options.RequestForwardTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestForwardTimeout")
	}
	if config.RequestComplainTimeout, err = time.ParseDuration(options.RequestComplainTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestComplainTimeout")
	}
	if config.RequestAutoRemoveTimeout, err = time.ParseDuration(options.RequestAutoRemoveTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option RequestAutoRemoveTimeout")
	}
	if config.ViewChangeResendInterval, err = time.ParseDuration(options.ViewChangeResendInterval); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeResendInterval")
	}
	if config.ViewChangeTimeout, err = time.ParseDuration(options.ViewChangeTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option ViewChangeTimeout")
	}
	if config.LeaderHeartbeatTimeout, err = time.ParseDuration(options.LeaderHeartbeatTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option LeaderHeartbeatTimeout")
	}
	config.LeaderHeartbeatCount = options.LeaderHeartbeatCount
	if config.CollectTimeout, err = time.ParseDuration(options.CollectTimeout); err != nil {
		return config, errors.Wrap(err, "bad config metadata option CollectTimeout")
	}
	config.SyncOnStart = options.SyncOnStart
	config.SpeedUpViewChange = options.SpeedUpViewChange

	if options.DecisionsPerLeader == 0 {
		config.DecisionsPerLeader = 1
	}

	// Enable rotation by default, but optionally disable it
	switch options.LeaderRotation {
	case smartbft.Options_OFF:
		config.LeaderRotation = false
		config.DecisionsPerLeader = 0
	default:
		config.LeaderRotation = true
	}

	if err = config.Validate(); err != nil {
		return config, errors.Wrap(err, "config validation failed")
	}

	return config, nil
}

// RequestInspector inspects incomming requests and validates serialized identity
type RequestInspector struct {
	ValidateIdentityStructure func(identity *msp.SerializedIdentity) error
}

func (ri *RequestInspector) RequestID(rawReq []byte) types.RequestInfo {
	req, err := ri.unwrapReq(rawReq)
	if err != nil {
		return types.RequestInfo{}
	}
	reqInfo, err := ri.requestIDFromSigHeader(req.sigHdr)
	if err != nil {
		return types.RequestInfo{}
	}
	return reqInfo
}

type request struct {
	sigHdr   *common.SignatureHeader
	envelope *common.Envelope
	chHdr    *common.ChannelHeader
}

func (ri *RequestInspector) requestIDFromSigHeader(sigHdr *common.SignatureHeader) (types.RequestInfo, error) {
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(sigHdr.Creator, sID); err != nil {
		return types.RequestInfo{}, errors.Wrap(err, "identity isn't an MSP Identity")
	}

	if err := ri.ValidateIdentityStructure(sID); err != nil {
		return types.RequestInfo{}, err
	}

	var preimage []byte
	preimage = append(preimage, sigHdr.Nonce...)
	preimage = append(preimage, sigHdr.Creator...)
	txID := sha256.Sum256(preimage)
	clientID := sha256.Sum256(sigHdr.Creator)
	return types.RequestInfo{
		ID:       hex.EncodeToString(txID[:]),
		ClientID: hex.EncodeToString(clientID[:]),
	}, nil
}

func (ri *RequestInspector) unwrapReq(req []byte) (*request, error) {
	envelope, err := utils2.UnmarshalEnvelope(req)
	if err != nil {
		return nil, err
	}
	payload := &common.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling payload")
	}

	if payload.Header == nil {
		return nil, errors.Errorf("no header in payload")
	}

	sigHdr := &common.SignatureHeader{}
	if err := proto.Unmarshal(payload.Header.SignatureHeader, sigHdr); err != nil {
		return nil, err
	}

	if len(payload.Header.ChannelHeader) == 0 {
		return nil, errors.New("no channel header in payload")
	}

	chdr, err := utils2.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.WithMessage(err, "error unmarshaling channel header")
	}

	return &request{
		chHdr:    chdr,
		sigHdr:   sigHdr,
		envelope: envelope,
	}, nil
}

// ConfigurationEnvelop extract configuration envelop
func ConfigurationEnvelop(configBlock *common.Block) (*common.ConfigEnvelope, error) {
	envelopeConfig, err := utils2.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract envelop from block")
	}

	payload, err := utils2.UnmarshalPayload(envelopeConfig.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal envelop payload")
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal configuration payload")
	}
	return configEnvelope, nil
}

// ConsenterCertificate denotes a TLS certificate of a consenter
type ConsenterCertificate []byte

// IsConsenterOfChannel returns whether the caller is a consenter of a channel
// by inspecting the given configuration block.
// It returns nil if true, else returns an error.
func (conCert ConsenterCertificate) IsConsenterOfChannel(configBlock *common.Block) error {
	if configBlock == nil {
		return errors.New("nil block")
	}
	envelopeConfig, err := utils2.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig)
	if err != nil {
		return err
	}
	oc, exists := bundle.OrdererConfig()
	if !exists {
		return errors.New("no orderer config in bundle")
	}
	if oc.ConsensusType() != "smartbft" {
		return errors.New("not a SmartBFT config block")
	}
	m := &smartbft.ConfigMetadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), m); err != nil {
		return err
	}

	for _, consenter := range m.Consenters {
		fmt.Println(base64.StdEncoding.EncodeToString(consenter.ServerTlsCert))
		if bytes.Equal(conCert, consenter.ServerTlsCert) || bytes.Equal(conCert, consenter.ClientTlsCert) {
			return nil
		}
	}
	return cluster.ErrNotInChannel
}

func RemoteNodesFromConfigBlock(block *common.Block, selfID uint64, logger *flogging.FabricLogger) (*nodeConfig, error) {
	env := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], env); err != nil {
		return nil, errors.Wrap(err, "failed unmarshaling envelope of config block")
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(env)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting a new bundle from envelope of config block")
	}

	oc, ok := bundle.OrdererConfig()
	if !ok {
		return nil, errors.New("no orderer config in config block")
	}

	m := &smartbft.ConfigMetadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}
	if m.Options == nil {
		return nil, errors.New("failed to retrieve consensus metadata options")
	}

	var nodeIDs []uint64
	var remoteNodes []cluster.RemoteNode
	id2Identies := map[uint64][]byte{}
	for _, consenter := range m.Consenters {
		sanitizedID, err := crypto.SanitizeIdentity(consenter.Identity)
		if err != nil {
			logger.Panicf("Failed to sanitize identity: %v", err)
		}
		id2Identies[consenter.ConsenterId] = sanitizedID
		//logger.Infof("%s %d ---> %s", bundle.ConfigtxValidator().ChainID(), consenter.ConsenterId, string(consenter.Identity)) rm for logic bug

		nodeIDs = append(nodeIDs, consenter.ConsenterId)

		// No need to know yourself
		if selfID == consenter.ConsenterId {
			continue
		}
		serverCertAsDER, err := pemToDER(consenter.ServerTlsCert, consenter.ConsenterId, "server", logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := pemToDER(consenter.ClientTlsCert, consenter.ConsenterId, "client", logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		// Validate certificate structure
		for _, cert := range [][]byte{serverCertAsDER, clientCertAsDER} {
			if _, err := x509.ParseCertificate(cert); err != nil {
				pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
				logger.Errorf("Invalid certificate: %s", string(pemBytes))
				return nil, err
			}
		}

		remoteNodes = append(remoteNodes, cluster.RemoteNode{
			ID:            consenter.ConsenterId,
			ClientTLSCert: clientCertAsDER,
			ServerTLSCert: serverCertAsDER,
			Endpoint:      fmt.Sprintf("%s:%d", consenter.Host, consenter.Port),
		})
	}

	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})

	return &nodeConfig{
		remoteNodes:   remoteNodes,
		id2Identities: id2Identies,
		nodeIDs:       nodeIDs,
	}, nil

}

type nodeConfig struct {
	id2Identities NodeIdentitiesByID
	remoteNodes   []cluster.RemoteNode
	nodeIDs       []uint64
}

// RuntimeConfig defines the configuration of the consensus
// that is related to runtime.
type RuntimeConfig struct {
	BFTConfig              types.Configuration
	isConfig               bool
	logger                 *flogging.FabricLogger
	id                     uint64
	LastCommittedBlockHash string
	RemoteNodes            []cluster.RemoteNode
	ID2Identities          NodeIdentitiesByID
	LastBlock              *common.Block
	LastConfigBlock        *common.Block
	Nodes                  []uint64
}

func (rtc RuntimeConfig) BlockCommitted(block *common.Block) (RuntimeConfig, error) {
	if _, err := cluster.ConfigFromBlock(block); err == nil {
		return rtc.configBlockCommitted(block)
	}
	return RuntimeConfig{
		BFTConfig:              rtc.BFTConfig,
		id:                     rtc.id,
		logger:                 rtc.logger,
		LastCommittedBlockHash: hex.EncodeToString(block.Header.Hash()),
		Nodes:                  rtc.Nodes,
		ID2Identities:          rtc.ID2Identities,
		RemoteNodes:            rtc.RemoteNodes,
		LastBlock:              block,
		LastConfigBlock:        rtc.LastConfigBlock,
	}, nil
}

func (rtc RuntimeConfig) configBlockCommitted(block *common.Block) (RuntimeConfig, error) {
	nodeConf, err := RemoteNodesFromConfigBlock(block, rtc.id, rtc.logger)
	if err != nil {
		return rtc, errors.Wrap(err, "remote nodes cannot be computed, rejecting config block")
	}

	bftConfig, err := configBlockToBFTConfig(rtc.id, block)
	if err != nil {
		return RuntimeConfig{}, err
	}

	return RuntimeConfig{
		BFTConfig:              bftConfig,
		isConfig:               true,
		id:                     rtc.id,
		logger:                 rtc.logger,
		LastCommittedBlockHash: hex.EncodeToString(block.Header.Hash()),
		Nodes:                  nodeConf.nodeIDs,
		ID2Identities:          nodeConf.id2Identities,
		RemoteNodes:            nodeConf.remoteNodes,
		LastBlock:              block,
		LastConfigBlock:        block,
	}, nil
}

func configBlockToBFTConfig(selfID uint64, block *common.Block) (types.Configuration, error) {
	if block == nil || block.Data == nil || len(block.Data.Data) == 0 {
		return types.Configuration{}, errors.New("empty block")
	}

	env, err := utils2.UnmarshalEnvelope(block.Data.Data[0])
	if err != nil {
		return types.Configuration{}, err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(env)
	if err != nil {
		return types.Configuration{}, err
	}

	oc, ok := bundle.OrdererConfig()
	if !ok {
		return types.Configuration{}, errors.New("no orderer config")
	}

	consensusMD := &smartbft.ConfigMetadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), consensusMD); err != nil {
		return types.Configuration{}, err
	}

	return configFromMetadataOptions(selfID, consensusMD.Options)
}

func isConfigBlock(block *common.Block) bool {
	if block.Data == nil || len(block.Data.Data) != 1 {
		return false
	}
	env, err := utils2.UnmarshalEnvelope(block.Data.Data[0])
	if err != nil {
		return false
	}
	payload, err := utils2.GetPayload(env)
	if err != nil {
		return false
	}

	if payload.Header == nil {
		return false
	}

	hdr, err := utils2.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return false
	}
	return common.HeaderType(hdr.Type) == common.HeaderType_CONFIG
}
