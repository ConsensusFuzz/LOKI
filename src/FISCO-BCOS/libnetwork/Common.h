/*
    This file is part of cpp-ethereum.

    cpp-ethereum is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    cpp-ethereum is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with cpp-ethereum.  If not, see <http://www.gnu.org/licenses/>.
*/
/** @file Common.h
 * Miscellanea required for the Host/Session/NodeTable classes.
 *
 * @author yujiechen
 * @date: 2018-09-19
 * @modifications:
 * 1. remove DeadlineOps(useless class)
 * 2. remove isPrivateAddress method for logical imperfect
 * 3. remove std::hash<bi::address> class since it has not been used
 */

#pragma once

#include <set>
#include <string>
#include <vector>

// Make sure boost/asio.hpp is included before windows.h.
#include <boost/asio.hpp>
#include <boost/asio/ip/tcp.hpp>
#include <boost/asio/ssl.hpp>
#include <boost/logic/tribool.hpp>

#include <libdevcore/Exceptions.h>
#include <libdevcore/FixedHash.h>
#include <libdevcore/Guards.h>
#include <libdevcore/RLP.h>
#include <libdevcrypto/Common.h>
#include <libethcore/Protocol.h>
#include <chrono>
#include <sstream>

namespace ba = boost::asio;
namespace bi = boost::asio::ip;
#define HOST_LOG(LEVEL) LOG(LEVEL) << "[NETWORK][Host]"
#define SESSION_LOG(LEVEL) LOG(LEVEL) << "[NETWORK][Session]"
#define ASIO_LOG(LEVEL) LOG(LEVEL) << "[NETWORK][ASIO]"

namespace dev
{
namespace network
{
/// define Exceptions
DEV_SIMPLE_EXCEPTION(NetworkStartRequired);
DEV_SIMPLE_EXCEPTION(InvalidPublicIPAddress);
DEV_SIMPLE_EXCEPTION(InvalidHostIPAddress);

enum DisconnectReason
{
    DisconnectRequested = 0,
    TCPError,
    BadProtocol,
    UselessPeer,
    TooManyPeers,
    DuplicatePeer,
    IncompatibleProtocol,
    NullIdentity,
    ClientQuit,
    UnexpectedIdentity,
    LocalIdentity,
    PingTimeout,
    UserReason = 0x10,
    IdleWaitTimeout = 0x11,
    NoDisconnect = 0xffff
};

///< P2PExceptionType and g_P2PExceptionMsg used in P2PException
enum P2PExceptionType
{
    Success = 0,
    ProtocolError,
    NetworkTimeout,
    Disconnect,
    P2PExceptionTypeCnt,
    ConnectError,
    DuplicateSession,
    NotInWhitelist,
    ALL,
};

enum PacketDecodeStatus
{
    PACKET_ERROR = -1,
    PACKET_INCOMPLETE = 0
};

using NodeID = dev::h512;

struct Options
{
    uint32_t subTimeout = 0;  ///< The timeout value of every node, used in send message to topic,
                              ///< in milliseconds.
    uint32_t timeout = 0;     ///< The timeout value of async function, in milliseconds.
};

/// node info obtained from the certificate
struct NodeInfo
{
    NodeID nodeID;
    std::string agencyName;
    std::string nodeName;
};

class Message : public std::enable_shared_from_this<Message>
{
public:
    virtual ~Message(){};

    typedef std::shared_ptr<Message> Ptr;

    virtual uint32_t length() = 0;
    virtual uint32_t seq() = 0;

    virtual bool isRequestPacket() = 0;

    virtual void encode(bytes& buffer) = 0;
    virtual ssize_t decode(const byte* buffer, size_t size) = 0;
};

class MessageFactory : public std::enable_shared_from_this<MessageFactory>
{
public:
    typedef std::shared_ptr<MessageFactory> Ptr;

    virtual ~MessageFactory(){};
    virtual Message::Ptr buildMessage() = 0;
};

class NetworkException : public std::exception
{
public:
    NetworkException(){};
    NetworkException(int _errorCode, const std::string& _msg)
      : m_errorCode(_errorCode), m_msg(_msg){};

    virtual int errorCode() { return m_errorCode; };
    virtual const char* what() const noexcept override { return m_msg.c_str(); };
    bool operator!() const { return m_errorCode == 0; }

private:
    int m_errorCode = 0;
    std::string m_msg = "";
};

/// @returns the string form of the given disconnection reason.
inline std::string reasonOf(DisconnectReason _r)
{
    switch (_r)
    {
    case DisconnectRequested:
        return "Disconnect was requested.";
    case TCPError:
        return "Low-level TCP communication error.";
    case BadProtocol:
        return "Data format error.";
    case UselessPeer:
        return "Peer had no use for this node.";
    case TooManyPeers:
        return "Peer had too many connections.";
    case DuplicatePeer:
        return "Peer was already connected.";
    case IncompatibleProtocol:
        return "Peer protocol versions are incompatible.";
    case NullIdentity:
        return "Null identity given.";
    case ClientQuit:
        return "Peer is exiting.";
    case UnexpectedIdentity:
        return "Unexpected identity given.";
    case LocalIdentity:
        return "Connected to ourselves.";
    case UserReason:
        return "Subprotocol reason.";
    case NoDisconnect:
        return "(No disconnect has happened.)";
    case IdleWaitTimeout:
        return "(Idle connection for no network io happens during 5s time intervals.)";
    default:
        return "Unknown reason.";
    }
}

// using NodeIPEndpoint = boost::asio::ip::tcp::endpoint;
/**
 * @brief client end endpoint. Node will connect to NodeIPEndpoint.
 */
struct NodeIPEndpoint
{
    NodeIPEndpoint() = default;
    NodeIPEndpoint(const NodeIPEndpoint& _nodeIPEndpoint) = default;
    NodeIPEndpoint(const std::string& _host, uint16_t _port) : m_host(_host), m_port(_port) {}
    NodeIPEndpoint(bi::address _addr, uint16_t _port)
      : m_host(_addr.to_string()), m_port(_port), m_ipv6(_addr.is_v6())
    {}

    virtual ~NodeIPEndpoint() = default;
    NodeIPEndpoint(const boost::asio::ip::tcp::endpoint& _endpoint)
    {
        m_host = _endpoint.address().to_string();
        m_port = _endpoint.port();
        m_ipv6 = _endpoint.address().is_v6();
    }
    bool operator<(const NodeIPEndpoint& rhs) const
    {
        if (m_host + std::to_string(m_port) < rhs.m_host + std::to_string(rhs.m_port))
        {
            return true;
        }
        return false;
    }
    bool operator==(const NodeIPEndpoint& rhs) const
    {
        return (m_host + std::to_string(m_port) == rhs.m_host + std::to_string(rhs.m_port));
    }
    operator boost::asio::ip::tcp::endpoint() const
    {
        return boost::asio::ip::tcp::endpoint(boost::asio::ip::make_address(m_host), m_port);
    }

    // Get the port associated with the endpoint.
    uint16_t port() const { return m_port; };

    // Get the IP address associated with the endpoint.
    std::string address() const { return m_host; };
    bool isIPv6() const { return m_ipv6; }

    std::string m_host;
    uint16_t m_port;
    bool m_ipv6 = false;
};

std::ostream& operator<<(std::ostream& _out, NodeIPEndpoint const& _endpoint);

bool getPublicKeyFromCert(std::shared_ptr<std::string> _nodeIDOut, X509* cert);
}  // namespace network
}  // namespace dev
