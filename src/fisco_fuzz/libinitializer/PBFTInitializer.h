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
 * @brief initializer for the PBFT module
 * @file PBFTInitializer.h
 * @author: yujiechen
 * @date 2021-06-10
 */
#pragma once
#include "Common.h"
#include "Common/TarsUtils.h"
#include "libinitializer/ProtocolInitializer.h"
#include <bcos-framework/interfaces/consensus/ConsensusInterface.h>
#include <bcos-framework/interfaces/dispatcher/SchedulerInterface.h>
#include <bcos-framework/interfaces/front/FrontServiceInterface.h>
#include <bcos-framework/interfaces/multigroup/GroupInfo.h>
#include <bcos-framework/interfaces/sealer/SealerInterface.h>
#include <bcos-framework/interfaces/storage/StorageInterface.h>
#include <bcos-framework/interfaces/sync/BlockSyncInterface.h>
#include <bcos-framework/interfaces/txpool/TxPoolInterface.h>
#include <bcos-ledger/src/libledger/Ledger.h>
#include <bcos-utilities/Timer.h>

namespace bcos
{
namespace sealer
{
class Sealer;
}
namespace sync
{
class BlockSync;
}
namespace consensus
{
class PBFTImpl;
}

namespace initializer
{
class PBFTInitializer
{
public:
    using Ptr = std::shared_ptr<PBFTInitializer>;
    PBFTInitializer(bcos::initializer::NodeArchitectureType _nodeArchType,
        bcos::tool::NodeConfig::Ptr _nodeConfig, ProtocolInitializer::Ptr _protocolInitializer,
        bcos::txpool::TxPoolInterface::Ptr _txpool, std::shared_ptr<bcos::ledger::Ledger> _ledger,
        bcos::scheduler::SchedulerInterface::Ptr _scheduler,
        bcos::storage::StorageInterface::Ptr _storage,
        bcos::front::FrontServiceInterface::Ptr _frontService);

    virtual ~PBFTInitializer() { stop(); }

    virtual void init();

    virtual void start();
    virtual void stop();

    virtual void startReport();

    bcos::txpool::TxPoolInterface::Ptr txpool();
    bcos::sync::BlockSyncInterface::Ptr blockSync();
    bcos::consensus::ConsensusInterface::Ptr pbft();
    bcos::sealer::SealerInterface::Ptr sealer();

    bcos::protocol::BlockFactory::Ptr blockFactory()
    {
        return m_protocolInitializer->blockFactory();
    }
    bcos::crypto::KeyFactory::Ptr keyFactory() { return m_protocolInitializer->keyFactory(); }

    bcos::group::GroupInfo::Ptr groupInfo() { return m_groupInfo; }

protected:
    virtual void initChainNodeInfo(bcos::initializer::NodeArchitectureType _nodeArchType,
        bcos::tool::NodeConfig::Ptr _nodeConfig);
    virtual void createSealer();
    virtual void createPBFT();
    virtual void createSync();
    virtual void registerHandlers();

    virtual void reportNodeInfo();

    template <typename T, typename S>
    void asyncNotifyGroupInfo(
        std::string const& _serviceName, bcos::group::GroupInfo::Ptr _groupInfo)
    {
        auto servicePrx = Application::getCommunicator()->stringToProxy<T>(_serviceName);
        vector<EndpointInfo> activeEndPoints;
        vector<EndpointInfo> nactiveEndPoints;
        servicePrx->tars_endpointsAll(activeEndPoints, nactiveEndPoints);
        if (activeEndPoints.size() == 0)
        {
            BCOS_LOG(TRACE) << LOG_DESC("asyncNotifyGroupInfo error for empty connection")
                            << bcos::group::printGroupInfo(_groupInfo);
            return;
        }
        for (auto const& endPoint : activeEndPoints)
        {
            auto endPointStr = bcostars::endPointToString(_serviceName, endPoint.getEndpoint());
            auto servicePrx = Application::getCommunicator()->stringToProxy<T>(endPointStr);
            auto serviceClient = std::make_shared<S>(servicePrx, _serviceName);
            serviceClient->asyncNotifyGroupInfo(
                _groupInfo, [endPointStr, _groupInfo](Error::Ptr&& _error) {
                    // TODO: retry when notify failed
                    if (_error)
                    {
                        BCOS_LOG(ERROR) << LOG_DESC("asyncNotifyGroupInfo error")
                                        << LOG_KV("endPoint", endPointStr)
                                        << LOG_KV("code", _error->errorCode())
                                        << LOG_KV("msg", _error->errorMessage());
                        return;
                    }
                });
        }
    }

    std::string generateGenesisConfig(bcos::tool::NodeConfig::Ptr _nodeConfig);
    std::string generateIniConfig(bcos::tool::NodeConfig::Ptr _nodeConfig);

private:
    bcos::tool::NodeConfig::Ptr m_nodeConfig;
    ProtocolInitializer::Ptr m_protocolInitializer;

    bcos::txpool::TxPoolInterface::Ptr m_txpool;
    // Note: PBFT and other modules (except rpc and gateway) access ledger with bcos-ledger SDK
    std::shared_ptr<bcos::ledger::Ledger> m_ledger;
    bcos::scheduler::SchedulerInterface::Ptr m_scheduler;
    bcos::storage::StorageInterface::Ptr m_storage;
    bcos::front::FrontServiceInterface::Ptr m_frontService;

    std::shared_ptr<bcos::sealer::Sealer> m_sealer;
    std::shared_ptr<bcos::sync::BlockSync> m_blockSync;
    std::shared_ptr<bcos::consensus::PBFTImpl> m_pbft;

    std::shared_ptr<bcos::Timer> m_timer;
    uint64_t m_timerSchedulerInterval = 3000;

    bcos::group::GroupInfo::Ptr m_groupInfo;
};
}  // namespace initializer
}  // namespace bcos