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
/** @file MemoryTable2.cpp
 *  @author ancelmo
 *  @date 20180921
 */
#include "MemoryTable2.h"
#include "Common.h"
#include "StorageException.h"
#include "Table.h"
#include <arpa/inet.h>
#include <json/json.h>
#include <libconfig/GlobalConfigure.h>
#include <libdevcore/FixedHash.h>
#include <libdevcrypto/CryptoInterface.h>
#include <libprecompiled/Common.h>
#include <tbb/parallel_invoke.h>
#include <tbb/parallel_sort.h>
#include <boost/exception/diagnostic_information.hpp>
#include <boost/lexical_cast.hpp>
#include <algorithm>
#include <csignal>
#include <thread>
#include <vector>

using namespace std;
using namespace dev;
using namespace dev::storage;
using namespace dev::precompiled;

void prepareExit(const std::string& _key)
{
    STORAGE_LOG(ERROR) << LOG_BADGE("MemoryTable2 prepare to exit") << LOG_KV("key", _key);
    raise(SIGTERM);
    while (!g_BCOSConfig.shouldExit.load())
    {  // wait to exit
        std::this_thread::yield();
    }
    BOOST_THROW_EXCEPTION(
        StorageException(-1, string("backend DB is dead. Prepare to exit.") + _key));
}

Entries::ConstPtr MemoryTable2::select(const std::string& key, Condition::Ptr condition)
{
    return selectNoLock(key, condition);
}

void MemoryTable2::proccessLimit(
    const Condition::Ptr& condition, const Entries::Ptr& entries, const Entries::Ptr& resultEntries)
{
    int begin = condition->getOffset();
    int end = 0;

    if (g_BCOSConfig.version() < V2_1_0)
    {
        end = begin + condition->getCount();
    }
    else
    {
        end = (int)std::min((size_t)INT_MAX, (size_t)begin + (size_t)condition->getCount());
    }

    int size = entries->size();
    if (begin >= size)
    {
        return;
    }
    else
    {
        end = end > size ? size : end;
    }
    for (int i = begin; i < end; i++)
    {
        resultEntries->addEntry(entries->get(i));
    }
}

Entries::Ptr MemoryTable2::selectNoLock(const std::string& key, Condition::Ptr condition)
{
    try
    {
        auto entries = std::make_shared<Entries>();
        condition->EQ(m_tableInfo->key, key);
        if (m_remoteDB)
        {
            // query remoteDB anyway
            Entries::Ptr dbEntries = m_remoteDB->select(m_blockNum, m_tableInfo, key, condition);
            if (!dbEntries)
            {
                return entries;
            }
            std::set<uint64_t> processed;
            std::set<uint64_t> diff;
            for (size_t i = 0; i < dbEntries->size(); ++i)
            {
                auto entryIt = m_dirty.find(dbEntries->get(i)->getID());
                if (entryIt != m_dirty.end())
                {
                    processed.insert(entryIt->second->getID());
                    if (g_BCOSConfig.version() >= V2_5_0 && !condition->process(entryIt->second))
                    {
                        continue;
                    }
                    entries->addEntry(entryIt->second);
                }
                else
                {
                    entries->addEntry(dbEntries->get(i));
                }
            }
            if (g_BCOSConfig.version() >= V2_5_0)
            {
                std::set_difference(m_dirty_updated[key].begin(), m_dirty_updated[key].end(),
                    processed.begin(), processed.end(), std::inserter(diff, diff.begin()));
                for (auto id : diff)
                {
                    if (condition->process(m_dirty[id]))
                    {
                        entries->addEntry(m_dirty[id]);
                    }
                }
            }
        }

        auto it = m_newEntries.find(key);
        if (it != m_newEntries.end())
        {
            auto indices = processEntries(it->second, condition);
            for (auto& itIndex : indices)
            {
                it->second->get(itIndex)->setTempIndex(itIndex);
                entries->addEntry(it->second->get(itIndex));
            }
        }
        if (condition->getOffset() >= 0 && condition->getCount() >= 0)
        {
            Entries::Ptr resultEntries = std::make_shared<Entries>();
            proccessLimit(condition, entries, resultEntries);
            return resultEntries;
        }
        return entries;
    }
    catch (std::exception& e)
    {
        STORAGE_LOG(ERROR) << LOG_BADGE("MemoryTable2") << LOG_DESC("Table select failed")
                           << LOG_KV("what", e.what());
        m_remoteDB->stop();
        prepareExit(key);
    }

    return std::make_shared<Entries>();
}

