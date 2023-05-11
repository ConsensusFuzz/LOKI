/*
    This file is part of FISCO-BCOS.

    FISCO-BCOS is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    FISCO-BCOS is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with FISCO-BCOS.  If not, see <http://www.gnu.org/licenses/>.
*/
/** @file ContractLifeCyclePrecompiled.cpp
 *  @author chaychen
 *  @date 20190106
 */
#include "ContractLifeCyclePrecompiled.h"
#include "libdevcrypto/CryptoInterface.h"
#include "libprecompiled/EntriesPrecompiled.h"
#include "libprecompiled/TableFactoryPrecompiled.h"
#include "libstoragestate/StorageState.h"
#include <libethcore/ABI.h>
#include <boost/algorithm/string.hpp>
#include <boost/algorithm/string/split.hpp>
#include <boost/lexical_cast.hpp>

using namespace dev;
using namespace dev::blockverifier;
using namespace dev::storage;
using namespace dev::precompiled;

// precompiled contract function
/*
contract ContractLifeCyclePrecompiled {
    function freeze(address addr) public returns(int);
    function unfreeze(address addr) public returns(int);
    function grantManager(address contractAddr, address userAddr) public returns(int);
    function revokeManager(address contractAddress, address userAddress) public returns(int);
    function getStatus(address addr) public constant returns(int,string);
    function listManager(address addr) public constant returns(int,address[]);
}
*/

const char* const METHOD_FREEZE_STR = "freeze(address)";
const char* const METHOD_UNFREEZE_STR = "unfreeze(address)";
const char* const METHOD_GRANT_STR = "grantManager(address,address)";
// support revokeManager when supported_version >= v2.7.0
const char* const METHOD_REVOKE_STR = "revokeManager(address,address)";
const char* const METHOD_QUERY_STR = "getStatus(address)";
const char* const METHOD_QUERY_AUTHORITY = "listManager(address)";

// contract state
// available,frozen

// state-transition matrix
/*
            available frozen
   freeze   frozen    ×
   unfreeze ×         available
*/

ContractLifeCyclePrecompiled::ContractLifeCyclePrecompiled()
{
    name2Selector[METHOD_FREEZE_STR] = getFuncSelector(METHOD_FREEZE_STR);
    name2Selector[METHOD_UNFREEZE_STR] = getFuncSelector(METHOD_UNFREEZE_STR);
    name2Selector[METHOD_GRANT_STR] = getFuncSelector(METHOD_GRANT_STR);
    name2Selector[METHOD_REVOKE_STR] = getFuncSelector(METHOD_REVOKE_STR);
    name2Selector[METHOD_QUERY_STR] = getFuncSelector(METHOD_QUERY_STR);
    name2Selector[METHOD_QUERY_AUTHORITY] = getFuncSelector(METHOD_QUERY_AUTHORITY);
}

bool ContractLifeCyclePrecompiled::checkPermission(
    ExecutiveContext::Ptr context, std::string const& tableName, Address const& origin)
{
    Table::Ptr table = openTable(context, tableName);
    if (table)
    {
        auto entries = table->select(storagestate::ACCOUNT_AUTHORITY, table->newCondition());
        if (entries->size() > 0)
        {
            // the key of authority would be set when support version >= 2.3.0
            for (size_t i = 0; i < entries->size(); i++)
            {
                std::string authority = entries->get(i)->getField(storagestate::STORAGE_VALUE);
                if (origin.hex() == authority)
                {
                    return true;
                }
            }
        }
    }
    if (g_BCOSConfig.version() >= V2_5_0)
    {
        auto acTable = openTable(context, SYS_ACCESS_TABLE);
        auto condition = acTable->newCondition();
        condition->EQ(SYS_AC_ADDRESS, origin.hexPrefixed());
        auto entries = acTable->select(SYS_ACCESS_TABLE, condition);
        if (entries->size() != 0u)
        {
            PRECOMPILED_LOG(INFO) << LOG_BADGE("ContractLifeCyclePrecompiled")
                                  << LOG_DESC("committee member is permitted to manage contract")
                                  << LOG_KV("origin", origin.hex());
            return true;
        }
    }
    return false;
}


