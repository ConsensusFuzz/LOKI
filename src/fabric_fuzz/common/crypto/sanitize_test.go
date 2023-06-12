/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto_test

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

const (
	highSCACert = `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJwRENDQVV1Z0F3SUJBZ0lSQUozQm9QcytuS2J0VnBVVFAzZG5pYWt3Q2dZSUtvWkl6ajBFQXdJd01qRXcKTUM0R0ExVUVCUk1uTWpBNU5qazBNVGN6TnpJek1URXhPRFF3TkRVek5UVTNNelUwTURVNU9EUTVOVEF4TURrMwpNQjRYRFRFNU1UQXlNVEV5TlRrd05Wb1hEVEk1TVRBeE9URXlOVGt3TlZvd01qRXdNQzRHQTFVRUJSTW5NakE1Ck5qazBNVGN6TnpJek1URXhPRFF3TkRVek5UVTNNelUwTURVNU9EUTVOVEF4TURrM01Ga3dFd1lIS29aSXpqMEMKQVFZSUtvWkl6ajBEQVFjRFFnQUVZU1QxTjhHT3h2VGJnQi93eGlZbGJ5UU1rTExCNWtTTmlmSDBXaWJDK3BBbgpvMHFIOUdNWEwxK1B5RGFLUlpNUGRMQ3NCa1o4Z0NHSEJXWjZZM28xaWFOQ01FQXdEZ1lEVlIwUEFRSC9CQVFECkFnR21NQjBHQTFVZEpRUVdNQlFHQ0NzR0FRVUZCd01DQmdnckJnRUZCUWNEQVRBUEJnTlZIUk1CQWY4RUJUQUQKQVFIL01Bb0dDQ3FHU000OUJBTUNBMGNBTUVRQ0lGWkhpZGNLeG9NcDB4RTNuM0lydW5rczlLQUZlaHhlaUt6Rgo4NURHMnRGOEFpQWJkdTFwc2pWK1c0WWpGZ3pyK2N3MUxVYUlFeTVmcGZ4ZTNjU1BtUm9sL0E9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==`
	highSCert   = `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJwVENDQVVxZ0F3SUJBZ0lRV29oOFNWZnlhRFZwcDN5TkFROHdWVEFLQmdncWhrak9QUVFEQWpBeU1UQXcKTGdZRFZRUUZFeWN5TURrMk9UUXhOek0zTWpNeE1URTROREEwTlRNMU5UY3pOVFF3TlRrNE5EazFNREV3T1RjdwpIaGNOTVRreE1ESXhNVEkxT1RBMVdoY05Namt4TURFNU1USTFPVEExV2pBeU1UQXdMZ1lEVlFRRkV5Y3hNakF6Ck16a3hPVEk0TWpNd05qZ3hNamswT0RFME5qQTJNREk1TkRNd05Ua3lNVEF6TWpVd1dUQVRCZ2NxaGtqT1BRSUIKQmdncWhrak9QUU1CQndOQ0FBVGs4ci9zZ1BKL2FwL2dZakw2T0dwcWc5TmRtd3dFSlp1OXFaaDAvYXRvbFNsVQp5V3cxUDdRR283Zk5rcVdXSi8xZm5jbUZ4ZTQzOTJEVmNJZERSTENYbzBJd1FEQU9CZ05WSFE4QkFmOEVCQU1DCkJhQXdIUVlEVlIwbEJCWXdGQVlJS3dZQkJRVUhBd0lHQ0NzR0FRVUZCd01CTUE4R0ExVWRFUVFJTUFhSEJIOEEKQUFFd0NnWUlLb1pJemowRUF3SURTUUF3UmdJaEFMdXZBSjlpUWJBVEFHMFRFanlqRmhuY3kwOVczQUpJbm91eQpvVnFZL3owNUFpRUE3QVhETkNLY3c3TU92dm0zTFFrMEJsdkRPSXNkRm5hMG96Rkp4RU0vdWRzPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==`
	lowSCert    = `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJwVENDQVV1Z0F3SUJBZ0lSQVBGeXYrdzVkNjEybm95M0V5VXBYdHN3Q2dZSUtvWkl6ajBFQXdJd01qRXcKTUM0R0ExVUVCUk1uTWpreU5qTXlNakUzTkRZeU1qQXdOVGswTWpjMU5qSXhOekU0TXpVM01UYzVPVGt6TmpFeQpNQjRYRFRFNU1UQXlNVEV6TURBek5Gb1hEVEk1TVRBeE9URXpNREF6TkZvd01qRXdNQzRHQTFVRUJSTW5Nekl3Ck9UTTVOell4TkRneE9UQXpOamN6TkRRME56azNORGM0Tmprek5UQXhORGt5T1RVMU1Ga3dFd1lIS29aSXpqMEMKQVFZSUtvWkl6ajBEQVFjRFFnQUVhZ0NmSDlIS1ZHMEs3S1BUclBUQVpGMGlHZFNES3E2b3E2cG9KVUI5dFI0ZgpXRDN5cEJQZ0xrSDd6R25yL0wrVERIQnVIZGEwNHROYkVha1BwVzhCdnFOQ01FQXdEZ1lEVlIwUEFRSC9CQVFECkFnV2dNQjBHQTFVZEpRUVdNQlFHQ0NzR0FRVUZCd01DQmdnckJnRUZCUWNEQVRBUEJnTlZIUkVFQ0RBR2h3Ui8KQUFBQk1Bb0dDQ3FHU000OUJBTUNBMGdBTUVVQ0lRQ2xCb2ZiNEZRREs1TDJxdjRWMTdaWHdHVm9LQWxuK1lmMQpReVNGblZIVk1BSWdNNzd4ZVBnQ3BNQ3BsOVFyb2ROQi9TV2tCWlZ4VGdKVlpmeWJBMFR3bGcwPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==`
)

