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
 * @brief application for TxPoolService
 * @file TxPoolServiceApp.cpp
 * @author: yujiechen
 * @date 2021-10-17
 */
#include "TxPoolServiceApp.h"
#include <bcos-ledger/libledger/Ledger.h>
#include <bcos-tars-protocol/client/FrontServiceClient.h>
#include <bcos-tars-protocol/client/PBFTServiceClient.h>

using namespace bcostars;
using namespace bcos::initializer;
using namespace bcos;
using namespace bcos::tool;

void TxPoolServiceApp::initialize()
{
    initConfig();
    initService();
    TxPoolServiceParam param;
    param.txPoolInitializer = m_txpoolInitializer;
    addServantWithParams<TxPoolServiceServer, TxPoolServiceParam>(
        getProxyDesc(bcos::protocol::TXPOOL_SERVANT_NAME), param);
}

void TxPoolServiceApp::initService()
{
    // init log
    boost::property_tree::ptree pt;
    boost::property_tree::read_ini(m_iniConfigPath, pt);
    m_logInitializer = std::make_shared<BoostLogInitializer>();
    m_logInitializer->setLogPath(getLogPath());
    m_logInitializer->initLog(pt);

    // load iniConfig
    auto nodeConfig =
        std::make_shared<NodeConfig>(std::make_shared<bcos::crypto::KeyFactoryImpl>());
    nodeConfig->loadConfig(m_iniConfigPath);

    // init the protocol
    auto protocolInitializer = std::make_shared<ProtocolInitializer>();
    protocolInitializer->init(nodeConfig);
    protocolInitializer->loadKeyPair(m_privateKeyPath);
    nodeConfig->loadNodeServiceConfig(protocolInitializer->keyPair()->publicKey()->hex(), pt);
    nodeConfig->loadServiceConfig(pt);
    // create frontService
    auto frontServiceProxy =
        Application::getCommunicator()->stringToProxy<bcostars::FrontServicePrx>(
            nodeConfig->frontServiceName());
    auto frontService = std::make_shared<bcostars::FrontServiceClient>(
        frontServiceProxy, protocolInitializer->keyFactory());

    // create the ledger
    // TODO: modify ledger to LedgerServiceClient and implement the ledger client interfaces with
    // tars protocol
    auto ledger =
        std::make_shared<bcos::ledger::Ledger>(protocolInitializer->blockFactory(), nullptr);

    // create txpoolInitializer
    m_txpoolInitializer =
        std::make_shared<TxPoolInitializer>(nodeConfig, protocolInitializer, frontService, ledger);

    // init the txpool
    auto pbftPrx = Application::getCommunicator()->stringToProxy<PBFTServicePrx>(
        nodeConfig->consensusServiceName());
    auto sealer = std::make_shared<PBFTServiceClient>(pbftPrx);
    m_txpoolInitializer->init(sealer);
    m_txpoolInitializer->start();
}