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

#include "Common.h"
#include <libblockverifier/ExecutiveContext.h>
#include <libethcore/ABI.h>
#include <libprecompiled/EntriesPrecompiled.h>
#include <libprecompiled/EntryPrecompiled.h>
#include <libstorage/Table.h>
#include <boost/test/unit_test.hpp>

using namespace dev;
using namespace dev::storage;
using namespace dev::precompiled;
using namespace dev::blockverifier;
using namespace dev::eth;

namespace test_EntriesPrecompiled
{
struct EntriesPrecompiledFixture
{
    EntriesPrecompiledFixture()
    {
        entry = std::make_shared<Entry>();
        entries = std::make_shared<Entries>();
        precompiledContext = std::make_shared<dev::blockverifier::ExecutiveContext>();
        entriesPrecompiled = std::make_shared<dev::precompiled::EntriesPrecompiled>();

        entriesPrecompiled->setEntries(entries);

        auto precompiledGasFactory = std::make_shared<dev::precompiled::PrecompiledGasFactory>(0);
        auto precompiledExecResultFactory = std::make_shared<PrecompiledExecResultFactory>();
        precompiledExecResultFactory->setPrecompiledGasFactory(precompiledGasFactory);
        entriesPrecompiled->setPrecompiledExecResultFactory(precompiledExecResultFactory);
    }
    ~EntriesPrecompiledFixture() {}

    dev::storage::Entry::Ptr entry;
    dev::storage::Entries::Ptr entries;
    dev::blockverifier::ExecutiveContext::Ptr precompiledContext;
    dev::precompiled::EntriesPrecompiled::Ptr entriesPrecompiled;
};

BOOST_FIXTURE_TEST_SUITE(EntriesPrecompiled, EntriesPrecompiledFixture)

BOOST_AUTO_TEST_CASE(testBeforeAndAfterBlock)
{
    BOOST_TEST_TRUE(entriesPrecompiled->toString() == "Entries");
}

BOOST_AUTO_TEST_CASE(testEntries)
{
    entry->setField("key", "value");
    entries->addEntry(entry);
    entriesPrecompiled->setEntries(entries);
    BOOST_TEST_TRUE(entriesPrecompiled->getEntries() == entries);
}

BOOST_AUTO_TEST_CASE(testGet)
{
    entry->setField("key", "hello");
    entries->addEntry(entry);
    u256 num = u256(0);
    ContractABI abi;
    bytes bint = abi.abiIn("get(int256)", num);
    auto callResult = entriesPrecompiled->call(precompiledContext, bytesConstRef(&bint));
    bytes out = callResult->execResult();
    Address address;
    abi.abiOut(bytesConstRef(&out), address);
    auto entryPrecompiled = precompiledContext->getPrecompiled(address);
    // BOOST_TEST_TRUE(entryPrecompiled.get());
}

BOOST_AUTO_TEST_CASE(testSize)
{
    entry->setField("key", "hello");
    entries->addEntry(entry);
    ContractABI abi;
    bytes bint = abi.abiIn("size()");
    auto callResult = entriesPrecompiled->call(precompiledContext, bytesConstRef(&bint));
    bytes out = callResult->execResult();
    u256 num;
    abi.abiOut(bytesConstRef(&out), num);
    BOOST_TEST_TRUE(num == u256(1));
}

BOOST_AUTO_TEST_SUITE_END()

}  // namespace test_EntriesPrecompiled