int MemoryTable2::update(
    const std::string& key, Entry::Ptr entry, Condition::Ptr condition, AccessOptions::Ptr options)
{
    try
    {
        if (options->check && !checkAuthority(options->origin))
        {
            STORAGE_LOG(WARNING) << LOG_BADGE("MemoryTable2")
                                 << LOG_DESC("update permission denied")
                                 << LOG_KV("origin", options->origin.hex()) << LOG_KV("key", key);

            return storage::CODE_NO_AUTHORIZED;
        }

        checkField(entry);

        auto entries = selectNoLock(key, condition);
        std::vector<Change::Record> records;

        for (size_t i = 0; i < entries->size(); ++i)
        {
            Entry::Ptr updateEntry = entries->get(i);

            // if id not equals to zero and not in the m_dirty, must be new dirty entry
            if (updateEntry->getID() != 0 && m_dirty.find(updateEntry->getID()) == m_dirty.end())
            {
                m_dirty.insert(std::make_pair(updateEntry->getID(), updateEntry));
                m_dirty_updated[key].insert(updateEntry->getID());
            }

            for (auto& it : *(entry))
            {
                // _id_ always got initialized value 0 from Entry::Entry()
                // no need to update _id_ while updating entry
                if (it.first != ID_FIELD && it.first != m_tableInfo->key)
                {
                    records.emplace_back(updateEntry->getTempIndex(), it.first,
                        updateEntry->getField(it.first), updateEntry->getID());
                    updateEntry->setField(it.first, it.second);
                }
            }
        }

        m_recorder(shared_from_this(), Change::Update, key, records);

        m_hashDirty = true;
        m_dataDirty = true;
        return entries->size();
    }
    catch (std::invalid_argument& e)
    {
        BOOST_THROW_EXCEPTION(e);
    }
    catch (std::exception& e)
    {
        STORAGE_LOG(ERROR) << LOG_BADGE("MemoryTable2")
                           << LOG_DESC("Access MemoryTable2 failed for")
                           << LOG_KV("msg", boost::diagnostic_information(e));
        m_remoteDB->stop();
        prepareExit(key);
    }

    return 0;
}

int MemoryTable2::insert(const std::string& key, Entry::Ptr entry, AccessOptions::Ptr options, bool)
{
    try
    {
        if (options->check && !checkAuthority(options->origin))
        {
            STORAGE_LOG(WARNING) << LOG_BADGE("MemoryTable2")
                                 << LOG_DESC("insert permission denied")
                                 << LOG_KV("origin", options->origin.hex()) << LOG_KV("key", key);
            return storage::CODE_NO_AUTHORIZED;
        }

        checkField(entry);

        entry->setField(m_tableInfo->key, key);
        auto it = m_newEntries.find(key);

        if (it == m_newEntries.end())
        {
            Entries::Ptr entries = std::make_shared<Entries>();
            it = m_newEntries.insert(std::make_pair(key, entries)).first;
        }
        auto iter = it->second->addEntry(entry);

        // auto iter = m_newEntries->addEntry(entry);
        Change::Record record(iter);

        std::vector<Change::Record> value{record};
        m_recorder(shared_from_this(), Change::Insert, key, value);

        m_hashDirty = true;
        m_dataDirty = true;
        return 1;
    }
    catch (std::invalid_argument& e)
    {
        BOOST_THROW_EXCEPTION(e);
    }
    catch (std::exception& e)
    {
        // impossible, so exit
        STORAGE_LOG(FATAL) << LOG_BADGE("MemoryTable2")
                           << LOG_DESC("Access MemoryTable2 failed for")
                           << LOG_KV("msg", boost::diagnostic_information(e));
    }

    return 0;
}

