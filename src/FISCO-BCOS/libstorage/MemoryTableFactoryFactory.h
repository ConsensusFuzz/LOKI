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
/** @file MemoryTableFactoryFactory.h
 *  @author ancelmo
 *  @date 20190328
 */

#pragma once

#include "MemoryTableFactory.h"
#include "Storage.h"
#include "Table.h"
#include <libdevcore/FixedHash.h>

namespace dev
{
namespace storage
{
class MemoryTableFactoryFactory : public TableFactoryFactory
{
public:
    TableFactory::Ptr newTableFactory(dev::h256 const& hash, int64_t number) override
    {
        MemoryTableFactory::Ptr tableFactory = std::make_shared<MemoryTableFactory>();
        tableFactory->setStateStorage(m_storage);
        tableFactory->setBlockHash(hash);
        tableFactory->setBlockNum(number);

        return tableFactory;
    }

    void setStorage(dev::storage::Storage::Ptr storage) { m_storage = storage; }

private:
    dev::storage::Storage::Ptr m_storage;
};

}  // namespace storage

}  // namespace dev
