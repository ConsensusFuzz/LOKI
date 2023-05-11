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
 * (c) 2016-2019 fisco-dev contributors.
 */

/**
 * @brief : parse ledger configuration
 * @file: LedgerParam.cpp
 * @author: xingqiangbai
 * @date: 2019-10-16
 */
#include "LedgerParam.h"
#include "libconsensus/Common.h"
#include <libblockchain/BlockChainInterface.h>
#include <libconfig/GlobalConfigure.h>
#include <libeventfilter/EventLogFilterManager.h>
#include <boost/algorithm/string.hpp>
#include <boost/bind/bind.hpp>
#include <boost/function_output_iterator.hpp>
#include <boost/property_tree/ini_parser.hpp>

using namespace boost::property_tree;
using namespace std;
using namespace dev;

namespace dev
{
namespace ledger
{
void LedgerParam::parseGenesisConfig(const std::string& _genesisFile)
{
    try
    {
        m_genesisConfigPath = _genesisFile;
        ptree pt;
        // read the configuration file for a specified group
        read_ini(_genesisFile, pt);
        m_groupID = pt.get<int>("group.id", 0);
        LedgerParam_LOG(INFO) << LOG_BADGE("parseGenesisConfig")
                              << LOG_DESC("initConsensusConfig/initStorageConfig/initTxConfig")
                              << LOG_KV("configFile", _genesisFile);
        initConsensusConfig(pt);
        initEventLogFilterManagerConfig(pt);
        /// use UTCTime directly as timeStamp in case of the clock differences between machines
        auto timeStamp = pt.get<int64_t>("group.timestamp", INT64_MAX);
        if (timeStamp < 0)
        {
            LedgerParam_LOG(ERROR)
                << LOG_BADGE("parseGenesisConfig") << LOG_DESC("invalid group timeStamp")
                << LOG_KV("timeStamp", timeStamp);
            BOOST_THROW_EXCEPTION(Exception("invalid group timestamp"));
        }
        mutableGenesisParam().timeStamp = timeStamp;
        LedgerParam_LOG(INFO) << LOG_BADGE("parseGenesisConfig")
                              << LOG_KV("timestamp", mutableGenesisParam().timeStamp);
        mutableStateParam().type = pt.get<std::string>("state.type", "storage");
        // Compatibility with previous versions RC2/RC1
        mutableStorageParam().type = pt.get<std::string>("storage.type", "LevelDB");
        mutableStorageParam().topic = pt.get<std::string>("storage.topic", "DB");
        mutableStorageParam().maxRetry = pt.get<uint>("storage.max_retry", 60);
        if (g_BCOSConfig.version() >= V2_4_0)
        {
            setEVMFlags(pt);
        }
    }
    catch (std::exception& e)
    {
        std::string error_info = "init genesis config failed for " + toString(m_groupID) +
                                 " failed, error_msg: " + boost::diagnostic_information(e);
        LedgerParam_LOG(ERROR) << LOG_DESC("parseGenesisConfig Failed")
                               << LOG_KV("EINFO", boost::diagnostic_information(e));
        BOOST_THROW_EXCEPTION(dev::InitLedgerConfigFailed() << errinfo_comment(error_info));
    }
    generateGenesisMark();
}

void LedgerParam::setEVMFlags(boost::property_tree::ptree const& _pt)
{
    bool enableFreeStorageVMSchedule = _pt.get<bool>("evm.enable_free_storage", false);
    if (enableFreeStorageVMSchedule)
    {
        mutableGenesisParam().evmFlags |= EVMFlags::FreeStorageGas;
    }
    LedgerParam_LOG(INFO) << LOG_DESC("setEVMFlags")
                          << LOG_KV("enableFreeStorageVMSchedule", enableFreeStorageVMSchedule)
                          << LOG_KV("evmFlags", mutableGenesisParam().evmFlags);
}

void LedgerParam::generateGenesisMark()
{
    std::stringstream s;
    s << int(m_groupID) << "-";
    s << mutableGenesisParam().nodeListMark << "-";
    s << mutableConsensusParam().consensusType << "-";
    if (g_BCOSConfig.version() <= RC2_VERSION)
    {
        s << mutableStorageParam().type << "-";
    }
    s << mutableStateParam().type << "-";
    if (g_BCOSConfig.version() >= V2_4_0)
    {
        LedgerParam_LOG(INFO) << LOG_DESC("store evmFlag")
                              << LOG_KV("evmFlag", mutableGenesisParam().evmFlags);
        s << mutableGenesisParam().evmFlags << "-";
    }

    s << mutableConsensusParam().maxTransactions << "-";
    s << mutableTxParam().txGasLimit;

    // init epochSealerNum and epochBlockNum for rPBFT
    if (dev::stringCmpIgnoreCase(mutableConsensusParam().consensusType, RPBFT_CONSENSUS_TYPE) == 0)
    {
        LedgerParam_LOG(INFO) << LOG_DESC("store rPBFT related configuration")
                              << LOG_KV("epochSealerNum", mutableConsensusParam().epochSealerNum)
                              << LOG_KV("epochBlockNum", mutableConsensusParam().epochBlockNum);
        s << "-" << mutableConsensusParam().epochSealerNum << "-";
        s << mutableConsensusParam().epochBlockNum;
    }
    // only the supported_version is greater than or equal to v2.6.0,
    // the consensus time runtime setting is enabled
    if (g_BCOSConfig.version() >= V2_6_0)
    {
        LedgerParam_LOG(INFO) << LOG_DESC("store consensus time")
                              << LOG_KV(
                                     "consensusTimeout", mutableConsensusParam().consensusTimeout);
        s << "-" << mutableConsensusParam().consensusTimeout;
    }
    m_genesisMark = s.str();
    LedgerParam_LOG(INFO) << LOG_BADGE("initMark") << LOG_KV("genesisMark", m_genesisMark);
}

void LedgerParam::parseIniConfig(const std::string& _iniConfigFile, const std::string& _dataPath)
{
    m_iniConfigPath = _iniConfigFile;
    std::string prefix = _dataPath + "/group" + std::to_string(m_groupID);
    if (_dataPath == "")
    {
        prefix = "./group" + std::to_string(m_groupID);
    }
    m_baseDir = prefix;
    LedgerParam_LOG(INFO) << LOG_BADGE("initIniConfig")
                          << LOG_DESC("initTxPoolConfig/initSyncConfig/initTxExecuteConfig")
                          << LOG_KV("configFile", _iniConfigFile);
    ptree pt;
    // all the configurations related to ini have default value
    // so here is no need to throw exception when not find _iniConfigFile
    if (boost::filesystem::exists(_iniConfigFile))
    {
        read_ini(_iniConfigFile, pt);
    }
    initStorageConfig(pt);
    initTxPoolConfig(pt);
    initSyncConfig(pt);
    initTxExecuteConfig(pt);
    // init params releated to consensus(ttl)
    initConsensusIniConfig(pt);
    initFlowControlConfig(pt);
    parseSDKAllowList(m_permissionParam.sdkAllowList, pt);
}

void LedgerParam::init(const std::string& _configFilePath, const std::string& _dataPath)
{
    /// The file group.X.genesis is required, otherwise the program terminates.
    /// load genesis config of group

    parseGenesisConfig(_configFilePath);
    // The file group.X.ini is available by default.
    std::string iniConfigFileName = _configFilePath;
    boost::replace_last(iniConfigFileName, "genesis", "ini");

    parseIniConfig(iniConfigFileName, _dataPath);
}

void LedgerParam::initTxExecuteConfig(ptree const& pt)
{
    if (dev::stringCmpIgnoreCase(mutableStateParam().type, "storage") == 0)
    {
        // enable parallel since v2.3.0 when stateType is storage
        if (g_BCOSConfig.version() >= V2_3_0)
        {
            mutableTxParam().enableParallel = true;
        }
        // can configure enable_parallel before v2.3.0
        else
        {
            mutableTxParam().enableParallel = pt.get<bool>("tx_execute.enable_parallel", true);
        }
    }
    else
    {
        mutableTxParam().enableParallel = false;
    }
    LedgerParam_LOG(INFO) << LOG_BADGE("InitTxExecuteConfig")
                          << LOG_KV("enableParallel", mutableTxParam().enableParallel);
}

void LedgerParam::initTxPoolConfig(ptree const& pt)
{
    try
    {
        mutableTxPoolParam().txPoolLimit =
            pt.get<int64_t>("tx_pool.limit", SYNC_TX_POOL_SIZE_DEFAULT);
        if (mutableTxPoolParam().txPoolLimit < 0)
        {
            BOOST_THROW_EXCEPTION(
                ForbidNegativeValue() << errinfo_comment("Please set tx_pool.limit to positive !"));
        }

        auto memorySizeLimit = pt.get<int64_t>("tx_pool.memory_limit", TX_POOL_DEFAULT_MEMORY_SIZE);
        if (memorySizeLimit < 0 || memorySizeLimit >= MAX_VALUE_IN_MB)
        {
            BOOST_THROW_EXCEPTION(
                ForbidNegativeValue() << errinfo_comment(
                    "Please set tx_pool.limit to be larger than 0 and smaller than " +
                    std::to_string(MAX_VALUE_IN_MB)));
        }
        mutableTxPoolParam().maxTxPoolMemorySize = memorySizeLimit * 1024 * 1024;

        auto notifyWorkerNum = pt.get<int64_t>("tx_pool.notify_worker_num", 2);
        if (notifyWorkerNum <= 0)
        {
            BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                      "Please set tx_pool.notify_worker_num to positive !"));
        }
        mutableTxPoolParam().notifyWorkerNum = notifyWorkerNum;
        LedgerParam_LOG(INFO) << LOG_BADGE("initTxPoolConfig")
                              << LOG_KV("txPoolLimit", mutableTxPoolParam().txPoolLimit)
                              << LOG_KV("memorySizeLimit(MB)", memorySizeLimit)
                              << LOG_KV("notifyWorkerNum", notifyWorkerNum);
    }
    catch (std::exception& e)
    {
        mutableTxPoolParam().txPoolLimit = SYNC_TX_POOL_SIZE_DEFAULT;
        LedgerParam_LOG(WARNING) << LOG_BADGE("txPoolLimit") << LOG_DESC("txPoolLimit invalid");
    }
}