int MemoryTable2::remove(
    const std::string& key, Condition::Ptr condition, AccessOptions::Ptr options)
{
    try
    {
        if (options->check && !checkAuthority(options->origin))
        {
            STORAGE_LOG(WARNING) << LOG_BADGE("MemoryTable2")
                                 << LOG_DESC("remove permission denied")
                                 << LOG_KV("origin", options->origin.hex()) << LOG_KV("key", key);
            return storage::CODE_NO_AUTHORIZED;
        }

        auto entries = selectNoLock(key, condition);

        std::vector<Change::Record> records;
        for (size_t i = 0; i < entries->size(); ++i)
        {
            Entry::Ptr removeEntry = entries->get(i);

            removeEntry->setStatus(1);

            // if id not equals to zero and not in the m_dirty, must be new dirty entry
            if (removeEntry->getID() != 0 && m_dirty.find(removeEntry->getID()) == m_dirty.end())
            {
                m_dirty.insert(std::make_pair(removeEntry->getID(), removeEntry));
            }

            records.emplace_back(removeEntry->getTempIndex(), "", "", removeEntry->getID());
        }

        m_recorder(shared_from_this(), Change::Remove, key, records);

        m_hashDirty = true;
        m_dataDirty = true;
        return entries->size();
    }
    catch (std::exception& e)
    {  // this catch is redundant, because selectNoLock already catch.
        // TODO: make catch simple, remove catch in selectNoLock
        STORAGE_LOG(ERROR) << LOG_BADGE("MemoryTable2")
                           << LOG_DESC("Access MemoryTable2 failed for")
                           << LOG_KV("msg", boost::diagnostic_information(e));
        m_remoteDB->stop();
        prepareExit(key);
    }

    return 0;
}

dev::h256 MemoryTable2::hash()
{
    if (m_hashDirty)
    {
        m_tableData.reset(new dev::storage::TableData());
        if (g_BCOSConfig.version() < V2_2_0)
        {
            dumpWithoutOptimize();
        }
        else
        {
            dump();
        }
    }

    return m_hash;
}

