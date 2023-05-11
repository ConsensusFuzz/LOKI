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
/** @file ScalableStorage.h
 *  @author xingqiangbai
 *  @date 20190917
 */

#include "ScalableStorage.h"
#include "StorageException.h"
#include "libdevcore/Common.h"
#include "libdevcore/Guards.h"
#include "libdevcore/Log.h"
#include <boost/lexical_cast.hpp>
#include <algorithm>
#include <mutex>

using namespace std;
using namespace boost;
using namespace dev;
using namespace dev::storage;

const string TABLE_BLOCK_TO_DB_NAME = "s_block_to_dbname";
const string DB_NAME = "dbName";

ScalableStorage::ScalableStorage(int64_t _scrollThreshold)
  : m_scrollThreshold(_scrollThreshold), m_remoteBlockNumber(-1)
{}

ScalableStorage::~ScalableStorage()
{
    stop();
}

int64_t ScalableStorage::setRemoteBlockNumber(int64_t _blockNumber)
{
    if (_blockNumber <= m_remoteBlockNumber.load())
    {
        return m_remoteBlockNumber.load();
    }
    m_remoteBlockNumber.store(_blockNumber);
    SCALABLE_STORAGE_LOG(DEBUG) << "setRemoteBlockNumber"
                                << LOG_KV("remoteNumber", m_remoteBlockNumber.load());
    return m_remoteBlockNumber.load();
}

bool ScalableStorage::isStateData(const string& _tableName)
{
    return m_archiveTables.end() ==
           find(m_archiveTables.begin(), m_archiveTables.end(), _tableName);
}

string ScalableStorage::getDBNameOfArchivedBlock(int64_t _blockNumber, bool _noException)
{
    auto tableInfo = std::make_shared<storage::TableInfo>();
    tableInfo->name = TABLE_BLOCK_TO_DB_NAME;
    tableInfo->key = "number";
    auto entries = m_state->select(_blockNumber, tableInfo, to_string(_blockNumber), nullptr);
    if (!entries || entries->size() == 0)
    {
        if (_noException)
        {
            return "";
        }
        SCALABLE_STORAGE_LOG(FATAL)
            << "Can't find archived block's dbName" << LOG_KV("blockNumber", _blockNumber);
    }
    return entries->get(0)->getField(DB_NAME);
}

Entries::Ptr ScalableStorage::selectFromArchive(
    int64_t num, TableInfo::Ptr tableInfo, const string& key, Condition::Ptr condition)
{
    Guard l(m_archiveMutex);
    if (num < m_archiveDBName)
    {
        auto dbName = lexical_cast<int64_t>(getDBNameOfArchivedBlock(num, false));
        auto dataStorage = m_storageFactory->getStorage(to_string(dbName), false);
        if (!dataStorage)
        {
            SCALABLE_STORAGE_LOG(ERROR)
                << "archive DB not exists" << LOG_KV("currentDBName", m_archiveDBName)
                << LOG_KV("dbName", dbName) << LOG_KV("key", key);
            return std::make_shared<Entries>();
        }
        SCALABLE_STORAGE_LOG(DEBUG)
            << "select from archive DB" << LOG_KV("currentDBName", m_archiveDBName)
            << LOG_KV("dbName", dbName) << LOG_KV("key", key);
        return dataStorage->select(num, tableInfo, key, condition);
    }
    else
    {
        SCALABLE_STORAGE_LOG(DEBUG)
            << "select from current DB" << LOG_KV("currentDBName", m_archiveDBName)
            << LOG_KV("key", key) << LOG_KV("number", num);
        return m_archive->select(num, tableInfo, key, condition);
    }
    return nullptr;
}