int ContractLifeCyclePrecompiled::updateFrozenStatus(ExecutiveContext::Ptr context,
    std::string const& tableName, std::string const& frozen, Address const& origin)
{
    int result = 0;

    Table::Ptr table = openTable(context, tableName);
    if (table)
    {
        auto entries = table->select(storagestate::ACCOUNT_FROZEN, table->newCondition());
        auto entry = table->newEntry();
        entry->setField(storagestate::STORAGE_VALUE, frozen);
        if (entries->size() != 0u)
        {
            result = table->update(storagestate::ACCOUNT_FROZEN, entry, table->newCondition(),
                std::make_shared<AccessOptions>(origin, false));
        }
        else
        {
            result = table->insert(storagestate::ACCOUNT_FROZEN, entry,
                std::make_shared<AccessOptions>(origin, false));
        }
    }

    return result;
}

void ContractLifeCyclePrecompiled::freeze(ExecutiveContext::Ptr context, bytesConstRef data,
    Address const& origin, PrecompiledExecResult::Ptr _callResult)
{
    dev::eth::ContractABI abi;
    Address contractAddress;
    abi.abiOut(data, contractAddress);
    int result = 0;

    std::string tableName = precompiled::getContractTableName(contractAddress);
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                           << LOG_KV("freeze contract", tableName);

    // existence check > permission check > status check
    ContractStatus status = getContractStatus(context, tableName);
    if (ContractStatus::AddressNonExistent == status)
    {
        result = CODE_INVALID_TABLE_NOT_EXIST;
    }
    else if (ContractStatus::NotContractAddress == status)
    {
        result = CODE_INVALID_CONTRACT_ADDRESS;
    }
    else if (!checkPermission(context, tableName, origin))
    {
        result = CODE_INVALID_NO_AUTHORIZED;
        PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                               << LOG_DESC("permission denied");
    }
    else if (ContractStatus::Frozen == status)
    {
        result = CODE_INVALID_CONTRACT_FROZEN;
    }
    else
    {
        result = updateFrozenStatus(context, tableName, STATUS_TRUE, origin);
    }
    getErrorCodeOut(_callResult->mutableExecResult(), result);
}

void ContractLifeCyclePrecompiled::unfreeze(ExecutiveContext::Ptr context, bytesConstRef data,
    Address const& origin, PrecompiledExecResult::Ptr _callResult)
{
    dev::eth::ContractABI abi;
    Address contractAddress;
    abi.abiOut(data, contractAddress);
    int result = 0;


    std::string tableName = precompiled::getContractTableName(contractAddress);
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                           << LOG_KV("unfreeze contract", tableName);

    // existence check > permission check > status check
    ContractStatus status = getContractStatus(context, tableName);
    if (ContractStatus::AddressNonExistent == status)
    {
        result = CODE_INVALID_TABLE_NOT_EXIST;
    }
    else if (ContractStatus::NotContractAddress == status)
    {
        result = CODE_INVALID_CONTRACT_ADDRESS;
    }
    else if (!checkPermission(context, tableName, origin))
    {
        result = CODE_INVALID_NO_AUTHORIZED;
        PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                               << LOG_DESC("permission denied");
    }
    else if (ContractStatus::Available == status)
    {
        result = CODE_INVALID_CONTRACT_AVAILABLE;
    }
    else
    {
        result = updateFrozenStatus(context, tableName, STATUS_FALSE, origin);
    }
    getErrorCodeOut(_callResult->mutableExecResult(), result);
}

bool ContractLifeCyclePrecompiled::checkContractManager(std::string const& _tableName,
    ExecutiveContext::Ptr _context, Address const& _contractAddress, Address const& _userAddress,
    Address const& _origin, int& _result)
{
    bool ret = true;
    // existence check > permission check > status check
    ContractStatus status = getContractStatus(_context, _tableName);
    if (ContractStatus::AddressNonExistent == status)
    {
        _result = CODE_INVALID_TABLE_NOT_EXIST;
        ret = false;
    }
    else if (ContractStatus::NotContractAddress == status)
    {
        _result = CODE_INVALID_CONTRACT_ADDRESS;
        ret = false;
    }
    else if (!checkPermission(_context, _tableName, _origin))
    {
        _result = CODE_INVALID_NO_AUTHORIZED;
        ret = false;
    }
    if (_result != 0)
    {
        PRECOMPILED_LOG(WARNING) << LOG_BADGE(
                                        "ContractLifeCyclePrecompiled checkContractManager failed")
                                 << LOG_KV("retCode", _result) << LOG_KV("table", _tableName)
                                 << LOG_KV("user", _userAddress)
                                 << LOG_KV("contract", _contractAddress);
    }
    return ret;
}

