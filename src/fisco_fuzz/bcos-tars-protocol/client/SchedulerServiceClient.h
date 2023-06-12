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
 * @file SchedulerServiceClient.h
 * @author: yujiechen
 * @date 2021-10-17
 */
#pragma once
#include "bcos-tars-protocol/tars/SchedulerService.h"
#include <bcos-framework/interfaces/crypto/CryptoSuite.h>
#include <bcos-framework/interfaces/dispatcher/SchedulerInterface.h>
#include <bcos-utilities/Common.h>
namespace bcostars
{
class SchedulerServiceClient : public bcos::scheduler::SchedulerInterface
{
public:
    SchedulerServiceClient(SchedulerServicePrx _prx, bcos::crypto::CryptoSuite::Ptr _cryptoSuite)
      : m_prx(_prx), m_cryptoSuite(_cryptoSuite)
    {}
    ~SchedulerServiceClient() override {}

    void call(bcos::protocol::Transaction::Ptr tx,
        std::function<void(bcos::Error::Ptr&&, bcos::protocol::TransactionReceipt::Ptr&&)>)
        override;

    // TODO: implement other functions
    void executeBlock(bcos::protocol::Block::Ptr, bool,
        std::function<void(bcos::Error::Ptr&&, bcos::protocol::BlockHeader::Ptr&&)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method executeBlock");
    }

    void commitBlock(bcos::protocol::BlockHeader::Ptr,
        std::function<void(bcos::Error::Ptr&&, bcos::ledger::LedgerConfig::Ptr&&)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method commitBlock");
    }

    void status(
        std::function<void(bcos::Error::Ptr&&, bcos::protocol::Session::ConstPtr&&)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method status");
    }


    void registerExecutor(std::string, bcos::executor::ParallelTransactionExecutorInterface::Ptr,
        std::function<void(bcos::Error::Ptr&&)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method registerExecutor");
    }

    void unregisterExecutor(const std::string&, std::function<void(bcos::Error::Ptr&&)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method unregisterExecutor");
    }

    void reset(std::function<void(bcos::Error::Ptr&&)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method reset");
    }

    void registerBlockNumberReceiver(
        std::function<void(bcos::protocol::BlockNumber blockNumber)>) override
    {
        BCOS_LOG(ERROR) << LOG_DESC("unimplemented method registerBlockNumberReceiver");
    }

    void getCode(std::string_view contract,
        std::function<void(bcos::Error::Ptr, bcos::bytes)> callback) override;

private:
    SchedulerServicePrx m_prx;
    bcos::crypto::CryptoSuite::Ptr m_cryptoSuite;
};
}  // namespace bcostars