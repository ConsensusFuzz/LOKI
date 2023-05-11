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
/** @file AES.h
 * @author asherli
 * @date 2018
 */

#pragma once

#include "Common.h"

namespace dev
{
namespace crypto
{
std::string aesCBCEncrypt(const std::string& _plainData, const std::string& _key);
std::string aesCBCDecrypt(const std::string& _cypherData, const std::string& _key);
std::string aesCBCEncrypt(
    const std::string& _plainData, const std::string& _key, const std::string& _ivData);
std::string aesCBCDecrypt(
    const std::string& _cypherData, const std::string& _key, const std::string& _ivData);
std::string aesCBCEncrypt(const unsigned char* _plainData, size_t _plainDataSize,
    const unsigned char* _key, size_t _keySize, const unsigned char* _ivData);
std::string aesCBCDecrypt(const unsigned char* _cypherData, size_t _cypherDataSize,
    const unsigned char* _key, size_t _keySize, const unsigned char* _ivData);
}  // namespace crypto
}  // namespace dev