void LedgerParam::initRPBFTConsensusIniConfig(boost::property_tree::ptree const& pt)
{
    mutableConsensusParam().broadcastPrepareByTree =
        pt.get<bool>("consensus.broadcast_prepare_by_tree", true);

    mutableConsensusParam().prepareStatusBroadcastPercent =
        pt.get<signed>("consensus.prepare_status_broadcast_percent", 33);
    if (mutableConsensusParam().prepareStatusBroadcastPercent < 25 ||
        mutableConsensusParam().prepareStatusBroadcastPercent > 100)
    {
        BOOST_THROW_EXCEPTION(
            InvalidConfiguration() << errinfo_comment(
                "consensus.prepare_status_broadcast_percent must be between 25 and 100"));
    }
    // maxRequestMissedTxsWaitTime
    mutableConsensusParam().maxRequestMissedTxsWaitTime =
        pt.get<int64_t>("consensus.max_request_missedTxs_waitTime", 100);
    if (mutableConsensusParam().maxRequestMissedTxsWaitTime < 5 ||
        mutableConsensusParam().maxRequestMissedTxsWaitTime > 1000)
    {
        BOOST_THROW_EXCEPTION(
            InvalidConfiguration()
            << errinfo_comment("consensus.max_request_missedTxs_waitTime must between 5 and 1000"));
    }
    // maxRequestPrepareWaitTime;
    mutableConsensusParam().maxRequestPrepareWaitTime =
        pt.get<int64_t>("consensus.max_request_prepare_waitTime", 100);
    if (mutableConsensusParam().maxRequestPrepareWaitTime < 10 ||
        mutableConsensusParam().maxRequestPrepareWaitTime > 1000)
    {
        BOOST_THROW_EXCEPTION(
            InvalidConfiguration()
            << errinfo_comment("consensus.max_request_prepare_waitTime must between 10 and 1000"));
    }
    LedgerParam_LOG(INFO) << LOG_BADGE("initRPBFTConsensusIniConfig")
                          << LOG_KV("broadcastPrepareByTree",
                                 mutableConsensusParam().broadcastPrepareByTree)
                          << LOG_KV("prepareStatusBroadcastPercent",
                                 mutableConsensusParam().prepareStatusBroadcastPercent)
                          << LOG_KV("maxRequestMissedTxsWaitTime",
                                 mutableConsensusParam().maxRequestMissedTxsWaitTime)
                          << LOG_KV("maxRequestPrepareWaitTime",
                                 mutableConsensusParam().maxRequestPrepareWaitTime);
}

