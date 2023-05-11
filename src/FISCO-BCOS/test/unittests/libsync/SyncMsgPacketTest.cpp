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
 * @brief : sync msg packet test
 * @author: catli
 * @date: 2018-10-24
 */

#include "FakeSyncToolsSet.h"
#include <libdevcrypto/Common.h>
#include <libnetwork/Common.h>
#include <libsync/SyncMsgPacket.h>
#include <libsync/SyncMsgPacketFactory.h>
#include <test/tools/libutils/TestOutputHelper.h>
#include <test/unittests/libethcore/FakeBlock.h>
#include <boost/test/unit_test.hpp>
#include <memory>

using namespace std;
using namespace dev;
using namespace dev::sync;
using namespace dev::p2p;
using namespace dev::test;

namespace dev
{
namespace test
{
class SyncMsgPacketFixture : public TestOutputHelperFixture
{
public:
    SyncMsgPacketFixture()
    {
        // m_host = createFakeHost(m_clientVersion, m_listenIp, m_listenPort);

        fakeSessionPtr = createFakeSession();
        fakeTransaction = createFakeTransaction(0);
    }

    std::shared_ptr<P2PSession> createFakeSession(std::string ip = "127.0.0.1")
    {
        NodeIPEndpoint peer_endpoint(boost::asio::ip::make_address(ip), m_listenPort);
        ;
        KeyPair key_pair = KeyPair::create();
#if 0
        std::shared_ptr<Peer> peer = std::make_shared<Peer>(key_pair.pub(), peer_endpoint);
        PeerSessionInfo peer_info({key_pair.pub(), peer_endpoint.address.to_string(),
            chrono::steady_clock::duration(), 0});
        std::shared_ptr<SessionFace> session =
            std::make_shared<FakeSessionForHost>(m_host, peer, peer_info);
#endif
        std::shared_ptr<P2PSession> session = std::make_shared<FakeSession>();
        session->start();
        return session;
    }

    Transaction::Ptr createFakeTransaction(int64_t _currentBlockNumber)
    {
        u256 value = u256(100);
        u256 gas = u256(100000000);
        u256 gasPrice = u256(0);
        Address dst = toAddress(KeyPair::create().pub());
        std::string str = "test transaction";
        bytes data(str.begin(), str.end());
        Transaction::Ptr tx = std::make_shared<Transaction>(value, gasPrice, gas, dst, data);
        tx->setNonce(tx->nonce() + u256(rand()));
        u256 c_maxBlockLimit = u256(500);
        tx->setBlockLimit(u256(_currentBlockNumber) + c_maxBlockLimit);
        KeyPair sigKeyPair = KeyPair::create();

        std::shared_ptr<crypto::Signature> sig =
            dev::crypto::Sign(sigKeyPair, tx->hash(WithoutSignature));
        /// update the signature of transaction
        tx->updateSignature(sig);
        return tx;
    }