void ContractLifeCyclePrecompiled::grantManager(ExecutiveContext::Ptr context, bytesConstRef data,
    Address const& origin, PrecompiledExecResult::Ptr _callResult)
{
    dev::eth::ContractABI abi;
    Address contractAddress;
    Address userAddress;
    abi.abiOut(data, contractAddress, userAddress);
    std::string tableName = precompiled::getContractTableName(contractAddress);
    int result = 0;
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                           << LOG_DESC("grant authorization") << LOG_KV("table", tableName)
                           << LOG_KV("user", userAddress) << LOG_KV("contract", contractAddress);

    if (!checkContractManager(tableName, context, contractAddress, userAddress, origin, result))
    {
        getErrorCodeOut(_callResult->mutableExecResult(), result);
        return;
    }

    Table::Ptr table = openTable(context, tableName);
    auto condition = table->newCondition();
    condition->EQ(storagestate::STORAGE_VALUE, userAddress.hex());
    auto entries = table->select(storagestate::ACCOUNT_AUTHORITY, condition);
    if (entries->size() > 0u)
    {
        result = CODE_INVALID_CONTRACT_REPEAT_AUTHORIZATION;
    }
    else
    {
        auto entry = table->newEntry();
        entry->setField(storagestate::STORAGE_KEY, storagestate::ACCOUNT_AUTHORITY);
        entry->setField(storagestate::STORAGE_VALUE, userAddress.hex());
        result = table->insert(
            storagestate::ACCOUNT_AUTHORITY, entry, std::make_shared<AccessOptions>(origin, false));
    }
    getErrorCodeOut(_callResult->mutableExecResult(), result);
}

void ContractLifeCyclePrecompiled::revokeManager(
    std::shared_ptr<blockverifier::ExecutiveContext> context, bytesConstRef data,
    Address const& origin, PrecompiledExecResult::Ptr _callResult)
{
    dev::eth::ContractABI abi;
    Address contractAddress;
    Address userAddress;
    abi.abiOut(data, contractAddress, userAddress);
    std::string tableName = precompiled::getContractTableName(contractAddress);
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled") << LOG_DESC("revokeManager")
                           << LOG_KV("table", tableName) << LOG_KV("user", userAddress)
                           << LOG_KV("contract", contractAddress);
    int result = 0;
    if (!checkContractManager(tableName, context, contractAddress, userAddress, origin, result))
    {
        getErrorCodeOut(_callResult->mutableExecResult(), result);
        return;
    }
    Table::Ptr table = openTable(context, tableName);
    auto condition = table->newCondition();
    // check the account size that can freeze/unfreeze the contract
    auto entries = table->select(storagestate::ACCOUNT_AUTHORITY, condition);
    if (entries->size() == 1)
    {
        PRECOMPILED_LOG(WARNING) << LOG_DESC(
                                        "revokeManager: the last contractManager can't be revoked")
                                 << LOG_KV("contractAddress", contractAddress);
        result = CODE_INVALID_REVOKE_LAST_AUTHORIZATION;
        getErrorCodeOut(_callResult->mutableExecResult(), result);
        return;
    }
    condition = table->newCondition();
    condition->EQ(storagestate::STORAGE_VALUE, userAddress.hex());
    entries = table->select(storagestate::ACCOUNT_AUTHORITY, condition);
    if (entries->size() == 0u)
    {
        PRECOMPILED_LOG(WARNING) << LOG_DESC("revokeManager: non-exist authorization")
                                 << LOG_KV("contractAddress", contractAddress);
        result = CODE_INVALID_NON_EXIST_AUTHORIZATION;
    }
    else
    {
        PRECOMPILED_LOG(DEBUG) << LOG_DESC("revokeManager, remove contract manager authorization");
        // Note: since the permission has been checked in [checkContractManager],
        //       it's no need to determine the AccessOptions when remove the account
        result = table->remove(storagestate::ACCOUNT_AUTHORITY, condition,
            std::make_shared<AccessOptions>(origin, false));
    }
    getErrorCodeOut(_callResult->mutableExecResult(), result);
}