void LedgerParam::initConsensusIniConfig(ptree const& pt)
{
    mutableConsensusParam().maxTTL = pt.get<int8_t>("consensus.ttl", consensus::MAXTTL);
    if (mutableConsensusParam().maxTTL < 0 ||
        mutableConsensusParam().maxTTL > mutableConsensusParam().ttlLimit)
    {
        BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                  "Please set consensus.ttl between 0 and " +
                                  std::to_string(mutableConsensusParam().ttlLimit) + " !"));
    }

    // the minimum block generation time(ms)
    int64_t minBlockGenTime = pt.get<signed>("consensus.min_block_generation_time", 500);
    if (minBlockGenTime < 0 || minBlockGenTime >= UINT_MAX)
    {
        BOOST_THROW_EXCEPTION(InvalidConfiguration() << errinfo_comment(
                                  "Please set consensus.min_block_generation_time between 1 and " +
                                  std::to_string(UINT_MAX) + " !"));
    }
    mutableConsensusParam().minBlockGenTime = minBlockGenTime;

    // enable dynamic block size
    mutableConsensusParam().enableDynamicBlockSize =
        pt.get<bool>("consensus.enable_dynamic_block_size", true);
    // obtain block size increase ratio
    mutableConsensusParam().blockSizeIncreaseRatio =
        pt.get<float>("consensus.block_size_increase_ratio", 0.5);

    if (mutableConsensusParam().blockSizeIncreaseRatio < 0 ||
        mutableConsensusParam().blockSizeIncreaseRatio > 2)
    {
        mutableConsensusParam().blockSizeIncreaseRatio = 0.5;
    }
    // set enableTTLOptimize
    if (g_BCOSConfig.version() >= V2_2_0)
    {
        mutableConsensusParam().enableTTLOptimize =
            pt.get<bool>("consensus.enable_ttl_optimization", true);
    }
    else
    {
        mutableConsensusParam().enableTTLOptimize =
            pt.get<bool>("consensus.enable_ttl_optimization", false);
    }

    // set enableTxsWithTxsHash
    if (g_BCOSConfig.version() >= V2_2_0)
    {
        mutableConsensusParam().enablePrepareWithTxsHash =
            pt.get<bool>("consensus.enable_prepare_with_txsHash", true);
    }
    else
    {
        mutableConsensusParam().enablePrepareWithTxsHash =
            pt.get<bool>("consensus.enable_prepare_with_txsHash", false);
    }
    // only support >= 2.2.0-rc2
    if (g_BCOSConfig.version() < RC2_VERSION)
    {
        mutableConsensusParam().enablePrepareWithTxsHash = false;
    }

    LedgerParam_LOG(INFO)
        << LOG_BADGE("initConsensusIniConfig")
        << LOG_KV("maxTTL", std::to_string(mutableConsensusParam().maxTTL))
        << LOG_KV("minBlockGenerationTime", mutableConsensusParam().minBlockGenTime)
        << LOG_KV("enablDynamicBlockSize", mutableConsensusParam().enableDynamicBlockSize)
        << LOG_KV("blockSizeIncreaseRatio", mutableConsensusParam().blockSizeIncreaseRatio)
        << LOG_KV("enableTTLOptimize", mutableConsensusParam().enableTTLOptimize)
        << LOG_KV("enablePrepareWithTxsHash", mutableConsensusParam().enablePrepareWithTxsHash);
    // init rpbft related configurations
    initRPBFTConsensusIniConfig(pt);
}