    std::shared_ptr<P2PSession> fakeSessionPtr;
    Transaction::Ptr fakeTransaction = std::make_shared<Transaction>();

protected:
    // FakeHost* m_host;
    std::string m_clientVersion = "2.0";
    std::string m_listenIp = "127.0.0.1";
    uint16_t m_listenPort = 30304;
};

BOOST_FIXTURE_TEST_SUITE(SyncMsgPacketTest, SyncMsgPacketFixture)

BOOST_AUTO_TEST_CASE(PacketDecodeTest)
{
    auto fakeMessagePtr = shared_ptr<P2PMessage>(nullptr);
    SyncMsgPacket msgPacket;

    // message is nullptr
    bool isSuccessful = msgPacket.decode(fakeSessionPtr, fakeMessagePtr);
    BOOST_CHECK(isSuccessful == false);

    // message contains no data
    fakeMessagePtr = make_shared<P2PMessage>();
    isSuccessful = msgPacket.decode(fakeSessionPtr, fakeMessagePtr);
    BOOST_CHECK(isSuccessful == false);

    // message is not char
    auto bufferPtr = make_shared<bytes>();
    bufferPtr->push_back(0x80);
    bufferPtr->push_back(0x20);
    fakeMessagePtr->setBuffer(bufferPtr);
    isSuccessful = msgPacket.decode(fakeSessionPtr, fakeMessagePtr);
    BOOST_CHECK(isSuccessful == false);

    // message is char
    bufferPtr->at(0) = 0x7f;
    fakeMessagePtr->setBuffer(bufferPtr);
    isSuccessful = msgPacket.decode(fakeSessionPtr, fakeMessagePtr);
    BOOST_CHECK(isSuccessful == true);
}

void testSyncStatus(SyncMsgPacketFactory::Ptr _statusFactory, int64_t _blockNumber,
    h256 const& _genesisHash, h256 const& _latestHash, bool _checkTime)
{
    auto peer = dev::KeyPair::create().pub();
    auto status =
        _statusFactory->createSyncStatusPacket(peer, _blockNumber, _genesisHash, _latestHash);
    status->alignedTime = utcTime();
    status->encode();
    auto p2pMessage = status->toMessage(0x01);
    // decode Packet
    auto decodedStatus = _statusFactory->createSyncStatusPacket();
    auto fakeSession = std::make_shared<dev::p2p::P2PSession>();
    decodedStatus->decode(fakeSession, p2pMessage);
    decodedStatus->decodePacket(decodedStatus->rlp(), peer);

    BOOST_CHECK(decodedStatus->number == status->number);
    BOOST_CHECK(decodedStatus->genesisHash == status->genesisHash);
    BOOST_CHECK(decodedStatus->latestHash == status->latestHash);
    if (_checkTime)
    {
        BOOST_CHECK(decodedStatus->alignedTime == status->alignedTime);
    }
}

BOOST_AUTO_TEST_CASE(SyncStatusPacketTest)
{
    std::srand(utcTime());
    // case1: test without time
    auto syncStatusFactory = std::make_shared<SyncMsgPacketFactory>();
    testSyncStatus(syncStatusFactory, (utcTime() + std::rand() % 12000),
        dev::keccak256("genesisHash"), dev::keccak256("latestHash"), false);

    // case2: test with large blockNumber and time
    syncStatusFactory = std::make_shared<SyncMsgPacketWithAlignedTimeFactory>();
    testSyncStatus(syncStatusFactory, (utcTime() + std::rand() % 12000),
        dev::keccak256("genesisHashT"), dev::keccak256("latestHashT"), true);
}

BOOST_AUTO_TEST_CASE(SyncTransactionsPacketTest)
{
    SyncTransactionsPacket txPacket;
    vector<bytes> txRLPs;
    txRLPs.emplace_back(fakeTransaction->rlp());

    txPacket.encode(txRLPs);
    auto msgPtr = txPacket.toMessage(0x02);
    txPacket.decode(fakeSessionPtr, msgPtr);

    auto rlpTx = txPacket.rlp()[0];
    std::shared_ptr<Transactions> txs = std::make_shared<Transactions>();
    dev::eth::TxsParallelParser::decode(txs, rlpTx.toBytesConstRef());
    BOOST_CHECK((*(*txs)[0]) == *fakeTransaction);
}

BOOST_AUTO_TEST_CASE(SyncBlocksPacketTest)
{
    SyncBlocksPacket blocksPacket;
    vector<bytes> blockRLPs;
    FakeBlock fakeBlock;

    blockRLPs.push_back(fakeBlock.getBlock()->rlp());
    blocksPacket.encode(blockRLPs);
    auto msgPtr = blocksPacket.toMessage(0x03);
    blocksPacket.decode(fakeSessionPtr, msgPtr);
    RLP const& rlps = blocksPacket.rlp();
    Block block(rlps[0].toBytes());
    BOOST_CHECK(block.equalAll(*fakeBlock.getBlock()));
}

BOOST_AUTO_TEST_CASE(SyncReqBlockPacketTest)
{
    SyncReqBlockPacket reqBlockPacket;

    reqBlockPacket.encode(int64_t(0x30), 0x40);
    auto msgPtr = reqBlockPacket.toMessage(0x03);
    reqBlockPacket.decode(fakeSessionPtr, msgPtr);
    auto rlpReqBlock = reqBlockPacket.rlp();
    BOOST_CHECK(rlpReqBlock[0].toInt<int64_t>() == 0x30);
    BOOST_CHECK(rlpReqBlock[1].toInt<unsigned>() == 0x40);
}

BOOST_AUTO_TEST_SUITE_END()
}  // namespace test
}  // namespace dev
