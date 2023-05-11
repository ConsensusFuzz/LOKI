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
 * (c) 2016-2018 fisco-dev contributors.
 */

/**
 * @brief : Sync packet decode and encode
 * @author: jimmyshi
 * @date: 2018-10-18
 */

#include "SyncMsgPacket.h"
#include <libethcore/TxsParallelParser.h>
#include <libp2p/P2PSession.h>
#include <libp2p/Service.h>

using namespace std;
using namespace dev;
using namespace dev::sync;
using namespace dev::p2p;
using namespace dev::eth;

bool SyncMsgPacket::decode(
    std::shared_ptr<dev::p2p::P2PSession> _session, dev::p2p::P2PMessage::Ptr _msg)
{
    if (_msg == nullptr)
        return false;

    bytesConstRef frame = ref(*(_msg->buffer()));
    if (!checkPacket(frame))
        return false;

    packetType = (SyncPacketType)(RLP(frame.cropped(0, 1)).toInt<unsigned>() - c_syncPacketIDBase);
    nodeId = _session->nodeID();
    m_rlp = RLP(frame.cropped(1));

    return true;
}

P2PMessage::Ptr SyncMsgPacket::toMessage(PROTOCOL_ID _protocolId)
{
    P2PMessage::Ptr msg = std::dynamic_pointer_cast<P2PMessage>(m_p2pFactory->buildMessage());

    std::shared_ptr<bytes> b = std::make_shared<bytes>();
    m_rlpStream.swapOut(*b);
    msg->setBuffer(b);
    msg->setProtocolID(_protocolId);
    return msg;
}

bool SyncMsgPacket::checkPacket(bytesConstRef _msg)
{
    if (_msg.size() < 2 || _msg[0] > 0x7f)
        return false;
    if (RLP(_msg.cropped(1)).actualSize() + 1 != _msg.size())
        return false;
    return true;
}

RLPStream& SyncMsgPacket::prep(RLPStream& _s, unsigned _id, unsigned _args)
{
    return _s.appendRaw(bytes(1, _id + c_syncPacketIDBase)).appendList(_args);
}

void SyncStatusPacket::encode()
{
    m_rlpStream.clear();
    // add aligned time into the fields
    prep(m_rlpStream, StatusPacket, m_itemCount);
    // Note: here must encode int64_t type value with append(bigint)
    // Otherwise, when the number is large the result of decode is incorrect
    m_rlpStream.append(bigint(number));
    m_rlpStream << genesisHash << latestHash;
}

void SyncStatusPacket::decodePacket(RLP const& _rlp, dev::h512 const& _peer)
{
    if (_rlp.itemCount() != m_itemCount)
    {
        LOG(WARNING) << LOG_BADGE("SYNC")
                     << LOG_DESC("SyncStatusPacket: receive invalid status packet format")
                     << LOG_KV("peer", _peer.abridged());
        BOOST_THROW_EXCEPTION(
            InValidSyncPacket() << errinfo_comment("receive invalid SyncStatusPacket format"));
    }
    nodeId = _peer;
    number = _rlp[0].toInt<int64_t>();
    genesisHash = _rlp[1].toHash<h256>();
    latestHash = _rlp[2].toHash<h256>();
}

void SyncStatusPacketWithAlignedTime::encode()
{
    SyncStatusPacket::encode();
    m_rlpStream.append(bigint(alignedTime));
}

void SyncStatusPacketWithAlignedTime::decodePacket(RLP const& _rlp, dev::h512 const& _peer)
{
    SyncStatusPacket::decodePacket(_rlp, _peer);
    // get alignedTime
    alignedTime = _rlp[3].toInt<int64_t>();
}

void SyncTransactionsPacket::encode(
    std::vector<bytes> const& _txRLPs, bool const& _enableTreeRouter, uint64_t const& _consIndex)
{
    if (g_BCOSConfig.version() >= RC2_VERSION)
    {
        unsigned fieldSize = 1;
        if (_enableTreeRouter)
        {
            fieldSize = 2;
        }
        encodeRC2(_txRLPs, fieldSize);
        // append _consIndex
        if (_enableTreeRouter)
        {
            m_rlpStream << _consIndex;
        }
        return;
    }

    m_rlpStream.clear();
    bytes txRLPS;
    unsigned txsSize = unsigned(_txRLPs.size());
    for (size_t i = 0; i < _txRLPs.size(); i++)
    {
        txRLPS += _txRLPs[i];
    }
    prep(m_rlpStream, TransactionsPacket, txsSize).appendRaw(txRLPS, txsSize);
}

void SyncTransactionsPacket::encodeRC2(
    std::vector<bytes> const& _txRLPs, unsigned const& _fieldSize)
{
    m_rlpStream.clear();
    bytes txsBytes = dev::eth::TxsParallelParser::encode(_txRLPs);
    prep(m_rlpStream, TransactionsPacket, _fieldSize).append(ref(txsBytes));
}

P2PMessage::Ptr SyncTransactionsPacket::toMessage(PROTOCOL_ID _protocolId, bool const& _fromRPC)
{
    auto msg = SyncMsgPacket::toMessage(_protocolId);
    msg->setPacketType((int)(_fromRPC));
    return msg;
}

void SyncBlocksPacket::encode(std::vector<dev::bytes> const& _blockRLPs)
{
    m_rlpStream.clear();
    unsigned size = _blockRLPs.size();
    prep(m_rlpStream, BlocksPacket, size);
    for (bytes const& bs : _blockRLPs)
        m_rlpStream.append(bs);
}

void SyncBlocksPacket::singleEncode(dev::bytes const& _blockRLP)
{
    m_rlpStream.clear();
    prep(m_rlpStream, BlocksPacket, 1);
    m_rlpStream.append(_blockRLP);
}

void SyncReqBlockPacket::encode(int64_t _from, unsigned _size)
{
    m_rlpStream.clear();
    prep(m_rlpStream, ReqBlocskPacket, 2) << _from << _size;
}

void SyncTxsStatusPacket::encode(
    int64_t const& _number, std::shared_ptr<std::set<dev::h256>> _txsHash)
{
    m_rlpStream.clear();
    auto& retRlp = prep(m_rlpStream, packetType, 2);
    retRlp << _number;
    retRlp.append(*_txsHash);
}

void SyncTxsReqPacket::encode(std::shared_ptr<std::vector<dev::h256>> _requestedTxs)
{
    m_rlpStream.clear();
    prep(m_rlpStream, packetType, 1).append(*_requestedTxs);
}