/// init consensus configurations:
/// 1. consensusType: current support pbft only (default is pbft)
/// 2. maxTransNum: max number of transactions can be sealed into a block
/// 3. intervalBlockTime: average block generation period
/// 4. sealer.${idx}: define the node id of every sealer related to the group
void LedgerParam::initConsensusConfig(ptree const& pt)
{
    mutableConsensusParam().consensusType =
        pt.get<std::string>("consensus.consensus_type", PBFT_CONSENSUS_TYPE);

    mutableConsensusParam().maxTransactions = pt.get<int64_t>("consensus.max_trans_num", 1000);
    if (mutableConsensusParam().maxTransactions <= 0)
    {
        BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                  "Please set consensus.max_trans_num to positive !"));
    }

    // init consensusTimeout
    auto consensusTimeout = pt.get<int64_t>("consensus.consensus_timeout", 3);

    if (mutableConsensusParam().consensusTimeout < dev::precompiled::SYSTEM_CONSENSUS_TIMEOUT_MIN ||
        mutableConsensusParam().consensusTimeout >= dev::precompiled::SYSTEM_CONSENSUS_TIMEOUT_MAX)
    {
        BOOST_THROW_EXCEPTION(
            InvalidConfiguration() << errinfo_comment(
                "Please set consensus.consensus_time must between " +
                std::to_string(dev::precompiled::SYSTEM_CONSENSUS_TIMEOUT_MIN) + "s and " +
                std::to_string(dev::precompiled::SYSTEM_CONSENSUS_TIMEOUT_MAX) + "s !"));
    }
    mutableConsensusParam().consensusTimeout = consensusTimeout;

    mutableConsensusParam().minElectTime = pt.get<int64_t>("consensus.min_elect_time", 1000);
    if (mutableConsensusParam().minElectTime <= 0)
    {
        BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                  "Please set consensus.min_elect_time to positive !"));
    }

    mutableConsensusParam().maxElectTime = pt.get<int64_t>("consensus.max_elect_time", 2000);
    if (mutableConsensusParam().maxElectTime <= 0)
    {
        BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                  "Please set consensus.max_elect_time to positive !"));
    }
    mutableTxParam().txGasLimit = pt.get<int64_t>("tx.gas_limit", 300000000);
    if (mutableTxParam().txGasLimit < 0)
    {
        BOOST_THROW_EXCEPTION(
            ForbidNegativeValue() << errinfo_comment("Please set tx.gas_limit to positive !"));
    }

    LedgerParam_LOG(INFO) << LOG_BADGE("initConsensusConfig")
                          << LOG_KV("type", mutableConsensusParam().consensusType)
                          << LOG_KV("consensusTimeout", mutableConsensusParam().consensusTimeout)
                          << LOG_KV("maxTxNum", mutableConsensusParam().maxTransactions)
                          << LOG_KV("txGasLimit", mutableTxParam().txGasLimit);

    // if the consensus node id is invalid, throw InvalidConfiguration exception
    parsePublicKeyListOfSection(mutableConsensusParam().sealerList, pt, "consensus", "node.");
    std::stringstream nodeListMark;
    // init nodeListMark
    for (auto const& node : mutableConsensusParam().sealerList)
    {
        nodeListMark << toHex(node) << ",";
    }
    mutableGenesisParam().nodeListMark = nodeListMark.str();

    // init configurations for rPBFT
    mutableConsensusParam().epochSealerNum =
        pt.get<int64_t>("consensus.epoch_sealer_num", mutableConsensusParam().sealerList.size());
    if (mutableConsensusParam().epochSealerNum <= 0)
    {
        BOOST_THROW_EXCEPTION(InvalidConfiguration() << errinfo_comment(
                                  "Please set consensus.epoch_sealer_num to be larger than 0!"));
    }

    mutableConsensusParam().epochBlockNum = pt.get<int64_t>("consensus.epoch_block_num", 1000);
    if (g_BCOSConfig.version() < V2_6_0)
    {
        if (mutableConsensusParam().epochBlockNum <= 0)
        {
            BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                      "Please set consensus.epoch_block_num to positive !"));
        }
    }
    else
    {
        // epoch_block_num is at least 2 when supported_version >= v2.6.0
        if (mutableConsensusParam().epochBlockNum <= dev::precompiled::RPBFT_EPOCH_BLOCK_NUM_MIN)
        {
            BOOST_THROW_EXCEPTION(
                InvalidConfiguration() << errinfo_comment(
                    "Please set consensus.epoch_block_num to be larger than " +
                    std::to_string(dev::precompiled::RPBFT_EPOCH_BLOCK_NUM_MIN) + "!"));
        }
    }
    LedgerParam_LOG(DEBUG) << LOG_BADGE("initConsensusConfig")
                           << LOG_KV("epochSealerNum", mutableConsensusParam().epochSealerNum)
                           << LOG_KV("epochBlockNum", mutableConsensusParam().epochBlockNum);
}