void ContractLifeCyclePrecompiled::getStatus(
    ExecutiveContext::Ptr context, bytesConstRef data, PrecompiledExecResult::Ptr _callResult)
{
    dev::eth::ContractABI abi;

    Address contractAddress;
    abi.abiOut(data, contractAddress);

    std::string tableName = precompiled::getContractTableName(contractAddress);
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                           << LOG_DESC("call query status")
                           << LOG_KV("contract table name", tableName);

    ContractStatus status = getContractStatus(context, tableName);
    _callResult->setExecResult(abi.abiIn("", (u256)0, CONTRACT_STATUS_DESC[status]));
}

void ContractLifeCyclePrecompiled::listManager(
    ExecutiveContext::Ptr context, bytesConstRef data, PrecompiledExecResult::Ptr _callResult)
{
    dev::eth::ContractABI abi;

    Address contractAddress;
    abi.abiOut(data, contractAddress);
    int result = 0;
    std::vector<Address> addrs;

    std::string tableName = precompiled::getContractTableName(contractAddress);
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                           << LOG_DESC("call query authority")
                           << LOG_KV("contract table name", tableName);

    ContractStatus status = getContractStatus(context, tableName);
    if (ContractStatus::AddressNonExistent == status)
    {
        result = CODE_INVALID_TABLE_NOT_EXIST;
    }
    else if (ContractStatus::NotContractAddress == status)
    {
        result = CODE_INVALID_CONTRACT_ADDRESS;
    }
    else
    {
        Table::Ptr table = openTable(context, tableName);
        if (table)
        {
            auto entries = table->select(storagestate::ACCOUNT_AUTHORITY, table->newCondition());
            if (entries->size() > 0)
            {
                for (size_t i = 0; i < entries->size(); i++)
                {
                    auto authority = entries->get(i)->getField(storagestate::STORAGE_VALUE);
                    addrs.push_back(dev::eth::toAddress(authority));
                }
            }
            else
            {
                PRECOMPILED_LOG(ERROR) << LOG_BADGE("ContractLifeCyclePrecompiled")
                                       << LOG_DESC("no authority is recorded");
            }
        }
    }
    _callResult->setExecResult(abi.abiIn("", (u256)result, addrs));
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled")
                           << LOG_DESC("call query authority result")
                           << LOG_KV("out", dev::toHex(_callResult->execResult()));
}

PrecompiledExecResult::Ptr ContractLifeCyclePrecompiled::call(
    ExecutiveContext::Ptr context, bytesConstRef param, Address const& origin, Address const&)
{
    PRECOMPILED_LOG(DEBUG) << LOG_BADGE("ContractLifeCyclePrecompiled");

    // parse function name
    uint32_t func = getParamFunc(param);
    bytesConstRef data = getParamData(param);

    auto callResult = m_precompiledExecResultFactory->createPrecompiledResult();

    if (func == name2Selector[METHOD_FREEZE_STR])
    {
        freeze(context, data, origin, callResult);
    }
    else if (func == name2Selector[METHOD_UNFREEZE_STR])
    {
        unfreeze(context, data, origin, callResult);
    }
    else if (func == name2Selector[METHOD_GRANT_STR])
    {
        grantManager(context, data, origin, callResult);
    }
    else if (func == name2Selector[METHOD_REVOKE_STR] && g_BCOSConfig.version() >= V2_7_0)
    {
        revokeManager(context, data, origin, callResult);
    }
    else if (func == name2Selector[METHOD_QUERY_STR])
    {
        getStatus(context, data, callResult);
    }
    else if (func == name2Selector[METHOD_QUERY_AUTHORITY])
    {
        listManager(context, data, callResult);
    }
    else
    {
        PRECOMPILED_LOG(ERROR) << LOG_BADGE("ContractLifeCyclePrecompiled")
                               << LOG_DESC("call undefined function") << LOG_KV("func", func);
    }

    return callResult;
}
