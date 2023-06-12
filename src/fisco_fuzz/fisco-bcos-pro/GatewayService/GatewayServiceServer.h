#pragma once

#include "Common/TarsUtils.h"
#include "GatewayService/GatewayInitializer.h"
#include "libinitializer/ProtocolInitializer.h"
#include <bcos-tars-protocol/Common.h>
#include <bcos-tars-protocol/ErrorConverter.h>
#include <bcos-tars-protocol/tars/GatewayService.h>
#include <chrono>
#include <mutex>

using namespace bcos;
using namespace bcos::group;

namespace bcostars
{
struct GatewayServiceParam
{
    GatewayInitializer::Ptr gatewayInitializer;
};
class GatewayServiceServer : public bcostars::GatewayService
{
public:
    GatewayServiceServer(GatewayServiceParam const& _param)
      : m_gatewayInitializer(_param.gatewayInitializer)
    {}
    void initialize() override {}
    void destroy() override {}

    bcostars::Error asyncSendBroadcastMessage(const std::string& groupID,
        const vector<tars::Char>& srcNodeID, const vector<tars::Char>& payload,
        tars::TarsCurrentPtr current) override
    {
        current->setResponse(false);
        auto bcosNodeID = m_gatewayInitializer->keyFactory()->createKey(
            bcos::bytesConstRef((const bcos::byte*)srcNodeID.data(), srcNodeID.size()));
        m_gatewayInitializer->gateway()->asyncSendBroadcastMessage(groupID, bcosNodeID,
            bcos::bytesConstRef((const bcos::byte*)payload.data(), payload.size()));

        async_response_asyncSendBroadcastMessage(current, toTarsError(nullptr));
        return bcostars::Error();
    }

    bcostars::Error asyncGetPeers(bcostars::GatewayInfo&, std::vector<bcostars::GatewayInfo>&,
        tars::TarsCurrentPtr current) override
    {
        GATEWAYSERVICE_LOG(DEBUG) << LOG_DESC("asyncGetPeers") << LOG_DESC("request");
        current->setResponse(false);
        m_gatewayInitializer->gateway()->asyncGetPeers(
            [current](const bcos::Error::Ptr _error, bcos::gateway::GatewayInfo::Ptr _localP2pInfo,
                bcos::gateway::GatewayInfosPtr _peers) {
                auto localtarsP2pInfo = toTarsGatewayInfo(_localP2pInfo);
                std::vector<bcostars::GatewayInfo> peersInfo;
                if (_peers)
                {
                    for (auto const& peer : *_peers)
                    {
                        peersInfo.emplace_back(toTarsGatewayInfo(peer));
                    }
                }
                async_response_asyncGetPeers(
                    current, toTarsError(_error), localtarsP2pInfo, peersInfo);
            });
        return bcostars::Error();
    }

    bcostars::Error asyncSendMessageByNodeID(const std::string& groupID,
        const vector<tars::Char>& srcNodeID, const vector<tars::Char>& dstNodeID,
        const vector<tars::Char>& payload, tars::TarsCurrentPtr current) override
    {
        current->setResponse(false);
        auto keyFactory = m_gatewayInitializer->keyFactory();
        auto bcosSrcNodeID = keyFactory->createKey(
            bcos::bytesConstRef((const bcos::byte*)srcNodeID.data(), srcNodeID.size()));
        auto bcosDstNodeID = keyFactory->createKey(
            bcos::bytesConstRef((const bcos::byte*)dstNodeID.data(), dstNodeID.size()));

        m_gatewayInitializer->gateway()->asyncSendMessageByNodeID(groupID, bcosSrcNodeID,
            bcosDstNodeID, bcos::bytesConstRef((const bcos::byte*)payload.data(), payload.size()),
            [current](bcos::Error::Ptr error) {
                async_response_asyncSendMessageByNodeID(current, toTarsError(error));
            });
        return bcostars::Error();
    }

    bcostars::Error asyncSendMessageByNodeIDs(const std::string& groupID,
        const vector<tars::Char>& srcNodeID, const vector<vector<tars::Char>>& dstNodeID,
        const vector<tars::Char>& payload, tars::TarsCurrentPtr current) override
    {
        current->setResponse(false);
        auto keyFactory = m_gatewayInitializer->keyFactory();
        auto bcosSrcNodeID = keyFactory->createKey(
            bcos::bytesConstRef((const bcos::byte*)srcNodeID.data(), srcNodeID.size()));
        std::vector<bcos::crypto::NodeIDPtr> nodeIDs;
        nodeIDs.reserve(dstNodeID.size());
        for (auto const& it : dstNodeID)
        {
            nodeIDs.push_back(keyFactory->createKey(
                bcos::bytesConstRef((const bcos::byte*)it.data(), it.size())));
        }

        m_gatewayInitializer->gateway()->asyncSendMessageByNodeIDs(groupID, bcosSrcNodeID, nodeIDs,
            bcos::bytesConstRef((const bcos::byte*)payload.data(), payload.size()));

        async_response_asyncSendMessageByNodeIDs(current, toTarsError(nullptr));
        return bcostars::Error();
    }

    bcostars::Error asyncGetNodeIDs(const std::string& groupID, vector<vector<tars::Char>>& nodeIDs,
        tars::TarsCurrentPtr current) override
    {
        current->setResponse(false);

        m_gatewayInitializer->gateway()->asyncGetNodeIDs(
            groupID, [current](bcos::Error::Ptr _error,
                         std::shared_ptr<const bcos::crypto::NodeIDs> _nodeIDs) {
                // Note: the nodeIDs maybe null if no connections
                std::vector<std::vector<char>> tarsNodeIDs;
                if (!_nodeIDs)
                {
                    async_response_asyncGetNodeIDs(current, toTarsError(_error), tarsNodeIDs);
                    return;
                }
                tarsNodeIDs.reserve(_nodeIDs->size());
                for (auto const& it : *_nodeIDs)
                {
                    auto nodeIDData = it->data();
                    tarsNodeIDs.emplace_back(nodeIDData.begin(), nodeIDData.end());
                }
                async_response_asyncGetNodeIDs(current, toTarsError(_error), tarsNodeIDs);
            });

        return bcostars::Error();
    }
    bcostars::Error asyncNotifyGroupInfo(
        const bcostars::GroupInfo& groupInfo, tars::TarsCurrentPtr current) override;

    bcostars::Error asyncSendMessageByTopic(const std::string& _topic,
        const vector<tars::Char>& _data, tars::Int32& _type, vector<tars::Char>& _responseData,
        tars::TarsCurrentPtr current) override;
    bcostars::Error asyncSubscribeTopic(const std::string& _clientID, const std::string& _topicInfo,
        tars::TarsCurrentPtr current) override;
    bcostars::Error asyncSendBroadbastMessageByTopic(const std::string& _topic,
        const vector<tars::Char>& _data, tars::TarsCurrentPtr current) override;
    bcostars::Error asyncRemoveTopic(const std::string& _clientID,
        const vector<std::string>& _topicList, tars::TarsCurrentPtr current) override;

private:
    GatewayInitializer::Ptr m_gatewayInitializer;
};
}  // namespace bcostars