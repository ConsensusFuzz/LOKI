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
/** @file CRUDPrecompiled.cpp
 *  @author ancelmo
 *  @date 20180921
 */

#include "CRUDPrecompiled.h"
#include "libprecompiled/EntriesPrecompiled.h"
#include "libprecompiled/TableFactoryPrecompiled.h"
#include "libstorage/StorageException.h"
#include <json/json.h>
#include <libdevcore/Common.h>
#include <libdevcrypto/Hash.h>
#include <libethcore/ABI.h>

using namespace std;
using namespace dev;
using namespace dev::blockverifier;
using namespace dev::storage;
using namespace dev::precompiled;

const char* const CRUD_METHOD_INSERT_STR = "insert(string,string,string,string)";
const char* const CRUD_METHOD_REMOVE_STR = "remove(string,string,string,string)";
const char* const CRUD_METHOD_UPDATE_STR = "update(string,string,string,string,string)";
const char* const CRUD_METHOD_SELECT_STR = "select(string,string,string,string)";
const char* const CRUD_METHOD_DESC_STR = "desc(string)";

CRUDPrecompiled::CRUDPrecompiled()
{
    name2Selector[CRUD_METHOD_INSERT_STR] = getFuncSelector(CRUD_METHOD_INSERT_STR);
    name2Selector[CRUD_METHOD_REMOVE_STR] = getFuncSelector(CRUD_METHOD_REMOVE_STR);
    name2Selector[CRUD_METHOD_UPDATE_STR] = getFuncSelector(CRUD_METHOD_UPDATE_STR);
    name2Selector[CRUD_METHOD_SELECT_STR] = getFuncSelector(CRUD_METHOD_SELECT_STR);
    name2Selector[CRUD_METHOD_DESC_STR] = getFuncSelector(CRUD_METHOD_DESC_STR);
}

std::string CRUDPrecompiled::toString()
{
    return "CRUD";
}

