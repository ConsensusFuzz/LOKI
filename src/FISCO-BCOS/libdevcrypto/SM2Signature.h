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
/** @file SM2Signature.h
 * @author xingqiangbai
 * @date 2020-04-27
 */

#pragma once

#include "Signature.h"
#include "libdevcore/RLP.h"
#include <vector>

namespace dev
{
namespace crypto
{
struct SM2Signature : public Signature
{
    SM2Signature() = default;

    SM2Signature(u256 const& _r, u256 const& _s, h512 _v) : v(_v)
    {
        r = _r;
        s = _s;
    }

    void encode(RLPStream& _s) const noexcept;
    std::vector<unsigned char> asBytes() const;

    /// @returns true if r,s,v values are valid, otherwise false
    bool isValid() const noexcept;

    h512 v;
};
}  // namespace crypto

class KeyPair;
std::shared_ptr<crypto::Signature> sm2Sign(KeyPair const& _keyPair, const h256& _hash);
bool sm2Verify(h512 const& _pubKey, std::shared_ptr<crypto::Signature> _sig, const h256& _hash);
h512 sm2Recover(std::shared_ptr<crypto::Signature> _sig, const h256& _hash);
std::shared_ptr<crypto::Signature> sm2SignatureFromRLP(RLP const& _rlp, size_t _start);
std::shared_ptr<crypto::Signature> sm2SignatureFromBytes(std::vector<unsigned char> _data);
std::pair<bool, std::vector<unsigned char>> recover(bytesConstRef _in);
}  // namespace dev
