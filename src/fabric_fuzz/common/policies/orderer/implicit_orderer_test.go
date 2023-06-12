/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderer

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/policies/orderer/mocks"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	protosmsp "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/orderer/smartbft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	highSCert = `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJwVENDQVVxZ0F3SUJBZ0lRV29oOFNWZnlhRFZwcDN5TkFROHdWVEFLQmdncWhrak9QUVFEQWpBeU1UQXcKTGdZRFZRUUZFeWN5TURrMk9UUXhOek0zTWpNeE1URTROREEwTlRNMU5UY3pOVFF3TlRrNE5EazFNREV3T1RjdwpIaGNOTVRreE1ESXhNVEkxT1RBMVdoY05Namt4TURFNU1USTFPVEExV2pBeU1UQXdMZ1lEVlFRRkV5Y3hNakF6Ck16a3hPVEk0TWpNd05qZ3hNamswT0RFME5qQTJNREk1TkRNd05Ua3lNVEF6TWpVd1dUQVRCZ2NxaGtqT1BRSUIKQmdncWhrak9QUU1CQndOQ0FBVGs4ci9zZ1BKL2FwL2dZakw2T0dwcWc5TmRtd3dFSlp1OXFaaDAvYXRvbFNsVQp5V3cxUDdRR283Zk5rcVdXSi8xZm5jbUZ4ZTQzOTJEVmNJZERSTENYbzBJd1FEQU9CZ05WSFE4QkFmOEVCQU1DCkJhQXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CTUE4R0ExVWRFUVFJTUFhSEJIOEEKQUFFd0NnWUlLb1pJemowRUF3SURTUUF3UmdJaEFMdXZBSjlpUWJBVEFHMFRFanlqRmhuY3kwOVczQUpJbm91eQpvVnFZL3owNUFpRUE3QVhETkNLY3c3TU92dm0zTFFrMEJsdkRPSXNkRm5hMG96Rkp4RU0vdWRzPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==`
	lowSCert  = `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJwVENDQVV1Z0F3SUJBZ0lSQVBGeXYrdzVkNjEybm95M0V5VXBYdHN3Q2dZSUtvWkl6ajBFQXdJd01qRXcKTUM0R0ExVUVCUk1uTWpreU5qTXlNakUzTkRZeU1qQXdOVGswTWpjMU5qSXhOekU0TXpVM01UYzVPVGt6TmpFeQpNQjRYRFRFNU1UQXlNVEV6TURBek5Gb1hEVEk1TVRBeE9URXpNREF6TkZvd01qRXdNQzRHQTFVRUJSTW5Nekl3Ck9UTTVOell4TkRneE9UQXpOamN6TkRRME56azNORGM0Tmprek5UQXhORGt5T1RVMU1Ga3dFd1lIS29aSXpqMEMKQVFZSUtvWkl6ajBEQVFjRFFnQUVhZ0NmSDlIS1ZHMEs3S1BUclBUQVpGMGlHZFNES3E2b3E2cG9KVUI5dFI0ZgpXRDN5cEJQZ0xrSDd6R25yL0wrVERIQnVIZGEwNHROYkVha1BwVzhCdnFOQ01FQXdEZ1lEVlIwUEFRSC9CQVFECkFnV2dNQjBHQTFVZEpRUVdNQlFHQ0NzR0FRVUZCd01DQmdnckJnRUZCUWNEQVRBUEJnTlZIUkVFQ0RBR2h3Ui8KQUFBQk1Bb0dDQ3FHU000OUJBTUNBMGdBTUVVQ0lRQ2xCb2ZiNEZRREs1TDJxdjRWMTdaWHdHVm9LQWxuK1lmMQpReVNGblZIVk1BSWdNNzd4ZVBnQ3BNQ3BsOVFyb2ROQi9TV2tCWlZ4VGdKVlpmeWJBMFR3bGcwPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==`
)

func TestNewPolicyProvider(t *testing.T) {
	deserializer := &mocks.IdentityDeserializerMock{}
	metadata, _, _ := prepareSmartBFTMetadata(4)
	provider := NewPolicyProvider(deserializer, "smartbft", metadata)
	assert.NotNil(t, provider)
}

func TestPolicyProvider_NewPolicy(t *testing.T) {
	deserializer := &mocks.IdentityDeserializerMock{}
	metadata, _, _ := prepareSmartBFTMetadata(4)
	provider := NewPolicyProvider(deserializer, "smartbft", metadata)

	t.Run("bad policy bytes", func(t *testing.T) {
		p, _, err := provider.NewPolicy([]byte{1, 2, 3, 4})
		assert.Nil(t, p)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Error unmarshaling to ImplicitOrdererPolicy")
	})

	t.Run("good", func(t *testing.T) {
		policyBytes, _ := proto.Marshal(&common.ImplicitOrdererPolicy{Rule: common.ImplicitOrdererPolicy_SMARTBFT})
		p, _, err := provider.NewPolicy(policyBytes)
		assert.NotNil(t, p)
		assert.NoError(t, err)
		assert.Equal(t, p.(*implicitBFTPolicy).quorumSize, computeQuorum(4))
	})
}

