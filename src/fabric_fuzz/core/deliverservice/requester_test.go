/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/deliverservice/mocks"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestTLSBinding(t *testing.T) {
	seekInfoContentTypes := []orderer.SeekInfo_SeekContentType{
		orderer.SeekInfo_BLOCK,
		orderer.SeekInfo_HEADER_WITH_SIG,
	}

	for _, contentType := range seekInfoContentTypes {
		t.Run(fmt.Sprintf("type: %v", contentType), func(t *testing.T) {
			defer ensureNoGoroutineLeak(t)()

			requester := blocksRequester{
				tls:     true,
				chainID: "testchainid",
			}

			caCert, serverTLScert, s := createABServer(t)
			orderer.RegisterAtomicBroadcastServer(s.Server(), &mockOrderer{t: t})
			go s.Start()
			defer s.Stop()
			time.Sleep(time.Second * 3)

			// Create deliver client and attempt to request block 100
			// from the ordering service
			client := createClient(t, serverTLScert, caCert)
			requester.client = client

			// Test both seekLatestFromCommitter and seekOldest

			// seekLatestFromCommitter
			requester.seekLatestFromCommitter(100, contentType)
			resp, err := requester.client.Recv()
			assert.NoError(t, err)
			block := resp.GetBlock()
			assert.Equal(t, 100, int(block.Header.Number))
			if contentType == orderer.SeekInfo_BLOCK {
				assert.Equal(t, [][]byte{{1, 2}, {3, 4}}, block.Data.Data)
			} else {
				assert.Nil(t, block.Data)
			}
			assert.Equal(t, [][]byte{{5, 6}, {7, 8}}, block.Metadata.Metadata)
			client.conn.Close()

			// seekOldest
			client = createClient(t, serverTLScert, caCert)
			requester.client = client
			requester.seekOldest(contentType)
			resp, err = requester.client.Recv()
			assert.NoError(t, err)
			assert.Equal(t, 0, int(resp.GetBlock().Header.Number))
			if contentType == orderer.SeekInfo_BLOCK {
				assert.Equal(t, [][]byte{{1, 2}, {3, 4}}, block.Data.Data)
			} else {
				assert.Nil(t, block.Data)
			}
			assert.Equal(t, [][]byte{{5, 6}, {7, 8}}, block.Metadata.Metadata)
			client.conn.Close()
		})
	}
}

func TestRequestByContent(t *testing.T) {
	type testCase struct {
		name        string
		contentType orderer.SeekInfo_SeekContentType
		height      uint64
	}

	testcases := []testCase{
		{"block-0", orderer.SeekInfo_BLOCK, 0},
		{"block-100", orderer.SeekInfo_BLOCK, 100},
		{"header-0", orderer.SeekInfo_HEADER_WITH_SIG, 0},
		{"header-100", orderer.SeekInfo_HEADER_WITH_SIG, 100},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer ensureNoGoroutineLeak(t)()

			requester := blocksRequester{
				tls:     true,
				chainID: "testchainid",
			}

			caCert, serverTLScert, s := createABServer(t)
			orderer.RegisterAtomicBroadcastServer(s.Server(), &mockOrderer{t: t})
			go s.Start()
			defer s.Stop()
			time.Sleep(time.Second * 3)

			// Create deliver client and attempt to request block 100
			// from the ordering service
			client := createClient(t, serverTLScert, caCert)
			requester.client = client

			ledgerInfoMock := &mocks.LedgerInfo{}
			ledgerInfoMock.On("LedgerHeight").Return(uint64(tc.height), nil)
			if tc.contentType == orderer.SeekInfo_BLOCK {
				requester.RequestBlocks(ledgerInfoMock)
			} else {
				requester.RequestHeaders(ledgerInfoMock)
			}
			resp, err := requester.client.Recv()
			assert.NoError(t, err)
			block := resp.GetBlock()
			assert.Equal(t, tc.height, block.Header.Number)
			if tc.contentType == orderer.SeekInfo_BLOCK {
				assert.Equal(t, [][]byte{{1, 2}, {3, 4}}, block.Data.Data)
			} else {
				assert.Nil(t, block.Data)
			}
			assert.Equal(t, [][]byte{{5, 6}, {7, 8}}, block.Metadata.Metadata)
			client.conn.Close()
		})
	}
}