func TestSanitizeIdentity(t *testing.T) {
	extractCertFromPEM := func(cert []byte) *x509.Certificate {
		bl, _ := pem.Decode(cert)

		certificate, err := x509.ParseCertificate(bl.Bytes)
		assert.NoError(t, err)

		return certificate
	}

	t.Run("lowS stays the same", func(t *testing.T) {
		cert, err := base64.StdEncoding.DecodeString(lowSCert)
		assert.NoError(t, err)

		identity := &msp.SerializedIdentity{
			Mspid:   "SampleOrg",
			IdBytes: cert,
		}

		identityPreSanitation, err := proto.Marshal(identity)
		assert.NoError(t, err)
		identityAfterSanitation, err := crypto.SanitizeIdentity(identityPreSanitation)
		assert.NoError(t, err)
		assert.Equal(t, identityPreSanitation, identityAfterSanitation)
	})

	t.Run("highS changes, but is still verifiable under the CA", func(t *testing.T) {
		cert, err := base64.StdEncoding.DecodeString(highSCert)
		assert.NoError(t, err)

		caCert, err := base64.StdEncoding.DecodeString(highSCACert)
		assert.NoError(t, err)

		identity := &msp.SerializedIdentity{
			Mspid:   "SampleOrg",
			IdBytes: cert,
		}
		identityPreSanitation, err := proto.Marshal(identity)
		assert.NoError(t, err)
		identityAfterSanitation, err := crypto.SanitizeIdentity(identityPreSanitation)
		assert.NoError(t, err)
		assert.NotEqual(t, identityPreSanitation, identityAfterSanitation)

		err = proto.Unmarshal(identityAfterSanitation, identity)
		assert.NoError(t, err)
		certAfterSanitation := identity.IdBytes

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCert)

		_, err = extractCertFromPEM(certAfterSanitation).Verify(x509.VerifyOptions{
			Roots: certPool,
		})
	})
}
