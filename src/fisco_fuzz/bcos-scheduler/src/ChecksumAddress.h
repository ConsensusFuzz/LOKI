/*
 *  Copyright (C) 2021 FISCO BCOS.
 *  SPDX-License-Identifier: Apache-2.0
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 * @file ChecksumAddress.h
 * @author: xingqiangbai
 * @date: 2021-07-30
 */

#pragma once

#include "bcos-framework/interfaces/crypto/Hash.h"
#include "bcos-utilities/DataConvertUtility.h"
#include <boost/algorithm/string.hpp>
#include <string>

namespace bcos
{
inline void toChecksumAddress(std::string& _addr, const std::string_view& addressHashHex)
{
    auto convertHexCharToInt = [](char byte) {
        int ret = 0;
        if (byte >= '0' && byte <= '9')
        {
            ret = byte - '0';
        }
        else if (byte >= 'a' && byte <= 'f')
        {
            ret = byte - 'a' + 10;
        }
        else if (byte >= 'A' && byte <= 'F')
        {
            ret = byte - 'A' + 10;
        }
        return ret;
    };
    for (size_t i = 0; i < _addr.size(); ++i)
    {
        if (isdigit(_addr[i]))
        {
            continue;
        }
        if (convertHexCharToInt(addressHashHex[i]) >= 8)
        {
            _addr[i] = toupper(_addr[i]);
        }
    }
}

inline void toChecksumAddress(std::string& _hexAddress, crypto::Hash::Ptr _hashImpl)
{
    boost::algorithm::to_lower(_hexAddress);
    toChecksumAddress(_hexAddress, _hashImpl->hash(_hexAddress).hex());
}

inline std::string toChecksumAddressFromBytes(
    const std::string_view& _AddressBytes, crypto::Hash::Ptr _hashImpl)
{
    auto hexAddress = *toHexString(_AddressBytes);
    toChecksumAddress(hexAddress, _hashImpl);
    return hexAddress;
}

}  // namespace bcos