dev::storage::TableData::Ptr MemoryTable2::dumpWithoutOptimize()
{
    TIME_RECORD("MemoryTable2 Dump");
    if (m_hashDirty)
    {
        m_tableData = std::make_shared<dev::storage::TableData>();
        m_tableData->info = m_tableInfo;
        m_tableData->dirtyEntries = std::make_shared<Entries>();

        auto tempEntries = tbb::concurrent_vector<Entry::Ptr>();

        tbb::parallel_for(m_dirty.range(),
            [&](tbb::concurrent_unordered_map<uint64_t, Entry::Ptr>::range_type& range) {
                for (auto it = range.begin(); it != range.end(); ++it)
                {
                    if (!it->second->deleted())
                    {
                        m_tableData->dirtyEntries->addEntry(it->second);
                        tempEntries.push_back(it->second);
                    }
                }
            });

        m_tableData->newEntries = std::make_shared<Entries>();
        tbb::parallel_for(m_newEntries.range(),
            [&](tbb::concurrent_unordered_map<std::string, Entries::Ptr>::range_type& range) {
                for (auto it = range.begin(); it != range.end(); ++it)
                {
                    tbb::parallel_for(tbb::blocked_range<size_t>(0, it->second->size(), 1000),
                        [&](tbb::blocked_range<size_t>& rangeIndex) {
                            for (auto i = rangeIndex.begin(); i < rangeIndex.end(); ++i)
                            {
                                if (!it->second->get(i)->deleted())
                                {
                                    m_tableData->newEntries->addEntry(it->second->get(i));
                                    tempEntries.push_back(it->second->get(i));
                                }
                            }
                        });
                }
            });

        TIME_RECORD("Sort data");
        tbb::parallel_sort(tempEntries.begin(), tempEntries.end(), EntryLessNoLock(m_tableInfo));
        tbb::parallel_sort(m_tableData->dirtyEntries->begin(), m_tableData->dirtyEntries->end(),
            EntryLessNoLock(m_tableInfo));
        tbb::parallel_sort(m_tableData->newEntries->begin(), m_tableData->newEntries->end(),
            EntryLessNoLock(m_tableInfo));
        TIME_RECORD("Submmit data");
        bytes allData;
        for (size_t i = 0; i < tempEntries.size(); ++i)
        {
            auto entry = tempEntries[i];
            if (g_BCOSConfig.version() < RC3_VERSION)
            {  // RC2 STATUS is in entry fields
                entry->setField(STATUS, to_string(entry->getStatus()));
            }
            for (auto fieldIt : *(entry))
            {
                if (isHashField(fieldIt.first))
                {
                    allData.insert(allData.end(), fieldIt.first.begin(), fieldIt.first.end());
                    allData.insert(allData.end(), fieldIt.second.begin(), fieldIt.second.end());
                }
            }
            if (g_BCOSConfig.version() < RC3_VERSION)
            {
                continue;
            }
            char status = (char)entry->getStatus();
            allData.insert(allData.end(), &status, &status + sizeof(status));
        }
        if (allData.empty())
        {
            m_hash = h256();
        }

        bytesConstRef bR(allData.data(), allData.size());

        if (g_BCOSConfig.SMCrypto())
        {
            m_hash = dev::sm3(bR);
        }
        else
        {
            m_hash = dev::sha256(bR);
        }
        m_hashDirty = false;
    }

    return m_tableData;
}

void MemoryTable2::parallelGenData(
    bytes& _generatedData, std::shared_ptr<std::vector<size_t>> _offsetVec, Entries::Ptr _entries)
{
    if (_entries->size() == 0)
    {
        return;
    }
    tbb::parallel_for(tbb::blocked_range<uint64_t>(0, _offsetVec->size() - 1),
        [&](const tbb::blocked_range<uint64_t>& range) {
            for (uint64_t i = range.begin(); i < range.end(); i++)
            {
                auto entry = (*_entries)[i];
                auto startOffSet = (*_offsetVec)[i];

                for (auto& fieldIt : *(entry))
                {
                    if (isHashField(fieldIt.first))
                    {
                        memcpy(
                            &_generatedData[startOffSet], &fieldIt.first[0], fieldIt.first.size());
                        startOffSet += fieldIt.first.size();

                        memcpy(&_generatedData[startOffSet], &fieldIt.second[0],
                            fieldIt.second.size());
                        startOffSet += fieldIt.second.size();
                    }
                }
                char status = (char)entry->getStatus();
                memcpy(&_generatedData[startOffSet], &status, sizeof(status));
            }
        });
}

std::shared_ptr<std::vector<size_t>> MemoryTable2::genDataOffset(
    Entries::Ptr _entries, size_t _startOffset)
{
    std::shared_ptr<std::vector<size_t>> dataOffset = std::make_shared<std::vector<size_t>>();
    dataOffset->push_back(_startOffset);
    for (size_t i = 0; i < _entries->size(); i++)
    {
        auto entry = (*_entries)[i];
        // 1 for status field
        auto offset = (*dataOffset)[i] + entry->capacityOfHashField() + 1;
        dataOffset->push_back(offset);
    }
    return dataOffset;
}