// Create an AtomicBroadcastServer
func createABServer(t *testing.T) ([]byte, tls.Certificate, *comm.GRPCServer) {
	serverCert, serverKey, caCert := loadCertificates(t)
	serverTLScert, err := tls.X509KeyPair(serverCert, serverKey)
	assert.NoError(t, err)
	comm.GetCredentialSupport().SetClientCertificate(serverTLScert)
	s, err := comm.NewGRPCServer("localhost:9435", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			RequireClientCert: true,
			Key:               serverKey,
			Certificate:       serverCert,
			ClientRootCAs:     [][]byte{caCert},
			UseTLS:            true,
		},
	})
	assert.NoError(t, err)
	return caCert, serverTLScert, s
}

func loadCertificates(t *testing.T) (cert []byte, key []byte, caCert []byte) {
	var err error
	caCertFile := filepath.Join("testdata", "ca.pem")
	certFile := filepath.Join("testdata", "cert.pem")
	keyFile := filepath.Join("testdata", "key.pem")

	cert, err = ioutil.ReadFile(certFile)
	assert.NoError(t, err)
	key, err = ioutil.ReadFile(keyFile)
	assert.NoError(t, err)
	caCert, err = ioutil.ReadFile(caCertFile)
	assert.NoError(t, err)
	return
}

type mockClient struct {
	blocksprovider.BlocksDeliverer
	conn *grpc.ClientConn
}

func createClient(t *testing.T, tlsCert tls.Certificate, caCert []byte) *mockClient {
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      x509.NewCertPool(),
	}
	tlsConfig.RootCAs.AppendCertsFromPEM(caCert)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	dialOpts := []grpc.DialOption{grpc.WithBlock(), grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))}
	conn, err := grpc.DialContext(ctx, "localhost:9435", dialOpts...)
	assert.NoError(t, err)
	cl := orderer.NewAtomicBroadcastClient(conn)

	stream, err := cl.Deliver(context.Background())
	assert.NoError(t, err)
	return &mockClient{
		conn:            conn,
		BlocksDeliverer: stream,
	}
}

type mockOrderer struct {
	t *testing.T
}

func (*mockOrderer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("not implemented")
}

func (o *mockOrderer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	envelope, _ := stream.Recv()
	inspectTLSBinding := comm.NewBindingInspector(true, func(msg proto.Message) []byte {
		env, isEnvelope := msg.(*common.Envelope)
		if !isEnvelope || env == nil {
			assert.Fail(o.t, "not an envelope")
		}
		ch, err := utils.ChannelHeader(env)
		assert.NoError(o.t, err)
		return ch.TlsCertHash
	})

	err := inspectTLSBinding(stream.Context(), envelope)
	assert.NoError(o.t, err, "orderer rejected TLS binding")

	payload, err := utils.UnmarshalPayload(envelope.Payload)
	assert.NoError(o.t, err, "cannot unmarshal payload")
	seekInfo := &orderer.SeekInfo{}
	err = proto.Unmarshal(payload.Data, seekInfo)
	assert.NoError(o.t, err, "cannot unmarshal payload.Data")

	block := &common.Block{
		Header: &common.BlockHeader{
			Number: 100,
		},
		Data: &common.BlockData{
			Data: [][]byte{{1, 2}, {3, 4}},
		},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{{5, 6}, {7, 8}},
		},
	}

	if oldest := seekInfo.Start.GetOldest(); oldest != nil {
		block.Header.Number = 0
	}

	if seekInfo.ContentType == orderer.SeekInfo_HEADER_WITH_SIG {
		block.Data = nil
	}

	response := &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{
			Block: block,
		},
	}

	stream.Send(response)
	return nil
}