void LedgerParam::initSyncConfig(ptree const& pt)
{
    // idleWaitMs: default is 30ms
    mutableSyncParam().idleWaitMs = pt.get<uint>("sync.idle_wait_ms", SYNC_IDLE_WAIT_DEFAULT);
    if (mutableSyncParam().idleWaitMs < 0)
    {
        BOOST_THROW_EXCEPTION(
            ForbidNegativeValue() << errinfo_comment("Please set sync.idle_wait_ms to positive !"));
    }

    LedgerParam_LOG(INFO) << LOG_BADGE("initSyncConfig")
                          << LOG_KV("idleWaitMs", mutableSyncParam().idleWaitMs);

    // send_txs_by_tree only supported after RC2
    if (g_BCOSConfig.version() < RC2_VERSION)
    {
        mutableSyncParam().enableSendTxsByTree = false;
    }
    // the support_version is lower than 2.2.0, default disable send_txs_by_tree
    else if (g_BCOSConfig.version() <= V2_1_0)
    {
        mutableSyncParam().enableSendTxsByTree = pt.get<bool>("sync.send_txs_by_tree", false);
        mutableSyncParam().enableSendBlockStatusByTree =
            pt.get<bool>("sync.sync_block_by_tree", false);
    }
    // the supported_version >= v2.2.0, default enable send_txs_by_tree
    else
    {
        mutableSyncParam().enableSendTxsByTree = pt.get<bool>("sync.send_txs_by_tree", true);
        mutableSyncParam().enableSendBlockStatusByTree =
            pt.get<bool>("sync.sync_block_by_tree", true);
    }
    LedgerParam_LOG(INFO) << LOG_BADGE("initSyncConfig")
                          << LOG_KV("enableSendTxsByTree", mutableSyncParam().enableSendTxsByTree)
                          << LOG_KV("enableSendBlockStatusByTree",
                                 mutableSyncParam().enableSendBlockStatusByTree);

    // set gossipInterval for syncMaster, default is 1s
    mutableSyncParam().gossipInterval = pt.get<int64_t>("sync.gossip_interval_ms", 1000);
    if (mutableSyncParam().gossipInterval < 1000 || mutableSyncParam().gossipInterval > 3000)
    {
        BOOST_THROW_EXCEPTION(InvalidConfiguration() << errinfo_comment(
                                  "Please set sync.gossip_interval_ms to between 1000ms-3000ms!"));
    }
    LedgerParam_LOG(INFO) << LOG_BADGE("initSyncConfig")
                          << LOG_KV("gossipInterval", mutableSyncParam().gossipInterval);

    // set the number of gossip peers for syncMaster, default is 3
    mutableSyncParam().gossipPeers = pt.get<int64_t>("sync.gossip_peers_number", 3);
    if (mutableSyncParam().gossipPeers <= 0)
    {
        BOOST_THROW_EXCEPTION(ForbidNegativeValue() << errinfo_comment(
                                  "Please set sync.gossip_peers_number to positive !"));
    }
    // set the sync-tree-width, default is 3
    mutableSyncParam().syncTreeWidth = 3;
    // max_block_sync_queue_size, default is 512MB
    mutableSyncParam().maxQueueSizeForBlockSync =
        pt.get<int>("sync.max_block_sync_memory_size", 512);

    if (mutableSyncParam().maxQueueSizeForBlockSync < 32 ||
        mutableSyncParam().maxQueueSizeForBlockSync >= MAX_VALUE_IN_MB)
    {
        BOOST_THROW_EXCEPTION(
            InvalidConfiguration() << errinfo_comment("Please set max_block_sync_memory_size must "
                                                      "be no smaller than 32MB and smaller than " +
                                                      std::to_string(MAX_VALUE_IN_MB)));
    }
    mutableSyncParam().txsStatusGossipMaxPeers = pt.get<signed>("sync.txs_max_gossip_peers_num", 5);
    if (mutableSyncParam().txsStatusGossipMaxPeers < 0)
    {
        BOOST_THROW_EXCEPTION(InvalidConfiguration() << errinfo_comment(
                                  "txs_gossip_max_peers_num must be no smaller than zero"));
    }
    mutableSyncParam().maxQueueSizeForBlockSync *= 1024 * 1024;

    LedgerParam_LOG(INFO)
        << LOG_BADGE("initSyncConfig")
        << LOG_KV("enableSendBlockStatusByTree", mutableSyncParam().enableSendBlockStatusByTree)
        << LOG_KV("gossipInterval", mutableSyncParam().gossipInterval)
        << LOG_KV("gossipPeers", mutableSyncParam().gossipPeers)
        << LOG_KV("syncTreeWidth", mutableSyncParam().syncTreeWidth)
        << LOG_KV("maxQueueSizeForBlockSync", mutableSyncParam().maxQueueSizeForBlockSync)
        << LOG_KV("txsStatusGossipMaxPeers", mutableSyncParam().txsStatusGossipMaxPeers);
}