func TestImplicitBFTPolicy_Evaluate(t *testing.T) {
	metadata, consenters, identities := prepareSmartBFTMetadata(4)
	data := []byte{1, 2, 3}
	sig1 := []byte{1, 8, 9}
	sig2 := []byte{2, 8, 9}
	sig3 := []byte{3, 8, 9}
	sig4 := []byte{4, 8, 9}
	sd1 := &common.SignedData{Data: data, Identity: consenters[0].Identity, Signature: sig1}
	sd2 := &common.SignedData{Data: data, Identity: consenters[1].Identity, Signature: sig2}
	sd3 := &common.SignedData{Data: data, Identity: consenters[2].Identity, Signature: sig3}
	sd4 := &common.SignedData{Data: data, Identity: consenters[3].Identity, Signature: sig4}

	t.Run("no signed data", func(t *testing.T) {
		deserializer := &mocks.IdentityDeserializerMock{}
		provider := NewPolicyProvider(deserializer, "smartbft", metadata)
		policyBytes, _ := proto.Marshal(&common.ImplicitOrdererPolicy{Rule: common.ImplicitOrdererPolicy_SMARTBFT})
		p, _, err := provider.NewPolicy(policyBytes)
		assert.NotNil(t, p)
		assert.NoError(t, err)
		err = p.Evaluate(nil)
		assert.EqualError(t, err, "expected at least 3 signatures, but there are only 0")
	})

	t.Run("3 good sigs, 3 out of 4", func(t *testing.T) {
		deserializer := &mocks.IdentityDeserializerMock{}
		provider := NewPolicyProvider(deserializer, "smartbft", metadata)
		policyBytes, _ := proto.Marshal(&common.ImplicitOrdererPolicy{Rule: common.ImplicitOrdererPolicy_SMARTBFT})
		p, _, err := provider.NewPolicy(policyBytes)
		assert.NotNil(t, p)
		assert.NoError(t, err)

		mockID1 := &mocks.IdentityMock{}
		mockID2 := &mocks.IdentityMock{}
		mockID3 := &mocks.IdentityMock{}

		deserializer.On("DeserializeIdentity", consenters[0].Identity).Return(mockID1, nil)
		deserializer.On("DeserializeIdentity", consenters[1].Identity).Return(mockID2, nil)
		deserializer.On("DeserializeIdentity", consenters[2].Identity).Return(mockID3, nil)

		mockID1.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID1", Mspid: identities[0].Mspid})
		mockID1.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID1.On("Verify", data, sig1).Return(nil)

		mockID2.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID2", Mspid: identities[1].Mspid})
		mockID2.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID2.On("Verify", data, sig2).Return(nil)

		mockID3.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID3", Mspid: identities[2].Mspid})
		mockID3.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID3.On("Verify", data, sig3).Return(nil)

		signatureSet := []*common.SignedData{sd1, sd2, sd3}
		err = p.Evaluate(signatureSet)
		assert.NoError(t, err)
	})

	t.Run("2 good sigs 1 bad, 3 out of 4", func(t *testing.T) {
		deserializer := &mocks.IdentityDeserializerMock{}

		provider := NewPolicyProvider(deserializer, "smartbft", metadata)
		policyBytes, _ := proto.Marshal(&common.ImplicitOrdererPolicy{Rule: common.ImplicitOrdererPolicy_SMARTBFT})
		p, _, err := provider.NewPolicy(policyBytes)
		assert.NotNil(t, p)
		assert.NoError(t, err)

		err = p.Evaluate(nil)
		assert.EqualError(t, err, "expected at least 3 signatures, but there are only 0")

		mockID1 := &mocks.IdentityMock{}
		mockID2 := &mocks.IdentityMock{}
		mockID3 := &mocks.IdentityMock{}

		deserializer.On("DeserializeIdentity", consenters[0].Identity).Return(mockID1, nil)
		deserializer.On("DeserializeIdentity", consenters[1].Identity).Return(mockID2, nil)
		deserializer.On("DeserializeIdentity", consenters[2].Identity).Return(mockID3, nil)

		mockID1.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID1", Mspid: identities[0].Mspid})
		mockID1.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID1.On("Verify", data, sig1).Return(nil)

		mockID2.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID2", Mspid: identities[1].Mspid})
		mockID2.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID2.On("Verify", data, sig2).Return(nil)

		mockID3.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID3", Mspid: identities[2].Mspid})
		mockID3.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID3.On("Verify", data, sig3).Return(errors.New("bad sig"))

		signatureSet := []*common.SignedData{sd1, sd2, sd3}
		err = p.Evaluate(signatureSet)
		assert.EqualError(t, err, "signature set did not satisfy policy")
	})

	t.Run("3 good sigs 1 bad, 3 out of 4", func(t *testing.T) {
		deserializer := &mocks.IdentityDeserializerMock{}

		provider := NewPolicyProvider(deserializer, "smartbft", metadata)
		policyBytes, _ := proto.Marshal(&common.ImplicitOrdererPolicy{Rule: common.ImplicitOrdererPolicy_SMARTBFT})
		p, _, err := provider.NewPolicy(policyBytes)
		assert.NotNil(t, p)
		assert.NoError(t, err)

		err = p.Evaluate(nil)
		assert.EqualError(t, err, "expected at least 3 signatures, but there are only 0")

		mockID1 := &mocks.IdentityMock{}
		mockID2 := &mocks.IdentityMock{}
		mockID3 := &mocks.IdentityMock{}
		mockID4 := &mocks.IdentityMock{}

		deserializer.On("DeserializeIdentity", consenters[0].Identity).Return(mockID1, nil)
		deserializer.On("DeserializeIdentity", consenters[1].Identity).Return(mockID2, nil)
		deserializer.On("DeserializeIdentity", consenters[2].Identity).Return(mockID3, nil)
		deserializer.On("DeserializeIdentity", consenters[3].Identity).Return(mockID4, nil)

		mockID1.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID1", Mspid: identities[0].Mspid})
		mockID1.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID1.On("Verify", data, sig1).Return(errors.New("bad sig"))

		mockID2.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID2", Mspid: identities[1].Mspid})
		mockID2.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID2.On("Verify", data, sig2).Return(nil)

		mockID3.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID3", Mspid: identities[2].Mspid})
		mockID3.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID3.On("Verify", data, sig3).Return(nil)

		mockID4.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "ID4", Mspid: identities[3].Mspid})
		mockID4.On("SatisfiesPrincipal", mock.Anything).Return(nil)
		mockID4.On("Verify", data, sig4).Return(nil)

		signatureSet := []*common.SignedData{sd1, sd2, sd3, sd4}
		err = p.Evaluate(signatureSet)
		assert.NoError(t, err)
	})

}