dev::storage::TableData::Ptr MemoryTable2::dump()
{
    // >= v2.2.0
    TIME_RECORD("MemoryTable2 Dump-" + m_tableInfo->name);
    if (m_hashDirty)
    {
        tbb::atomic<size_t> allSize = 0;

        m_tableData = std::make_shared<dev::storage::TableData>();
        m_tableData->info = m_tableInfo;
        m_tableData->dirtyEntries = std::make_shared<Entries>();

        tbb::parallel_for(m_dirty.range(),
            [&](tbb::concurrent_unordered_map<uint64_t, Entry::Ptr>::range_type& range) {
                for (auto it = range.begin(); it != range.end(); ++it)
                {
                    if (!it->second->deleted())
                    {
                        m_tableData->dirtyEntries->addEntry(it->second);
                        allSize += (it->second->capacityOfHashField() + 1);  // 1 for status field
                    }
                }
            });

        m_tableData->newEntries = std::make_shared<Entries>();
        tbb::parallel_for(m_newEntries.range(),
            [&](tbb::concurrent_unordered_map<std::string, Entries::Ptr>::range_type& range) {
                for (auto it = range.begin(); it != range.end(); ++it)
                {
                    tbb::parallel_for(tbb::blocked_range<size_t>(0, it->second->size(), 1000),
                        [&](tbb::blocked_range<size_t>& rangeIndex) {
                            for (auto i = rangeIndex.begin(); i < rangeIndex.end(); ++i)
                            {
                                if (!it->second->get(i)->deleted())
                                {
                                    m_tableData->newEntries->addEntry(it->second->get(i));
                                    // 1 for status field
                                    allSize += (it->second->get(i)->capacityOfHashField() + 1);
                                }
                            }
                        });
                }
            });

        if (m_tableInfo->enableConsensus)
        {
            TIME_RECORD("Sort data");
            tbb::parallel_sort(m_tableData->dirtyEntries->begin(), m_tableData->dirtyEntries->end(),
                EntryLessNoLock(m_tableInfo));
            tbb::parallel_sort(m_tableData->newEntries->begin(), m_tableData->newEntries->end(),
                EntryLessNoLock(m_tableInfo));
            TIME_RECORD("Calc hash");

            auto startT = utcTime();
            std::shared_ptr<bytes> allData = std::make_shared<bytes>();
            allData->resize(allSize);
            // get offset for dirtyEntries
            size_t insertDataStartOffset = 0;
            std::shared_ptr<std::vector<size_t>> dirtyEntriesOffset;
            if (m_tableData->dirtyEntries->size() > 0)
            {
                dirtyEntriesOffset = genDataOffset(m_tableData->dirtyEntries, 0);
                insertDataStartOffset = (*dirtyEntriesOffset)[m_tableData->dirtyEntries->size()];
            }
            // get offset for newEntries
            std::shared_ptr<std::vector<size_t>> newEntriesOffset;
            if (m_tableData->newEntries->size() > 0)
            {
                newEntriesOffset = genDataOffset(m_tableData->newEntries, insertDataStartOffset);
            }
            // Parallel processing dirtyEntries and newEntries
            tbb::parallel_invoke(
                [this, allData, dirtyEntriesOffset]() {
                    parallelGenData(*allData, dirtyEntriesOffset, m_tableData->dirtyEntries);
                },
                [this, allData, newEntriesOffset]() {
                    parallelGenData(*allData, newEntriesOffset, m_tableData->newEntries);
                });


#if FISCO_DEBUG
            auto printEntries = [](const string& tableName, Entries::Ptr entries) {
                if (entries->size() == 0)
                {
                    STORAGE_LOG(DEBUG) << LOG_BADGE("FISCO_DEBUG") << " entries is empty!" << endl;
                    return;
                }
                stringstream ss;
                for (size_t i = 0; i < entries->size(); ++i)
                {
                    auto data = entries->get(i);
                    ss << endl << "***" << i << " [ id=" << data->getID() << " ]";
                    for (auto& it : *data)
                    {
                        ss << "[ " << it.first << "=" << it.second << " ]";
                    }
                }
                STORAGE_LOG(DEBUG)
                    << LOG_BADGE("FISCO_DEBUG") << LOG_KV("TableName", tableName) << ss.str();
            };
            printEntries(m_tableData->info->name, m_tableData->dirtyEntries);
            printEntries(m_tableData->info->name, m_tableData->newEntries);
#endif
            auto writeDataT = utcTime() - startT;
            startT = utcTime();
            bytesConstRef bR(allData->data(), allData->size());
            auto transDataT = utcTime() - startT;
            startT = utcTime();
            if (g_BCOSConfig.version() <= V2_4_0)
            {
                if (g_BCOSConfig.SMCrypto())
                {
                    m_hash = dev::sm3(bR);
                }
                else
                {
                    // in previous version(<= 2.4.0), we use sha256(...) to calculate hash of the
                    // data, for now, to keep consistent with transction's implementation, we decide
                    // to use keccak256(...) to calculate hash of the data. This `else` branch is just
                    // for compatibility.
                    m_hash = dev::sha256(bR);
                }
            }
            else
            {
                m_hash = crypto::Hash(bR);
            }
            auto getHashT = utcTime() - startT;
            STORAGE_LOG(DEBUG) << LOG_BADGE("MemoryTable2 dump") << LOG_KV("writeDataT", writeDataT)
                               << LOG_KV("transDataT", transDataT) << LOG_KV("getHashT", getHashT)
                               << LOG_KV("hash", m_hash.abridged());
        }
        else
        {
            m_hash = dev::h256();

            STORAGE_LOG(DEBUG) << "Ignore sort and hash for: " << m_tableInfo->name
                               << " hash: " << m_hash.hex();
        }
        m_hashDirty = false;
    }

    return m_tableData;
}

