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
 * @brief config for the block sync
 * @file BlockSyncConfig.h
 * @author: yujiechen
 * @date 2021-05-24
 */
#pragma once
#include "bcos-sync/interfaces/BlockSyncMsgFactory.h"
#include <bcos-framework/interfaces/consensus/ConsensusInterface.h>
#include <bcos-framework/interfaces/crypto/KeyInterface.h>
#include <bcos-framework/interfaces/dispatcher/SchedulerInterface.h>
#include <bcos-framework/interfaces/front/FrontServiceInterface.h>
#include <bcos-framework/interfaces/ledger/LedgerInterface.h>
#include <bcos-framework/interfaces/protocol/BlockFactory.h>
#include <bcos-framework/interfaces/protocol/TransactionSubmitResultFactory.h>
#include <bcos-framework/interfaces/sync/SyncConfig.h>
#include <bcos-framework/interfaces/txpool/TxPoolInterface.h>
namespace bcos
{
namespace sync
{
class BlockSyncConfig : public SyncConfig
{
public:
    using Ptr = std::shared_ptr<BlockSyncConfig>;
    BlockSyncConfig(bcos::crypto::PublicPtr _nodeId, bcos::ledger::LedgerInterface::Ptr _ledger,
        bcos::txpool::TxPoolInterface::Ptr _txpool, bcos::protocol::BlockFactory::Ptr _blockFactory,
        bcos::protocol::TransactionSubmitResultFactory::Ptr _txResultFactory,
        bcos::front::FrontServiceInterface::Ptr _frontService,
        bcos::scheduler::SchedulerInterface::Ptr _scheduler,
        bcos::consensus::ConsensusInterface::Ptr _consensus, BlockSyncMsgFactory::Ptr _msgFactory)
      : SyncConfig(_nodeId),
        m_ledger(_ledger),
        m_txpool(_txpool),
        m_blockFactory(_blockFactory),
        m_txResultFactory(_txResultFactory),
        m_frontService(_frontService),
        m_scheduler(_scheduler),
        m_consensus(_consensus),
        m_msgFactory(_msgFactory)
    {}
    ~BlockSyncConfig() override {}

    bcos::ledger::LedgerInterface::Ptr ledger() { return m_ledger; }
    bcos::protocol::BlockFactory::Ptr blockFactory() { return m_blockFactory; }
    bcos::front::FrontServiceInterface::Ptr frontService() { return m_frontService; }
    bcos::scheduler::SchedulerInterface::Ptr scheduler() { return m_scheduler; }
    bcos::consensus::ConsensusInterface::Ptr consensus() { return m_consensus; }

    BlockSyncMsgFactory::Ptr msgFactory() { return m_msgFactory; }
    virtual void resetConfig(bcos::ledger::LedgerConfig::Ptr _ledgerConfig);

    bcos::crypto::HashType const& genesisHash() const { return m_genesisHash; }
    void setGenesisHash(bcos::crypto::HashType const& _hash);

    bcos::protocol::BlockNumber blockNumber() const { return m_blockNumber; }
    bcos::crypto::HashType const& hash() const;

    bcos::protocol::BlockNumber nextBlock() const { return m_nextBlock; }
    void resetBlockInfo(
        bcos::protocol::BlockNumber _blockNumber, bcos::crypto::HashType const& _hash);

    void setKnownHighestNumber(bcos::protocol::BlockNumber _highestNumber);
    bcos::protocol::BlockNumber knownHighestNumber() { return m_knownHighestNumber; }

    void setKnownLatestHash(bcos::crypto::HashType const& _hash);

    bcos::crypto::HashType const& knownLatestHash();

    size_t maxDownloadingBlockQueueSize() const { return m_maxDownloadingBlockQueueSize; }
    void setMaxDownloadingBlockQueueSize(size_t _maxDownloadingBlockQueueSize);

    void setMaxDownloadRequestQueueSize(size_t _maxDownloadRequestQueueSize);

    size_t maxDownloadRequestQueueSize() const { return m_maxDownloadRequestQueueSize; }

    size_t downloadTimeout() const { return m_downloadTimeout; }

    size_t maxRequestBlocks() const { return m_maxRequestBlocks; }
    size_t maxShardPerPeer() const { return m_maxShardPerPeer; }

    void setExecutedBlock(bcos::protocol::BlockNumber _executedBlock);
    bcos::protocol::BlockNumber executedBlock() { return m_executedBlock; }

    bcos::txpool::TxPoolInterface::Ptr txpool() { return m_txpool; }
    bcos::protocol::TransactionSubmitResultFactory::Ptr txResultFactory()
    {
        return m_txResultFactory;
    }

    void setCommittedProposalNumber(bcos::protocol::BlockNumber _committedProposalNumber)
    {
        m_committedProposalNumber = _committedProposalNumber;
    }

    bcos::protocol::BlockNumber committedProposalNumber() const
    {
        return m_committedProposalNumber;
    }

protected:
    void setHash(bcos::crypto::HashType const& _hash);

private:
    bcos::ledger::LedgerInterface::Ptr m_ledger;
    bcos::txpool::TxPoolInterface::Ptr m_txpool;
    bcos::protocol::BlockFactory::Ptr m_blockFactory;
    bcos::protocol::TransactionSubmitResultFactory::Ptr m_txResultFactory;
    bcos::front::FrontServiceInterface::Ptr m_frontService;
    bcos::scheduler::SchedulerInterface::Ptr m_scheduler;
    bcos::consensus::ConsensusInterface::Ptr m_consensus;
    BlockSyncMsgFactory::Ptr m_msgFactory;

    bcos::crypto::HashType m_genesisHash;
    std::atomic<bcos::protocol::BlockNumber> m_blockNumber = {0};
    std::atomic<bcos::protocol::BlockNumber> m_nextBlock = {0};
    std::atomic<bcos::protocol::BlockNumber> m_executedBlock = {0};
    bcos::crypto::HashType m_hash;
    mutable SharedMutex x_hash;

    std::atomic<bcos::protocol::BlockNumber> m_knownHighestNumber = {0};
    bcos::crypto::HashType m_knownLatestHash;
    mutable SharedMutex x_knownLatestHash;
    mutable Mutex m_mutex;

    std::atomic<size_t> m_maxDownloadingBlockQueueSize = 256;
    std::atomic<size_t> m_maxDownloadRequestQueueSize = 1000;
    std::atomic<size_t> m_downloadTimeout = (200 * m_maxDownloadingBlockQueueSize);
    // the max number of blocks this node can requested to
    std::atomic<size_t> m_maxRequestBlocks = {8};

    std::atomic<size_t> m_maxShardPerPeer = {2};

    std::atomic<bcos::protocol::BlockNumber> m_committedProposalNumber = {0};
};
}  // namespace sync
}  // namespace bcos