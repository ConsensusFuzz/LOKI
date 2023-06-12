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
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSmartBFT(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SmartBFT-based Ordering Service Suite")
}

var components *nwo.Components
var suiteBase = integration.SmartBFTBasePort

var _ = SynchronizedBeforeSuite(func() []byte {
	components = &nwo.Components{}
	components.Build()

	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	components.Cleanup()
})

func StartPort() int {
	return suiteBase + (GinkgoParallelNode()-1)*100
}
