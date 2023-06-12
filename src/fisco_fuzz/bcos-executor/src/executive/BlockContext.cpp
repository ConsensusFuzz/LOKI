/*
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
 * @brief block level context
 * @file BlockContext.h
 * @author: xingqiangbai
 * @date: 2021-05-27
 */

#include "BlockContext.h"
#include "../precompiled/Common.h"
#include "../precompiled/Utilities.h"
#include "../vm/Precompiled.h"
#include "TransactionExecutive.h"
#include "bcos-codec/abi/ContractABICodec.h"
#include "bcos-framework/interfaces/protocol/Exceptions.h"
#include "bcos-framework/interfaces/storage/StorageInterface.h"
#include "bcos-framework/interfaces/storage/Table.h"
#include "bcos-utilities/Error.h"
#include <boost/core/ignore_unused.hpp>
#include <boost/lexical_cast.hpp>
#include <boost/throw_exception.hpp>
#include <string>

using namespace bcos::executor;
using namespace bcos::protocol;
using namespace bcos::precompiled;
using namespace std;

BlockContext::BlockContext(std::shared_ptr<storage::StateStorage> storage,
    crypto::Hash::Ptr _hashImpl, bcos::protocol::BlockNumber blockNumber, h256 blockHash,
    uint64_t timestamp, int32_t blockVersion, const EVMSchedule& _schedule, bool _isWasm,
    bool _isAuthCheck)
  : m_blockNumber(blockNumber),
    m_blockHash(blockHash),
    m_timeStamp(timestamp),
    m_blockVersion(blockVersion),
    m_schedule(_schedule),
    m_isWasm(_isWasm),
    m_isAuthCheck(_isAuthCheck),
    m_storage(std::move(storage)),
    m_hashImpl(_hashImpl)
{}

BlockContext::BlockContext(std::shared_ptr<storage::StateStorage> storage,
    storage::StorageInterface::Ptr _lastStorage, crypto::Hash::Ptr _hashImpl,
    protocol::BlockHeader::ConstPtr _current, const EVMSchedule& _schedule, bool _isWasm,
    bool _isAuthCheck)
  : BlockContext(storage, _hashImpl, _current->number(), _current->hash(), _current->timestamp(),
        _current->version(), _schedule, _isWasm, _isAuthCheck)
{
    m_lastStorage = std::move(_lastStorage);
}

void BlockContext::insertExecutive(int64_t contextID, int64_t seq, ExecutiveState state)
{
    auto it = m_executives.find(std::tuple{contextID, seq});
    if (it != m_executives.end())
    {
        BOOST_THROW_EXCEPTION(
            BCOS_ERROR(-1, "Executive exists: " + boost::lexical_cast<std::string>(contextID)));
    }

    bool success;
    std::tie(it, success) = m_executives.emplace(std::tuple{contextID, seq}, std::move(state));
}

bcos::executor::BlockContext::ExecutiveState* BlockContext::getExecutive(
    int64_t contextID, int64_t seq)
{
    auto it = m_executives.find({contextID, seq});
    if (it == m_executives.end())
    {
        return nullptr;
    }

    return &it->second;
}
