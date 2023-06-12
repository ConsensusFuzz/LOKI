/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderer

import (
	"math"

	"github.com/hyperledger/fabric/common/crypto"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	protossmartbft "github.com/hyperledger/fabric/protos/orderer/smartbft"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("policies.ImplicitOrderer")

type policyProvider struct {
	signaturePolicyProvider policies.Provider
	consensusType           string
	consensusMetadata       []byte
}

//go:generate mockery -dir . -name IdentityDeserializerMock -case underscore -output mocks/
type IdentityDeserializerMock interface {
	msp.IdentityDeserializer
}

//go:generate mockery -dir . -name IdentityMock -case underscore -output mocks/
type IdentityMock interface {
	msp.Identity
}

func NewPolicyProvider(
	deserializer msp.IdentityDeserializer,
	consensusType string,
	consensusMetadata []byte,
) policies.Provider {
	return &policyProvider{
		signaturePolicyProvider: cauthdsl.NewPolicyProvider(deserializer),
		consensusType:           consensusType,
		consensusMetadata:       consensusMetadata,
	}
}

func NewPolicyFromString(ruleName string) (*cb.ImplicitOrdererPolicy, error) {
	logger.Debugf("Entry: ruleName=%s", ruleName)
	ruleVal, exist := cb.ImplicitOrdererPolicy_Rule_value[ruleName]
	if !exist {
		return nil, errors.Errorf("ImplicitOrdererPolicy Rule '%v' not supported", ruleName)
	}
	return &cb.ImplicitOrdererPolicy{Rule: cb.ImplicitOrdererPolicy_Rule(ruleVal)}, nil
}

// NewPolicy creates a new policy based on the policy bytes
func (p *policyProvider) NewPolicy(data []byte) (policies.Policy, proto.Message, error) {
	definition := &cb.ImplicitOrdererPolicy{}
	if err := proto.Unmarshal(data, definition); err != nil {
		return nil, nil, errors.Wrap(err, "Error unmarshaling to ImplicitOrdererPolicy")
	}
	if definition.Rule != cb.ImplicitOrdererPolicy_SMARTBFT {
		return nil, nil, errors.Errorf("ImplicitOrdererPolicy Rule '%v' not supported", definition.Rule)
	}

	smartbftMetadata := &protossmartbft.ConfigMetadata{}
	if err := proto.Unmarshal(p.consensusMetadata, smartbftMetadata); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal smartbft metadata configuration")
	}

	var identities [][]byte
	for _, consenter := range smartbftMetadata.Consenters {
		id, err := crypto.SanitizeIdentity(consenter.Identity)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to sanitize identity")
		}
		identities = append(identities, id)
	}

	q := computeQuorum(len(identities))

	sigEnv := cauthdsl.SignedByNOutOfGivenIdentities(int32(q), identities)
	sigData, err := proto.Marshal(sigEnv)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to marshal envelope from SignedByNOutOfGivenIdentities signature policy")
	}
	sigPol, _, err := p.signaturePolicyProvider.NewPolicy(sigData)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create SignedByNOutOfGivenRole signature policy")
	}

	ip := &implicitBFTPolicy{
		sigPolicy:  sigPol,
		quorumSize: q,
	}

	logger.Debugf("Successfully created an ImplicitOrdererPolicy with rule '%s', and %d out-of %d signature policy",
		cb.ImplicitOrdererPolicy_Rule_name[int32(definition.Rule)], q, len(identities))

	return ip, nil, nil
}

type implicitBFTPolicy struct {
	sigPolicy  policies.Policy // N out of a set of explicit identities, taken from the consenter set
	quorumSize int             // The BFT quorum size
}

func (ip *implicitBFTPolicy) Evaluate(signatureSet []*cb.SignedData) error {
	logger.Debugf("Entry: signatureSet size: %d", len(signatureSet))

	if len(signatureSet) < ip.quorumSize {
		return errors.Errorf("expected at least %d signatures, but there are only %d", ip.quorumSize, len(signatureSet))
	}
	// check that quorumSize signatures are valid
	return ip.sigPolicy.Evaluate(signatureSet)
}

// computeQuorum calculates the BFT quorum size Q, given a cluster size N.
//
// The calculation satisfies the following:
// Given a cluster size of N nodes, which tolerates f failures according to:
//    f = argmax ( N >= 3f+1 )
// Q is the size of the quorum such that:
//    any two subsets q1, q2 of size Q, intersect in at least f+1 nodes.
//
// Note that this is different from N-f (the number of correct nodes), when N=3f+3. That is, we have two extra nodes
// above the minimum required to tolerate f failures.
func computeQuorum(N int) (Q int) {
	F := (N - 1) / 3
	Q = int(math.Ceil((float64(N) + float64(F) + 1) / 2.0))
	return
}
