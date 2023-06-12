/**
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
 * @brief tars implementation for Transaction
 * @file TransactionImpl.cpp
 * @author: ancelmo
 * @date 2021-04-20
 */
#include "TransactionImpl.h"

using namespace bcostars;
using namespace bcostars::protocol;

void TransactionImpl::decode(bcos::bytesConstRef _txData)
{
    m_buffer.assign(_txData.begin(), _txData.end());

    tars::TarsInputStream<tars::BufferReader> input;
    input.setBuffer((const char*)m_buffer.data(), m_buffer.size());

    m_inner()->readFrom(input);
}

bcos::bytesConstRef TransactionImpl::encode(bool _onlyHashFields) const
{
    if (m_dataBuffer.empty())
    {
        tars::TarsOutputStream<bcostars::protocol::BufferWriterByteVector> output;
        m_inner()->data.writeTo(output);
        output.getByteBuffer().swap(m_dataBuffer);
    }

    if (_onlyHashFields)
    {
        return bcos::ref(m_dataBuffer);
    }
    else
    {
        if (m_buffer.empty())
        {
            tars::TarsOutputStream<bcostars::protocol::BufferWriterByteVector> output;

            auto hash = m_cryptoSuite->hash(m_dataBuffer);
            m_inner()->dataHash.assign(hash.begin(), hash.end());
            m_inner()->writeTo(output);
            output.getByteBuffer().swap(m_buffer);
        }
        return bcos::ref(m_buffer);
    }
}

bcos::crypto::HashType TransactionImpl::hash() const
{
    if (m_inner()->dataHash.empty())
    {
        auto buffer = encode(true);
        auto hash = m_cryptoSuite->hash(buffer);
        m_inner()->dataHash.assign(hash.begin(), hash.end());
    }

    return *(reinterpret_cast<bcos::crypto::HashType*>(m_inner()->dataHash.data()));
}

bcos::u256 TransactionImpl::nonce() const
{
    if (!m_inner()->data.nonce.empty())
    {
        m_nonce = boost::lexical_cast<bcos::u256>(m_inner()->data.nonce);
    }
    return m_nonce;
}

bcos::bytesConstRef TransactionImpl::input() const
{
    return bcos::bytesConstRef(reinterpret_cast<const bcos::byte*>(m_inner()->data.input.data()),
        m_inner()->data.input.size());
}