std::string LedgerParam::uriEncode(const std::string& keyWord)
{
    std::vector<std::string> keyWordList;
    string keyWordEncode;
    std::transform(keyWord.begin(), keyWord.end(), std::back_inserter(keyWordList),
        [](std::string::value_type v) -> std::string {
            if (isalnum(v))
            {
                return std::string() + v;
            }
            std::ostringstream enc;
            enc << '%' << std::setw(2) << std::setfill('0') << std::hex << std::uppercase
                << int(static_cast<unsigned char>(v));
            return enc.str();
        });
    for (const auto& item : keyWordList)
    {
        keyWordEncode.append(item);
    }
    return keyWordEncode;
}


void LedgerParam::initStorageConfig(ptree const& pt)
{
    if (g_BCOSConfig.version() > RC2_VERSION)
    {
        mutableStorageParam().type = pt.get<std::string>("storage.type", "RocksDB");
        mutableStorageParam().topic = pt.get<std::string>("storage.topic", "DB");
        mutableStorageParam().maxRetry = pt.get<uint>("storage.max_retry", 60);
        mutableStorageParam().binaryLog = pt.get<bool>("storage.binary_log", false);
        mutableStorageParam().CachedStorage = pt.get<bool>("storage.cached_storage", true);
        if (!dev::stringCmpIgnoreCase(mutableStorageParam().type, "LevelDB"))
        {
            mutableStorageParam().type = "RocksDB";
            LedgerParam_LOG(WARNING) << "LevelDB is deprecated! RocksDB is recommended.";
        }
    }
    mutableStorageParam().path = baseDir() + "/block";
    if (!dev::stringCmpIgnoreCase(mutableStorageParam().type, "RocksDB"))
    {
        mutableStorageParam().path += "/RocksDB";
    }
    else if (!dev::stringCmpIgnoreCase(mutableStorageParam().type, "Scalable"))
    {
        mutableStorageParam().path += "/Scalable";
    }
    mutableStorageParam().maxCapacity = pt.get<int>("storage.max_capacity", 32);

    if (mutableStorageParam().maxCapacity <= 0 &&
        mutableStorageParam().maxCapacity >= MAX_VALUE_IN_MB)
    {
        BOOST_THROW_EXCEPTION(InvalidConfiguration() << errinfo_comment(
                                  "storage.max_capacity must be larger than 0 and smaller than " +
                                  std::to_string(MAX_VALUE_IN_MB)));
    }

    auto scrollThresholdMultiple = pt.get<uint>("storage.scroll_threshold_multiple", 2);
    mutableStorageParam().scrollThreshold =
        scrollThresholdMultiple > 0 ? scrollThresholdMultiple * g_BCOSConfig.c_blockLimit : 2000;

    mutableStorageParam().maxForwardBlock = pt.get<uint>("storage.max_forward_block", 10);

    if (mutableStorageParam().maxRetry <= 1)
    {
        LedgerParam_LOG(WARNING) << LOG_BADGE("initStorageConfig maxRetry should max than 1");
        mutableStorageParam().maxRetry = 60;
    }

    mutableStorageParam().dbType = pt.get<std::string>("storage.db_type", "mysql");
    mutableStorageParam().dbIP = pt.get<std::string>("storage.db_ip", "127.0.0.1");
    mutableStorageParam().dbPort = pt.get<int>("storage.db_port", 3306);
    mutableStorageParam().dbUsername = pt.get<std::string>("storage.db_username", "");
    mutableStorageParam().dbPasswd = uriEncode(pt.get<std::string>("storage.db_passwd", ""));
    mutableStorageParam().dbName = pt.get<std::string>("storage.db_name", "");
    mutableStorageParam().dbCharset = pt.get<std::string>("storage.db_charset", "utf8mb4");
    mutableStorageParam().initConnections = pt.get<int>("storage.init_connections", 15);
    mutableStorageParam().maxConnections = pt.get<int>("storage.max_connections", 50);

    LedgerParam_LOG(INFO) << LOG_BADGE("initStorageConfig")
                          << LOG_KV("stateType", mutableStateParam().type)
                          << LOG_KV("storageDB", mutableStorageParam().type)
                          << LOG_KV("storagePath", mutableStorageParam().path)
                          << LOG_KV("baseDir", baseDir())
                          << LOG_KV("dbtype", mutableStorageParam().dbType)
                          << LOG_KV("dbip", mutableStorageParam().dbIP)
                          << LOG_KV("dbport", mutableStorageParam().dbPort)
                          << LOG_KV("dbcharset", mutableStorageParam().dbCharset)
                          << LOG_KV("initconnections", mutableStorageParam().initConnections)
                          << LOG_KV("maxconnections", mutableStorageParam().maxConnections)
                          << LOG_KV("scrollThreshold", mutableStorageParam().scrollThreshold);
}

