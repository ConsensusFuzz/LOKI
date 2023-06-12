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
	"bytes"
	"encoding/pem"
	"fmt"
	"path"
	"reflect"

	"github.com/golang/protobuf/proto"
	proto2 "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/inactive"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/smartbft"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// CreateChainCallback creates a new chain
type CreateChainCallback func()

// ChainGetter obtains instances of ChainSupport for the given channel
type ChainGetter interface {
	// GetChain obtains the ChainSupport for the given channel.
	// Returns nil, false when the ChainSupport for the given channel
	// isn't found.
	GetChain(chainID string) *multichannel.ChainSupport
}

type PolicyManagerRetriever func(channel string) policies.Manager

// Consenter implementation of the BFT smart based consenter
type Consenter struct {
	CreateChain func(chainName string)
	cluster.InactiveChainRegistry
	GetPolicyManager PolicyManagerRetriever
	Logger           *flogging.FabricLogger
	Cert             []byte
	Comm             *cluster.Comm
	Chains           ChainGetter
	SignerSerializer signerSerializer
	Registrar        *multichannel.Registrar
	WALBaseDir       string
	ClusterDialer    *cluster.PredicateDialer
	Conf             *localconfig.TopLevel
	Metrics          *Metrics
}

// New creates Consenter of type smart bft
func New(
	icr cluster.InactiveChainRegistry,
	pmr PolicyManagerRetriever,
	signerSerializer signerSerializer,
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	r *multichannel.Registrar,
	metricsProvider metrics.Provider,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.smartbft")

	metrics := cluster.NewMetrics(metricsProvider)

	var walConfig WALConfig
	err := mapstructure.Decode(conf.Consensus, &walConfig)
	if err != nil {
		logger.Panicf("Failed to decode consensus configuration: %s", err)
	}

	logger.Infof("WAL Directory is %s", walConfig.WALDir)

	consenter := &Consenter{
		InactiveChainRegistry: icr,
		Registrar:             r,
		GetPolicyManager:      pmr,
		Conf:                  conf,
		ClusterDialer:         clusterDialer,
		Logger:                logger,
		Cert:                  srvConf.SecOpts.Certificate,
		Chains:                r,
		SignerSerializer:      signerSerializer,
		WALBaseDir:            walConfig.WALDir,
		Metrics:               NewMetrics(metricsProvider),
		CreateChain:           r.CreateChain,
	}

	consenter.Comm = &cluster.Comm{
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		SendBufferSize:                   conf.General.Cluster.SendBufferSize,
		Logger:                           flogging.MustGetLogger("orderer.common.cluster"),
		Chan2Members:                     make(map[string]cluster.MemberMapping),
		Connections:                      cluster.NewConnectionStore(clusterDialer, metrics.EgressTLSConnectionCount),
		Metrics:                          metrics,
		ChanExt:                          consenter,
		H: &Ingreess{
			Logger:        logger,
			ChainSelector: consenter,
		},
	}

	svc := &cluster.Service{
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: metrics,
		},
		StepLogger: flogging.MustGetLogger("orderer.common.cluster.step"),
		Logger:     flogging.MustGetLogger("orderer.common.cluster"),
		Dispatcher: consenter.Comm,
	}

	orderer.RegisterClusterServer(srv.Server(), svc)

	return consenter
}

// ReceiverByChain returns the MessageReceiver for the given channelID or nil
// if not found.
func (c *Consenter) ReceiverByChain(channelID string) MessageReceiver {
	cs := c.Chains.GetChain(channelID)
	if cs == nil {
		return nil
	}
	if cs.Chain == nil {
		c.Logger.Panicf("Programming error - Chain %s is nil although it exists in the mapping", channelID)
	}
	if smartBFTChain, isBFTSmart := cs.Chain.(*BFTChain); isBFTSmart {
		return smartBFTChain
	}
	c.Logger.Warningf("Chain %s is of type %v and not smartbft.Chain", channelID, reflect.TypeOf(cs.Chain))
	return nil
}

func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	m := &smartbft.ConfigMetadata{}
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}
	if m.Options == nil {
		return nil, errors.New("failed to retrieve consensus metadata options")
	}

	selfID, err := c.detectSelfID(m.Consenters)
	if err != nil {
		errNotServiced := fmt.Sprintf("channel %s is not serviced by me", support.ChainID())
		c.Logger.Errorf(errNotServiced)
		c.InactiveChainRegistry.TrackChain(support.ChainID(), support.Block(0), func() {
			c.CreateChain(support.ChainID())
		})
		return &inactive.Chain{Err: errors.Errorf("channel %s is not serviced by me", support.ChainID())}, nil
	}
	c.Logger.Infof("Local consenter id is %d", selfID)

	puller, err := newBlockPuller(support, c.ClusterDialer, c.Conf.General.Cluster)
	if err != nil {
		c.Logger.Panicf("Failed initializing block puller")
	}

	config, err := configFromMetadataOptions(selfID, m.Options)
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing smartbft configuration")
	}
	c.Logger.Debugf("SmartBFT-Go config: %+v", config)

	configValidator := &ConfigBlockValidator{
		ChannelConfigTemplator: c.Registrar,
		ValidatingChannel:      support.ChainID(),
		Filters:                c.Registrar,
		ConfigUpdateProposer:   c.Registrar,
		Logger:                 c.Logger,
	}

	chain, err := NewChain(configValidator, selfID, config, path.Join(c.WALBaseDir, support.ChainID()), puller, c.Comm, c.SignerSerializer, c.GetPolicyManager(support.ChainID()), support, c.Metrics)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating a new BFTChain")
	}

	return chain, nil
}

func pemToDER(pemBytes []byte, id uint64, certType string, logger *flogging.FabricLogger) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

func (c *Consenter) detectSelfID(consenters []*smartbft.Consenter) (uint64, error) {
	var serverCertificates []string
	for _, cst := range consenters {
		serverCertificates = append(serverCertificates, string(cst.ServerTlsCert))
		if bytes.Equal(c.Cert, cst.ServerTlsCert) {
			return cst.ConsenterId, nil
		}
	}

	c.Logger.Warning("Could not find", string(c.Cert), "among", serverCertificates)
	return 0, cluster.ErrNotInChannel
}

// TargetChannel extracts the channel from the given proto.Message.
// Returns an empty string on failure.
func (c *Consenter) TargetChannel(message proto2.Message) string {
	switch req := message.(type) {
	case *orderer.ConsensusRequest:
		return req.Channel
	case *orderer.SubmitRequest:
		return req.Channel
	default:
		return ""
	}
}