PrecompiledExecResult::Ptr CRUDPrecompiled::call(
    ExecutiveContext::Ptr context, bytesConstRef param, Address const& origin, Address const&)
{
    PRECOMPILED_LOG(TRACE) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("call")
                           << LOG_KV("param", toHex(param));
    // parse function name
    uint32_t func = getParamFunc(param);
    bytesConstRef data = getParamData(param);

    dev::eth::ContractABI abi;
    auto callResult = m_precompiledExecResultFactory->createPrecompiledResult();
    callResult->gasPricer()->setMemUsed(param.size());

    if (func == name2Selector[CRUD_METHOD_DESC_STR])
    {  // desc(string)
        std::string tableName;
        abi.abiOut(data, tableName);
        tableName = precompiled::getTableName(tableName);
        Table::Ptr table = openTable(context, storage::SYS_TABLES);
        callResult->gasPricer()->appendOperation(InterfaceOpcode::OpenTable);
        auto entries = table->select(tableName, table->newCondition());
        // Note: Because the selected data has been returned, the memory is not updated here
        callResult->gasPricer()->appendOperation(InterfaceOpcode::Select, entries->size());

        string keyField, valueFiled;
        if (entries->size() != 0)
        {
            auto entry = entries->get(0);
            keyField = entry->getField("key_field");
            valueFiled = entry->getField("value_field");
        }
        else
        {
            PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("table not exist")
                                   << LOG_KV("tableName", tableName);
        }
        callResult->setExecResult(abi.abiIn("", keyField, valueFiled));
        return callResult;
    }
    else if (func == name2Selector[CRUD_METHOD_INSERT_STR])
    {  // insert(string tableName, string key, string entry, string optional)
        std::string tableName, key, entryStr, optional;
        abi.abiOut(data, tableName, key, entryStr, optional);
        checkLengthValidate(
            key, USER_TABLE_KEY_VALUE_MAX_LENGTH, CODE_TABLE_KEYVALUE_LENGTH_OVERFLOW);

        tableName = precompiled::getTableName(tableName);
        Table::Ptr table = openTable(context, tableName);
        callResult->gasPricer()->appendOperation(InterfaceOpcode::OpenTable);
        if (table)
        {
            Entry::Ptr entry = table->newEntry();
            int parseEntryResult = parseEntry(entryStr, entry);
            if (parseEntryResult != CODE_SUCCESS)
            {
                callResult->setExecResult(abi.abiIn("", u256(parseEntryResult)));
                return callResult;
            }

            auto it = entry->begin();
            for (; it != entry->end(); ++it)
            {
                checkLengthValidate(it->second, USER_TABLE_FIELD_VALUE_MAX_LENGTH,
                    CODE_TABLE_KEYVALUE_LENGTH_OVERFLOW);
            }

            int result = table->insert(key, entry, std::make_shared<AccessOptions>(origin));
            if (result > 0)
            {
                callResult->gasPricer()->appendOperation(InterfaceOpcode::Insert, result);
                callResult->gasPricer()->updateMemUsed(entry->capacity() * result);
            }
            callResult->setExecResult(abi.abiIn("", u256(result)));
        }
        else
        {
            PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("table open error")
                                   << LOG_KV("tableName", tableName);
            callResult->setExecResult(abi.abiIn("", u256(int32_t(CODE_TABLE_NOT_EXIST))));
        }

        return callResult;
    }
    if (func == name2Selector[CRUD_METHOD_UPDATE_STR])
    {  // update(string tableName, string key, string entry, string condition, string optional)
        std::string tableName, key, entryStr, conditionStr, optional;
        abi.abiOut(data, tableName, key, entryStr, conditionStr, optional);
        tableName = precompiled::getTableName(tableName);
        Table::Ptr table = openTable(context, tableName);
        callResult->gasPricer()->appendOperation(InterfaceOpcode::OpenTable);
        if (table)
        {
            Entry::Ptr entry = table->newEntry();
            int parseEntryResult = parseEntry(entryStr, entry);
            if (parseEntryResult != CODE_SUCCESS)
            {
                callResult->setExecResult(abi.abiIn("", u256(parseEntryResult)));
                return callResult;
            }
            // check the entry
            if (entry->fieldExist(table->tableInfo()->key) && g_BCOSConfig.version() >= V2_7_0)
            {
                PRECOMPILED_LOG(ERROR)
                    << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("can't update the key")
                    << LOG_KV("table", tableName) << LOG_KV("key", table->tableInfo()->key);
                callResult->setExecResult(
                    abi.abiIn("", u256(int32_t(CODE_INVALID_UPDATE_TABLE_KEY))));
                return callResult;
            }
            Condition::Ptr condition = table->newCondition();
            int parseConditionResult = parseCondition(conditionStr, condition, callResult);
            if (parseConditionResult != CODE_SUCCESS)
            {
                callResult->setExecResult(abi.abiIn("", u256(parseConditionResult)));
                return callResult;
            }

            auto it = entry->begin();
            for (; it != entry->end(); ++it)
            {
                checkLengthValidate(it->second, USER_TABLE_FIELD_VALUE_MAX_LENGTH,
                    CODE_TABLE_KEYVALUE_LENGTH_OVERFLOW);
            }

            int result =
                table->update(key, entry, condition, std::make_shared<AccessOptions>(origin));
            if (result > 0)
            {
                callResult->gasPricer()->updateMemUsed(entry->capacity() * result);
                callResult->gasPricer()->appendOperation(InterfaceOpcode::Update, result);
            }
            callResult->setExecResult(abi.abiIn("", u256(result)));
        }
        else
        {
            PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("table open error")
                                   << LOG_KV("tableName", tableName);
            callResult->setExecResult(abi.abiIn("", u256(int32_t(CODE_TABLE_NOT_EXIST))));
        }

        return callResult;
    }
    if (func == name2Selector[CRUD_METHOD_REMOVE_STR])
    {  // remove(string tableName, string key, string condition, string optional)
        std::string tableName, key, conditionStr, optional;
        abi.abiOut(data, tableName, key, conditionStr, optional);
        tableName = precompiled::getTableName(tableName);
        Table::Ptr table = openTable(context, tableName);
        callResult->gasPricer()->appendOperation(InterfaceOpcode::OpenTable);
        if (table)
        {
            Condition::Ptr condition = table->newCondition();
            int parseConditionResult = parseCondition(conditionStr, condition, callResult);
            if (parseConditionResult != CODE_SUCCESS)
            {
                callResult->setExecResult(abi.abiIn("", u256(parseConditionResult)));
                return callResult;
            }
            int result = table->remove(key, condition, std::make_shared<AccessOptions>(origin));
            if (result > 0)
            {
                callResult->gasPricer()->appendOperation(InterfaceOpcode::Remove, result);
            }
            callResult->setExecResult(abi.abiIn("", u256(result)));
        }
        else
        {
            PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("table open error")
                                   << LOG_KV("tableName", tableName);
            callResult->setExecResult(abi.abiIn("", u256(int32_t(CODE_TABLE_NOT_EXIST))));
        }

        return callResult;
    }
    if (func == name2Selector[CRUD_METHOD_SELECT_STR])
    {  // select(string tableName, string key, string condition, string optional)
        std::string tableName, key, conditionStr, optional;
        abi.abiOut(data, tableName, key, conditionStr, optional);
        if (tableName != storage::SYS_TABLES)
        {
            tableName = precompiled::getTableName(tableName);
        }
        Table::Ptr table = openTable(context, tableName);
        callResult->gasPricer()->appendOperation(InterfaceOpcode::OpenTable);
        if (table)
        {
            Condition::Ptr condition = table->newCondition();
            int parseConditionResult = parseCondition(conditionStr, condition, callResult);
            if (parseConditionResult != CODE_SUCCESS)
            {
                callResult->setExecResult(abi.abiIn("", u256(parseConditionResult)));
                return callResult;
            }
            auto entries = table->select(key, condition);
            callResult->gasPricer()->appendOperation(InterfaceOpcode::Select, entries->size());
            Json::Value records = Json::Value(Json::arrayValue);
            if (entries)
            {
                for (size_t i = 0; i < entries->size(); i++)
                {
                    auto entry = entries->get(i);
                    Json::Value record;
                    for (auto iter = entry->begin(); iter != entry->end(); iter++)
                    {
                        record[iter->first] = iter->second;
                    }
                    records.append(record);
                }
            }

            auto str = records.toStyledString();
            callResult->setExecResult(abi.abiIn("", str));
        }
        else
        {
            PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("table open error")
                                   << LOG_KV("tableName", tableName);
            callResult->setExecResult(abi.abiIn("", u256(int32_t(CODE_TABLE_NOT_EXIST))));
        }

        return callResult;
    }
    else
    {
        PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled")
                               << LOG_DESC("call undefined function") << LOG_KV("func", func);
        callResult->setExecResult(abi.abiIn("", u256(int32_t(CODE_UNKNOW_FUNCTION_CALL))));

        return callResult;
    }
}