void LedgerParam::initEventLogFilterManagerConfig(boost::property_tree::ptree const& pt)
{
    mutableEventLogFilterManagerParams().maxBlockRange =
        pt.get<int64_t>("event_filter.max_block_range", MAX_BLOCK_RANGE_EVENT_FILTER);

    mutableEventLogFilterManagerParams().maxBlockPerProcess =
        pt.get<int64_t>("event_filter.max_block_per_process", MAX_BLOCK_PER_PROCESS);

    LedgerParam_LOG(INFO) << LOG_BADGE("max_block_range")
                          << LOG_KV("maxBlockRange",
                                 mutableEventLogFilterManagerParams().maxBlockRange)
                          << LOG_KV("maxBlockPerProcess",
                                 mutableEventLogFilterManagerParams().maxBlockPerProcess);
}

void LedgerParam::initFlowControlConfig(boost::property_tree::ptree const& _pt)
{
    auto maxQPS =
        _pt.get<int64_t>("flow_control.limit_req", mutableFlowControlParam().maxDefaultValue);
    if (maxQPS <= 0)
    {
        BOOST_THROW_EXCEPTION(
            InvalidConfiguration() << errinfo_comment("flow_control.limit_req must be positive"));
    }
    mutableFlowControlParam().maxQPS = maxQPS;
    auto outGoingBandwidth = _pt.get<double>(
        "flow_control.outgoing_bandwidth_limit", mutableFlowControlParam().maxDefaultValue);
    // values configured using configuration items
    if (outGoingBandwidth != (double)mutableFlowControlParam().maxDefaultValue)
    {
        if (outGoingBandwidth <= (double)(-0.0) || outGoingBandwidth >= (double)MAX_VALUE_IN_Mb)
        {
            BOOST_THROW_EXCEPTION(InvalidConfiguration()
                                  << errinfo_comment("flow_control.outgoing_bandwidth_limit must "
                                                     "be larger than 0 and smaller than " +
                                                     std::to_string(MAX_VALUE_IN_Mb)));
        }
        outGoingBandwidth = outGoingBandwidth * 1024 * 1024 / 8;
        mutableFlowControlParam().outGoingBandwidthLimit = (int64_t)(outGoingBandwidth);
    }
    else
    {
        LedgerParam_LOG(DEBUG) << LOG_BADGE("initFlowControlConfig")
                               << LOG_DESC(
                                      "disable NetworkBandWidthLimiter for "
                                      "flow_control.outgoing_bandwidth_limit is not configured");
        mutableFlowControlParam().outGoingBandwidthLimit =
            mutableFlowControlParam().maxDefaultValue;
    }

    LedgerParam_LOG(INFO) << LOG_BADGE("initFlowControlConfig")
                          << LOG_KV("maxQPS", mutableFlowControlParam().maxQPS)
                          << LOG_KV("outGoingBandwidth(Bytes)",
                                 mutableFlowControlParam().outGoingBandwidthLimit);
}

