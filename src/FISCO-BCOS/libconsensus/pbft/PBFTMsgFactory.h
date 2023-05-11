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
 * (c) 2016-2019 fisco-dev contributors.
 */

/**
 * @file: PBFTMsgFactory.h
 * @author: yujiechen
 *
 * @date: 2019-10-22
 *
 */
#pragma once
#include "Common.h"

namespace dev
{
namespace consensus
{
class PBFTMsgFactory
{
public:
    using Ptr = std::shared_ptr<PBFTMsgFactory>;
    PBFTMsgFactory() = default;
    virtual ~PBFTMsgFactory() {}
    virtual PBFTMsgPacket::Ptr createPBFTMsgPacket() { return std::make_shared<PBFTMsgPacket>(); }
};

// create ttl-optimized pbftMsgPacket
class OPBFTMsgFactory : public PBFTMsgFactory
{
public:
    using Ptr = std::shared_ptr<OPBFTMsgFactory>;
    OPBFTMsgFactory() = default;
    virtual ~OPBFTMsgFactory() {}
    PBFTMsgPacket::Ptr createPBFTMsgPacket() override { return std::make_shared<OPBFTMsgPacket>(); }
};
}  // namespace consensus
}  // namespace dev