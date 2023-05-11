/*
    This file is part of cpp-ethereum.

    cpp-ethereum is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    cpp-ethereum is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with cpp-ethereum.  If not, see <http://www.gnu.org/licenses/>.
*/
/**
 * @brief define common Exceptions
 * @file Exceptions.h
 * @author Gav Wood <i@gavwood.com>
 * @date 2014
 */

#pragma once

#pragma warning(push)
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wunused-function"
#include <boost/exception/diagnostic_information.hpp>
#pragma warning(pop)
#pragma GCC diagnostic pop
#include "FixedHash.h"
#include <boost/exception/errinfo_api_function.hpp>
#include <boost/exception/exception.hpp>
#include <boost/exception/info.hpp>
#include <boost/exception/info_tuple.hpp>
#include <boost/throw_exception.hpp>
#include <boost/tuple/tuple.hpp>
#include <exception>
#include <string>

namespace dev
{
/**
 * @brief : Base class for all exceptions
 */
struct Exception : virtual std::exception, virtual boost::exception
{
    Exception(std::string _message = std::string()) : m_message(std::move(_message)) {}
    const char* what() const noexcept override
    {
        return m_message.empty() ? std::exception::what() : m_message.c_str();
    }

private:
    std::string m_message;
};

/// construct a new exception class overidding Exception
#define DEV_SIMPLE_EXCEPTION(X)  \
    struct X : virtual Exception \
    {                            \
    }

/// Base class for all RLP exceptions.
struct RLPException : virtual Exception
{
};

/// construct a new exception class overriding RLPException
#define DEV_SIMPLE_EXCEPTION_RLP(X) \
    struct X : virtual RLPException \
    {                               \
    }

/// RLP related Exceptions
DEV_SIMPLE_EXCEPTION_RLP(BadCast);
DEV_SIMPLE_EXCEPTION_RLP(BadRLP);
DEV_SIMPLE_EXCEPTION_RLP(OversizeRLP);
DEV_SIMPLE_EXCEPTION_RLP(UndersizeRLP);

DEV_SIMPLE_EXCEPTION(BadHexCharacter);
DEV_SIMPLE_EXCEPTION(NoNetworking);
DEV_SIMPLE_EXCEPTION(RootNotFound);
DEV_SIMPLE_EXCEPTION(BadRoot);
DEV_SIMPLE_EXCEPTION(FileError);
DEV_SIMPLE_EXCEPTION(Overflow);
DEV_SIMPLE_EXCEPTION(FailedInvariant);

DEV_SIMPLE_EXCEPTION(MissingField);

DEV_SIMPLE_EXCEPTION(InterfaceNotSupported);
DEV_SIMPLE_EXCEPTION(ExternalFunctionFailure);
DEV_SIMPLE_EXCEPTION(InitLedgerConfigFailed);

DEV_SIMPLE_EXCEPTION(StorageError);
DEV_SIMPLE_EXCEPTION(OpenDBFailed);

DEV_SIMPLE_EXCEPTION(EncryptedDB);

DEV_SIMPLE_EXCEPTION(DatabaseError);
DEV_SIMPLE_EXCEPTION(DatabaseNeedRetry);

DEV_SIMPLE_EXCEPTION(WriteDBFailed);
DEV_SIMPLE_EXCEPTION(DecryptFailed);
DEV_SIMPLE_EXCEPTION(EncryptFailed);
DEV_SIMPLE_EXCEPTION(InvalidDiskEncryptionSetting);

DEV_SIMPLE_EXCEPTION(DBNotOpened);
DEV_SIMPLE_EXCEPTION(UnsupportedFeature);

DEV_SIMPLE_EXCEPTION(ForbidNegativeValue);
DEV_SIMPLE_EXCEPTION(InvalidConfiguration);
DEV_SIMPLE_EXCEPTION(InvalidPort);
DEV_SIMPLE_EXCEPTION(InvalidSupportedVersion);
/**
 * @brief : error information to be added to exceptions
 */
using errinfo_invalidSymbol = boost::error_info<struct tag_invalidSymbol, char>;
using errinfo_wrongAddress = boost::error_info<struct tag_address, std::string>;
using errinfo_comment = boost::error_info<struct tag_comment, std::string>;
using errinfo_required = boost::error_info<struct tag_required, bigint>;
using errinfo_got = boost::error_info<struct tag_got, bigint>;
using errinfo_min = boost::error_info<struct tag_min, bigint>;
using errinfo_max = boost::error_info<struct tag_max, bigint>;
using RequirementError = boost::tuple<errinfo_required, errinfo_got>;
using RequirementErrorComment = boost::tuple<errinfo_required, errinfo_got, errinfo_comment>;
using errinfo_hash256 = boost::error_info<struct tag_hash, h256>;
using errinfo_required_h256 = boost::error_info<struct tag_required_h256, h256>;
using errinfo_got_h256 = boost::error_info<struct tag_get_h256, h256>;
using Hash256RequirementError = boost::tuple<errinfo_required_h256, errinfo_got_h256>;
using errinfo_extraData = boost::error_info<struct tag_extraData, bytes>;
using errinfo_externalFunction = boost::errinfo_api_function;
using errinfo_interface = boost::error_info<struct tag_interface, std::string>;
using errinfo_path = boost::error_info<struct tag_path, std::string>;
}  // namespace dev