void LedgerParam::parsePublicKeyListOfSection(dev::h512s& _nodeList,
    boost::property_tree::ptree const& _pt, std::string const& _sectionName,
    std::string const& _subSectionName)
{
    if (!_pt.get_child_optional(_sectionName))
    {
        LedgerParam_LOG(DEBUG) << LOG_DESC("parsePublicKeyListOfSection return for empty config")
                               << LOG_KV("sectionName", _sectionName);
        return;
    }
    for (auto const& it : _pt.get_child(_sectionName))
    {
        if (it.first.find(_subSectionName) != 0)
        {
            continue;
        }
        std::string data = it.second.data();
        boost::to_lower(data);
        if (!isNodeIDOk(data))
        {
            LedgerParam_LOG(WARNING)
                << LOG_BADGE("load public key: invalid public key") << LOG_KV("invalidPubKey", data)
                << LOG_KV("sectionName", _sectionName);
            BOOST_THROW_EXCEPTION(InvalidConfiguration() << errinfo_comment(
                                      "load public key failed, invalid public key:" + data +
                                      ", configuration section:" + _sectionName));
        }
        LedgerParam_LOG(INFO) << LOG_BADGE("parsePublicKeyListOfSection")
                              << LOG_KV("sectionName", _sectionName) << LOG_KV("it.first", data);
        _nodeList.push_back(dev::h512(data));
    }
    LedgerParam_LOG(INFO) << LOG_BADGE("parsePublicKeyListOfSection")
                          << LOG_KV("totalPubKeySize", _nodeList.size());
}

void LedgerParam::parseSDKAllowList(dev::h512s& _nodeList, boost::property_tree::ptree const& _pt)
{
    parsePublicKeyListOfSection(_nodeList, _pt, "sdk_allowlist", "public_key.");
    bool enableSDKAllowListControl = (_nodeList.size() > 0);
    LedgerParam_LOG(INFO) << LOG_DESC("parseSDKAllowList") << LOG_KV("sdkAllowList", _nodeList)
                          << LOG_KV("enableSDKAllowListControl", enableSDKAllowListControl);
}

}  // namespace ledger
}  // namespace dev