void MemoryTable2::rollback(const Change& _change)
{
#if 0
    LOG(TRACE) << "Before rollback newEntries size: " << m_newEntries.size();
#endif
    switch (_change.kind)
    {
    case Change::Insert:
    {
#if 0
        LOG(TRACE) << "Rollback insert record newIndex: " << _change.value[0].index;
#endif

        auto it = m_newEntries.find(_change.key);
        if (it != m_newEntries.end())
        {
            auto entry = it->second->get(_change.value[0].index);
            entry->setDeleted(true);
        }
        break;
    }
    case Change::Update:
    {
        for (auto& record : _change.value)
        {
#if 0
            LOG(TRACE) << "Rollback update record id: " << record.id
                       << " newIndex: " << record.index;
#endif

            if (record.id)
            {
                auto it = m_dirty.find(record.id);
                if (it != m_dirty.end())
                {
                    it->second->setField(record.key, record.oldValue);
                }
            }
            else
            {
                auto it = m_newEntries.find(_change.key);
                if (it != m_newEntries.end())
                {
                    auto entry = it->second->get(record.index);
                    entry->setField(record.key, record.oldValue);
                }
            }
        }
        break;
    }
    case Change::Remove:
    {
        for (auto& record : _change.value)
        {
#if 0
            LOG(TRACE) << "Rollback remove record id: " << record.id
                       << " newIndex: " << record.index;
#endif
            if (record.id)
            {
                auto it = m_dirty.find(record.id);
                if (it != m_dirty.end())
                {
                    it->second->setStatus(0);
                }
            }
            else
            {
                auto it = m_newEntries.find(_change.key);
                if (it != m_newEntries.end())
                {
                    auto entry = it->second->get(record.index);
                    entry->setStatus(0);
                }
            }
        }
        break;
    }
    case Change::Select:

    default:
        break;
    }

    // LOG(TRACE) << "After rollback newEntries size: " << m_newEntries->size();
}
