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
 * @file BlockSyncConfig.cpp
 * @author: yujiechen
 * @date 2021-05-25
 */
#include "BlockSyncConfig.h"
#include "bcos-sync/utilities/Common.h"
using namespace bcos;
using namespace bcos::sync;
using namespace bcos::crypto;
using namespace bcos::protocol;
using namespace bcos::ledger;

void BlockSyncConfig::resetConfig(LedgerConfig::Ptr _ledgerConfig)
{
    if (_ledgerConfig->blockNumber() <= m_blockNumber && m_blockNumber > 0)
    {
        return;
    }
    // must resetConfig for the consensus module firstly for the following block check depends on
    // the consensus config
    m_consensus->asyncNotifyNewBlock(_ledgerConfig, [_ledgerConfig](Error::Ptr _error) {
        if (!_error)
        {
            return;
        }
        BLKSYNC_LOG(WARNING) << LOG_DESC("asyncNotifyNewBlock to consensus failed")
                             << LOG_KV("number", _ledgerConfig->blockNumber())
                             << LOG_KV("hash", _ledgerConfig->hash().abridged())
                             << LOG_KV("error", _error->errorCode())
                             << LOG_KV("msg", _error->errorMessage());
    });

    // Note: can't add lock before asyncNotifyNewBlock in case of deadlock
    Guard l(m_mutex);
    if (_ledgerConfig->blockNumber() <= m_blockNumber && m_blockNumber > 0)
    {
        return;
    }
    resetBlockInfo(_ledgerConfig->blockNumber(), _ledgerConfig->hash());
    setConsensusNodeList(_ledgerConfig->consensusNodeList());
    setObserverList(_ledgerConfig->observerNodeList());
    BLKSYNC_LOG(INFO) << LOG_DESC("BlockSyncConfig resetConfig") << LOG_KV("number", m_blockNumber)
                      << LOG_KV("consNodeSize", consensusNodeList().size())
                      << LOG_KV("observerNodeSize", observerNodeList().size());
}

void BlockSyncConfig::setGenesisHash(HashType const& _hash)
{
    m_genesisHash = _hash;
    if (knownLatestHash() == HashType())
    {
        setKnownLatestHash(m_genesisHash);
    }
}

void BlockSyncConfig::resetBlockInfo(BlockNumber _blockNumber, bcos::crypto::HashType const& _hash)
{
    m_blockNumber = _blockNumber;
    setHash(_hash);
    m_nextBlock = m_blockNumber + 1;
    if (m_knownHighestNumber < _blockNumber)
    {
        m_knownHighestNumber = _blockNumber;
        setKnownLatestHash(_hash);
    }
    if (_blockNumber > m_executedBlock)
    {
        m_executedBlock = _blockNumber;
    }
}

HashType const& BlockSyncConfig::hash() const
{
    ReadGuard l(x_hash);
    return m_hash;
}

void BlockSyncConfig::setHash(HashType const& _hash)
{
    WriteGuard l(x_hash);
    m_hash = _hash;
}

void BlockSyncConfig::setKnownHighestNumber(BlockNumber _highestNumber)
{
    m_knownHighestNumber = _highestNumber;
}

void BlockSyncConfig::setKnownLatestHash(HashType const& _hash)
{
    WriteGuard l(x_knownLatestHash);
    m_knownLatestHash = _hash;
}

HashType const& BlockSyncConfig::knownLatestHash()
{
    ReadGuard l(x_knownLatestHash);
    return m_knownLatestHash;
}

void BlockSyncConfig::setMaxDownloadingBlockQueueSize(size_t _maxDownloadingBlockQueueSize)
{
    m_maxDownloadingBlockQueueSize = _maxDownloadingBlockQueueSize;
}

void BlockSyncConfig::setMaxDownloadRequestQueueSize(size_t _maxDownloadRequestQueueSize)
{
    m_maxDownloadRequestQueueSize = _maxDownloadRequestQueueSize;
}

void BlockSyncConfig::setExecutedBlock(BlockNumber _executedBlock)
{
    if (m_blockNumber <= _executedBlock)
    {
        m_executedBlock = _executedBlock;
    }
}