int CRUDPrecompiled::parseCondition(const std::string& conditionStr, Condition::Ptr& condition,
    PrecompiledExecResult::Ptr _execResult)
{
    Json::Reader reader;
    Json::Value conditionJson;
    if (!reader.parse(conditionStr, conditionJson))
    {
        PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled")
                               << LOG_DESC("condition json parse error")
                               << LOG_KV("condition", conditionStr);

        return CODE_PARSE_CONDITION_ERROR;
    }
    else
    {
        auto members = conditionJson.getMemberNames();
        Json::Value OPJson;
        for (auto iter = members.begin(); iter != members.end(); iter++)
        {
            if (!isHashField(*iter))
            {
                continue;
            }
            OPJson = conditionJson[*iter];
            auto op = OPJson.getMemberNames();
            for (auto it = op.begin(); it != op.end(); it++)
            {
                if (*it == "eq")
                {
                    condition->EQ(*iter, OPJson[*it].asString());
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::EQ);
                }
                else if (*it == "ne")
                {
                    condition->NE(*iter, OPJson[*it].asString());
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::NE);
                }
                else if (*it == "gt")
                {
                    condition->GT(*iter, OPJson[*it].asString());
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::GT);
                }
                else if (*it == "ge")
                {
                    condition->GE(*iter, OPJson[*it].asString());
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::GE);
                }
                else if (*it == "lt")
                {
                    condition->LT(*iter, OPJson[*it].asString());
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::LT);
                }
                else if (*it == "le")
                {
                    condition->LE(*iter, OPJson[*it].asString());
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::LE);
                }
                else if (*it == "limit")
                {
                    std::string offsetCount = OPJson[*it].asString();
                    std::vector<std::string> offsetCountList;
                    boost::split(offsetCountList, offsetCount, boost::is_any_of(","));
                    int offset = boost::lexical_cast<int>(offsetCountList[0]);
                    int count = boost::lexical_cast<int>(offsetCountList[1]);
                    condition->limit(offset, count);
                    _execResult->gasPricer()->appendOperation(InterfaceOpcode::Limit);
                }
                else
                {
                    PRECOMPILED_LOG(ERROR)
                        << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("condition operation undefined")
                        << LOG_KV("operation", *it);

                    return CODE_CONDITION_OPERATION_UNDEFINED;
                }
            }
        }
    }

    return CODE_SUCCESS;
}

int CRUDPrecompiled::parseEntry(const std::string& entryStr, Entry::Ptr& entry)
{
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("table records")
                           << LOG_KV("entryStr", entryStr);
    Json::Value entryJson;
    Json::Reader reader;
    if (!reader.parse(entryStr, entryJson))
    {
        PRECOMPILED_LOG(ERROR) << LOG_BADGE("CRUDPrecompiled") << LOG_DESC("entry json parse error")
                               << LOG_KV("entry", entryStr);

        return CODE_PARSE_ENTRY_ERROR;
    }
    else
    {
        auto memebers = entryJson.getMemberNames();
        for (auto iter = memebers.begin(); iter != memebers.end(); iter++)
        {
            entry->setField(*iter, entryJson[*iter].asString());
        }

        return CODE_SUCCESS;
    }
}