func prepareSmartBFTMetadata(numNodes int) ([]byte, []*smartbft.Consenter, []*protosmsp.SerializedIdentity) {
	var consenters []*smartbft.Consenter
	var identities []*protosmsp.SerializedIdentity
	for i := 1; i <= numNodes; i++ {
		mspid := fmt.Sprintf("OrdererOrg-%d.MSPID", i)
		var certBytes []byte
		if i%2 == 0 {
			certBytes, _ = base64.StdEncoding.DecodeString(lowSCert)
		} else {
			certBytes, _ = base64.StdEncoding.DecodeString(highSCert)
		}
		identity := &protosmsp.SerializedIdentity{
			Mspid:   mspid,
			IdBytes: certBytes,
		}
		identityPreSanitation := utils.MarshalOrPanic(identity)

		consenter := &smartbft.Consenter{
			ConsenterId:   uint64(i),
			Host:          "example.org",
			Port:          uint32(8100 + i),
			MspId:         mspid,
			Identity:      identityPreSanitation,
			ClientTlsCert: []byte("/bogus"),
			ServerTlsCert: []byte("/bogus"),
		}
		consenters = append(consenters, consenter)
		identities = append(identities, identity)
	}

	metadata := &smartbft.ConfigMetadata{
		Consenters: consenters,
		Options: &smartbft.Options{
			RequestBatchMaxCount:      uint64(100),
			RequestBatchMaxBytes:      uint64(1000000),
			RequestBatchMaxInterval:   "50ms",
			IncomingMessageBufferSize: uint64(200),
			RequestPoolSize:           uint64(400),
			RequestForwardTimeout:     "2s",
			RequestComplainTimeout:    "10s",
			RequestAutoRemoveTimeout:  "1m",
			ViewChangeResendInterval:  "5s",
			ViewChangeTimeout:         "20s",
			LeaderHeartbeatTimeout:    "30s",
			LeaderHeartbeatCount:      uint64(10),
			CollectTimeout:            "1m",
			SyncOnStart:               false,
			SpeedUpViewChange:         false,
		},
	}

	metadataBytes := utils.MarshalOrPanic(metadata)
	return metadataBytes, consenters, identities
}