Entries::Ptr ScalableStorage::select(
    int64_t num, TableInfo::Ptr tableInfo, const string& key, Condition::Ptr condition)
{
    if (isStateData(tableInfo->name))
    {
        SCALABLE_STORAGE_LOG(TRACE)
            << "select from state DB" << LOG_KV("dbName", tableInfo->name)
            << LOG_KV("key", key) << LOG_KV("number", num);
        return m_state->select(num, tableInfo, key, condition);
    }
    else
    {
        if (num < m_remoteBlockNumber.load() && m_remote)
        {
            SCALABLE_STORAGE_LOG(DEBUG) << "select from remote" << LOG_KV("table", tableInfo->name)
                                        << LOG_KV("key", key) << LOG_KV("number", num);
            auto entries = m_remote->select(num, tableInfo, key, condition);
            if (num > 0 && entries->size() == 0)
            {
                SCALABLE_STORAGE_LOG(FATAL) << "select from remote DB failed";
            }
            conversionData(tableInfo->name, entries);
            return entries;
        }
        else if (m_archive)
        {
            return selectFromArchive(num, tableInfo, key, condition);
        }
    }
    SCALABLE_STORAGE_LOG(FATAL) << "Can't find data or remote storage died, please check!";
    return nullptr;
}

void ScalableStorage::conversionData(const std::string& tableName, Entries::Ptr entries)
{
    if (m_archiveTables.end() != find(m_archiveTables.begin(), m_archiveTables.end(), tableName))
    {
        LOG(TRACE) << LOG_BADGE("ScalableStorage") << LOG_DESC("conversion table data") << LOG_KV("table name", tableName);
        for (size_t i = 0; i < entries->size(); i++)
        {
            auto entry = entries->get(i);
            auto dataStr = entry->getField("value");
            auto dataBytes = std::make_shared<bytes>(fromHex(dataStr.c_str()));
            entry->setField("value", dataBytes->data(), dataBytes->size());
        }
    } 
    else 
    {
        LOG(ERROR) << LOG_BADGE("ScalableStorage") << LOG_DESC("invalid table data") << LOG_KV("table name", tableName);
    }
}

void ScalableStorage::separateData(const vector<TableData::Ptr>& datas,
    vector<TableData::Ptr>& stateData, vector<TableData::Ptr>& archiveData)
{
    for (auto const& tableData : datas)
    {
        if (isStateData(tableData->info->name))
        {
            stateData.emplace_back(tableData);
        }
        else
        {
            archiveData.emplace_back(tableData);
        }
    }
}

TableData::Ptr ScalableStorage::getNumberToDBNameData(int64_t _blockNumber)
{
    // prepare block number dbName map
    auto tableData = std::make_shared<storage::TableData>();
    auto tableInfo = std::make_shared<storage::TableInfo>();
    tableInfo->name = TABLE_BLOCK_TO_DB_NAME;
    tableInfo->key = "number";
    tableData->info = tableInfo;
    tableData->newEntries = std::make_shared<Entries>();
    Entry::Ptr dbNameEntry = std::make_shared<Entry>();
    dbNameEntry->setNum(_blockNumber);
    dbNameEntry->setStatus(0);
    dbNameEntry->setField("number", to_string(_blockNumber));
    dbNameEntry->setField(DB_NAME, to_string(m_archiveDBName));
    tableData->newEntries->addEntry(dbNameEntry);
    return tableData;
}

size_t ScalableStorage::commit(int64_t num, const vector<TableData::Ptr>& datas)
{
    SCALABLE_STORAGE_LOG(DEBUG) << "commit" << LOG_KV("size", datas.size()) << LOG_KV("block", num)
                                << LOG_KV("dbName", m_archiveDBName);
    vector<TableData::Ptr> archiveData;
    archiveData.reserve(m_archiveTables.size());
    vector<TableData::Ptr> stateData;
    separateData(datas, stateData, archiveData);
    size_t size = 0;
    stateData.push_back(getNumberToDBNameData(num));
    size += m_archive->commit(num, archiveData);
    size += m_state->commit(num, stateData);
    if ((num + 1) % m_scrollThreshold == 0)
    {
        Guard l(m_archiveMutex);
        m_archiveDBName = num + 1;
        m_archive = m_storageFactory->getStorage(to_string(m_archiveDBName), true);
        SCALABLE_STORAGE_LOG(DEBUG)
            << "create new Storage" << LOG_KV("block", num) << LOG_KV("dbIndex", m_archiveDBName);
    }
    return size;
}

void ScalableStorage::stop()
{
    if (m_remote)
    {
        m_remote->stop();
    }
    SCALABLE_STORAGE_LOG(INFO) << "stopped";
}
