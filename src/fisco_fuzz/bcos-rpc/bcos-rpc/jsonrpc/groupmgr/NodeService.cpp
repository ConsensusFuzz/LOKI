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
 * @brief NodeService
 * @file NodeService.cpp
 * @author: yujiechen
 * @date 2021-10-11
 */
#include "NodeService.h"
#include "Common.h"
#include <bcos-crypto/signature/key/KeyFactoryImpl.h>
#include <bcos-framework/interfaces/protocol/ServiceDesc.h>
#include <bcos-tars-protocol/client/LedgerServiceClient.h>
#include <bcos-tars-protocol/client/PBFTServiceClient.h>
#include <bcos-tars-protocol/client/SchedulerServiceClient.h>
#include <bcos-tars-protocol/client/TxPoolServiceClient.h>
#include <tarscpp/servant/Application.h>
using namespace bcos;
using namespace bcos::rpc;
using namespace bcos::crypto;
using namespace bcos::group;
using namespace bcos::protocol;

NodeService::Ptr NodeServiceFactory::buildNodeService(
    std::string const&, std::string const&, bcos::group::ChainNodeInfo::Ptr _nodeInfo)
{
    auto appName = _nodeInfo->nodeName();
    // create cryptoSuite
    auto const& type = _nodeInfo->nodeType();
    CryptoSuite::Ptr cryptoSuite = nullptr;
    if (type == NodeType::SM_NODE)
    {
        cryptoSuite = createSMCryptoSuite();
    }
    else
    {
        cryptoSuite = createCryptoSuite();
    }
    auto keyFactory = std::make_shared<bcos::crypto::KeyFactoryImpl>();
    cryptoSuite->setKeyFactory(keyFactory);

    auto blockFactory = createBlockFactory(cryptoSuite);
    auto ledgerClient = createServicePrx<bcostars::LedgerServiceClient, bcostars::LedgerServicePrx>(
        LEDGER, _nodeInfo, blockFactory);
    if (!ledgerClient.first)
    {
        return nullptr;
    }
    auto schedulerClient =
        createServicePrx<bcostars::SchedulerServiceClient, bcostars::SchedulerServicePrx>(
            SCHEDULER, _nodeInfo, cryptoSuite);
    if (!schedulerClient.first)
    {
        return nullptr;
    }
    // create txpool client
    auto txpoolClient = createServicePrx<bcostars::TxPoolServiceClient, bcostars::TxPoolServicePrx>(
        TXPOOL, _nodeInfo, cryptoSuite, blockFactory);
    if (!txpoolClient.first)
    {
        return nullptr;
    }

    // create consensus client
    auto consensusClient = createServicePrx<bcostars::PBFTServiceClient, bcostars::PBFTServicePrx>(
        CONSENSUS, _nodeInfo);
    if (!consensusClient.first)
    {
        return nullptr;
    }
    // create sync client
    auto syncClient = createServicePrx<bcostars::BlockSyncServiceClient, bcostars::PBFTServicePrx>(
        CONSENSUS, _nodeInfo);
    if (!syncClient.first)
    {
        return nullptr;
    }
    auto nodeService = std::make_shared<NodeService>(ledgerClient.first, schedulerClient.first,
        txpoolClient.first, consensusClient.first, syncClient.first, blockFactory);

    nodeService->setLedgerPrx(ledgerClient.second);
    return nodeService;
}