/*
 * @CopyRight:
 * FISCO-BCOS is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * FISCO-BCOS is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with FISCO-BCOS.  If not, see <http://www.gnu.org/licenses/>
 * (c) 2016-2020 fisco-dev contributors.
 */
/**
 * @brief : Implement Channel network statistics
 * @file: ChannelNetworkStatHandler.h
 * @author: yujiechen
 * @date: 2020-03-22
 */
#pragma once
#include <libdevcore/ThreadPool.h>
#include <libstat/NetworkStatHandler.h>

#define CHANNEL_STAT_LOG(LEVEL) LOG(LEVEL) << LOG_BADGE("ChannelNetworkStatHandler")

namespace dev
{
namespace stat
{
class ChannelNetworkStatHandler : public std::enable_shared_from_this<ChannelNetworkStatHandler>
{
public:
    using Ptr = std::shared_ptr<ChannelNetworkStatHandler>;

    ChannelNetworkStatHandler(std::string const& _statisticName)
      : m_statisticName(_statisticName),
        m_statLogFlushThread(std::make_shared<dev::ThreadPool>("statFlush", 1)),
        m_p2pStatHandlers(std::make_shared<std::map<GROUP_ID, NetworkStatHandler::Ptr>>())
    {}
    virtual ~ChannelNetworkStatHandler() {}

    virtual void start();
    virtual void stop() { m_running.store(false); }

    void setFlushInterval(int64_t const& _flushInterval);
    int64_t flushInterval() const { return m_flushInterval; }
    void appendGroupP2PStatHandler(GROUP_ID const& _groupId, NetworkStatHandler::Ptr _handler);
    void removeGroupP2PStatHandler(GROUP_ID const& _groupId);

    virtual void updateGroupResponseTraffic(
        GROUP_ID const& _groupId, uint32_t const& _msgType, uint64_t const& _msgSize);

    void updateIncomingTrafficForRPC(GROUP_ID _groupId, uint64_t const& _msgSize);
    void updateOutgoingTrafficForRPC(GROUP_ID _groupId, uint64_t const& _msgSize);

    void updateAMOPInTraffic(int64_t const& _msgSize);
    void updateAMOPOutTraffic(int64_t const& _msgSize);

    bool running() const { return m_running; }

private:
    void flushLog();
    NetworkStatHandler::Ptr getP2PHandlerByGroupId(GROUP_ID const& _groupId);

private:
    std::string m_statisticName;
    int64_t m_flushInterval;
    dev::ThreadPool::Ptr m_statLogFlushThread;

    std::atomic<int64_t> m_AMOPIn = {0};
    std::atomic<int64_t> m_AMOPOut = {0};

    std::shared_ptr<std::map<GROUP_ID, NetworkStatHandler::Ptr>> m_p2pStatHandlers;
    mutable SharedMutex x_p2pStatHandlers;
    std::atomic_bool m_running = {false};
};
}  // namespace stat
}  // namespace dev