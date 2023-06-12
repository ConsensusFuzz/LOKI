/**
 *  Copyright (C) 2021 FISCO BCOS.
 *  SPDX-License-Identifier: Apache-2.0
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 * @brief test for gateway
 * @file GatewayConfigTest.cpp
 * @author: octopus
 * @date 2021-05-17
 */

#include <bcos-gateway/GatewayConfig.h>
#include <bcos-gateway/GatewayFactory.h>
#include <bcos-utilities/testutils/TestPromptFixture.h>
#include <boost/filesystem.hpp>
#include <boost/test/unit_test.hpp>

using namespace bcos;
using namespace gateway;
using namespace bcos::test;

BOOST_FIXTURE_TEST_SUITE(GatewayConfigTest, TestPromptFixture)

BOOST_AUTO_TEST_CASE(test_validPort)
{
    auto config = std::make_shared<GatewayConfig>();
    BOOST_CHECK(!config->isValidPort(1024));
    BOOST_CHECK(!config->isValidPort(65536));
    BOOST_CHECK(config->isValidPort(30300));
}

BOOST_AUTO_TEST_CASE(test_hostAndPort2Endpoint)
{
    auto config = std::make_shared<GatewayConfig>();

    {
        NodeIPEndpoint endpoint;
        BOOST_CHECK_NO_THROW(config->hostAndPort2Endpoint("127.0.0.1:1111", endpoint));
        BOOST_CHECK_EQUAL(endpoint.address(), "127.0.0.1");
        BOOST_CHECK_EQUAL(endpoint.port(), 1111);
        BOOST_CHECK(!endpoint.isIPv6());
    }

    {
        NodeIPEndpoint endpoint;
        BOOST_CHECK_NO_THROW(config->hostAndPort2Endpoint("[::1]:1234", endpoint));
        BOOST_CHECK_EQUAL(endpoint.address(), "::1");
        BOOST_CHECK_EQUAL(endpoint.port(), 1234);
        BOOST_CHECK(endpoint.isIPv6());
    }

    {
        NodeIPEndpoint endpoint;
        BOOST_CHECK_NO_THROW(config->hostAndPort2Endpoint("8.129.188.218:12345", endpoint));
        BOOST_CHECK_EQUAL(endpoint.address(), "8.129.188.218");
        BOOST_CHECK_EQUAL(endpoint.port(), 12345);
        BOOST_CHECK(!endpoint.isIPv6());
    }

    {
        NodeIPEndpoint endpoint;
        BOOST_CHECK_NO_THROW(
            config->hostAndPort2Endpoint("[fe80::1a9d:50ae:3207:80d9]:54321", endpoint));
        BOOST_CHECK_EQUAL(endpoint.address(), "fe80::1a9d:50ae:3207:80d9");
        BOOST_CHECK_EQUAL(endpoint.port(), 54321);
        BOOST_CHECK(endpoint.isIPv6());
    }

    {
        NodeIPEndpoint endpoint;
        BOOST_CHECK_THROW(config->hostAndPort2Endpoint("abcdef:fff", endpoint), std::exception);
    }
}

BOOST_AUTO_TEST_CASE(test_nodesJsonParser)
{
    {
        std::string json =
            "{\"nodes\":[\"127.0.0.1:30300\",\"127.0.0.1:30301\","
            "\"127.0.0.1:30302\"]}";
        auto config = std::make_shared<GatewayConfig>();
        std::set<NodeIPEndpoint> nodeIPEndpointSet;
        config->parseConnectedJson(json, nodeIPEndpointSet);
        BOOST_CHECK_EQUAL(nodeIPEndpointSet.size(), 3);
        BOOST_CHECK_EQUAL(config->threadPoolSize(), 16);
    }

    {
        std::string json = "{\"nodes\":[]}";
        auto config = std::make_shared<GatewayConfig>();
        std::set<NodeIPEndpoint> nodeIPEndpointSet;
        config->parseConnectedJson(json, nodeIPEndpointSet);
        BOOST_CHECK_EQUAL(nodeIPEndpointSet.size(), 0);
        BOOST_CHECK_EQUAL(config->threadPoolSize(), 16);
    }

    {
        std::string json =
            "{\"nodes\":[\"["
            "fe80::1a9d:50ae:3207:80d9]:30302\","
            "\"[fe80::1a9d:50ae:3207:80d9]:30303\"]}";
        auto config = std::make_shared<GatewayConfig>();
        std::set<NodeIPEndpoint> nodeIPEndpointSet;
        config->parseConnectedJson(json, nodeIPEndpointSet);
        BOOST_CHECK_EQUAL(nodeIPEndpointSet.size(), 2);
        BOOST_CHECK_EQUAL(config->threadPoolSize(), 16);
    }
}

BOOST_AUTO_TEST_CASE(test_initConfig)
{
    {
        std::string configIni("../../../bcos-gateway/test/unittests/data/config/config_ipv4.ini");
        auto config = std::make_shared<GatewayConfig>();
        config->initConfig(configIni);
        config->loadP2pConnectedNodes();

        BOOST_CHECK_EQUAL(config->listenIP(), "127.0.0.1");
        BOOST_CHECK_EQUAL(config->listenPort(), 12345);
        BOOST_CHECK_EQUAL(config->smSSL(), false);
        BOOST_CHECK_EQUAL(config->connectedNodes().size(), 3);

        auto certConfig = config->certConfig();
        BOOST_CHECK(!certConfig.caCert.empty());
        BOOST_CHECK(!certConfig.nodeCert.empty());
        BOOST_CHECK(!certConfig.nodeKey.empty());
    }
}

BOOST_AUTO_TEST_CASE(test_initSMConfig)
{
    {
        std::string configIni("../../../bcos-gateway/test/unittests/data/config/config_ipv6.ini");

        auto config = std::make_shared<GatewayConfig>();
        config->initConfig(configIni);
        config->loadP2pConnectedNodes();

        BOOST_CHECK_EQUAL(config->listenIP(), "0.0.0.0");
        BOOST_CHECK_EQUAL(config->listenPort(), 54321);
        BOOST_CHECK_EQUAL(config->smSSL(), true);
        BOOST_CHECK_EQUAL(config->connectedNodes().size(), 1);

        auto smCertConfig = config->smCertConfig();
        BOOST_CHECK(!smCertConfig.caCert.empty());
        BOOST_CHECK(!smCertConfig.nodeCert.empty());
        BOOST_CHECK(!smCertConfig.nodeKey.empty());
        BOOST_CHECK(!smCertConfig.enNodeCert.empty());
        BOOST_CHECK(!smCertConfig.enNodeKey.empty());
    }
}

BOOST_AUTO_TEST_SUITE_END()
