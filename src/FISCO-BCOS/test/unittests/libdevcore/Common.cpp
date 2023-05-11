/**
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
 *
 * @brief: unit test for Common.* of libdevcore
 *
 * @file Common.cpp
 * @author: yujiechen
 * @date 2018-08-24
 */


#include <libdevcore/Common.h>
#include <libdevcore/Exceptions.h>
#include <test/tools/libutils/TestOutputHelper.h>
#include <unistd.h>
#include <boost/test/unit_test.hpp>

using namespace dev;
namespace dev
{
namespace test
{
BOOST_FIXTURE_TEST_SUITE(DevcoreCommonTest, TestOutputHelperFixture)
/// test Arith Calculations
BOOST_AUTO_TEST_CASE(testArithCal)
{
    ///=========test u2s==================
    u256 u_bigint("343894723987432");
    bigint c_end = bigint(1) << 256;
    BOOST_CHECK(u2s(u_bigint) == u_bigint);
    u_bigint = Invalid256;
    BOOST_CHECK(u2s(u_bigint) < s256(0));
    u_bigint = u256("0xa170d8e0ae1b57d7ecc121f6fe5ceb03c1267801ff720edd2f8463e7effac6c6");
    BOOST_CHECK(u2s(u_bigint) < s256(0));
    BOOST_CHECK(u2s(u_bigint) == s256(-(c_end - u_bigint)));
    u_bigint = u256("0x7170d8e0ae1b57d7ecc121f6fe5ceb03c1267801ff720edd2f8463e7effac6c6");
    BOOST_CHECK(u2s(u_bigint) == u_bigint);
    ///=========test s2u==================
    s256 s_bigint("0x7170d8e0ae1b57d7ecc121f6fe5ceb03c1267801ff720edd2f8463e7effac6c6");
    BOOST_CHECK(s2u(s_bigint) == s_bigint);
    s_bigint = s256("0xf170d8e0ae1b57d7ecc121f6fe5ceb03c1267801ff720edd2f8463e7effac6c6");
    BOOST_CHECK(s2u(s_bigint) == u256(c_end + s_bigint));
    ///=========test exp10==================
    BOOST_CHECK(exp10<1>() == u256(10));
    BOOST_CHECK(exp10<9>() == u256(1000000000));
    BOOST_CHECK(exp10<0>() == u256(1));
    ///=========test diff==================
    int a = 10;
    int b = 20;
    BOOST_CHECK(diff(a, b) == diff(b, a));
    BOOST_CHECK(diff(b, a) == 10);
    b = 10;
    BOOST_CHECK(diff(a, b) == diff(b, a));
    BOOST_CHECK(diff(b, a) == 0);
    u256 a_u256("0xf170d8e0ae1b57d7ecc121f6fe5ceb03c1267801ff720edd2f8463e7effac6c6");
    u256 b_u256("0xf170d8e0ae1b57d7ecc121f6fe5ceb03c1267801ff720edd2f8463e7effac6c4");
    BOOST_CHECK(diff(a_u256, b_u256) == diff(b_u256, a_u256));
    BOOST_CHECK(diff(a_u256, b_u256) == u256(2));
}
/// test utcTime
BOOST_AUTO_TEST_CASE(testUtcTime)
{
    uint64_t old_time = utcTime();
    usleep(1000);
    BOOST_CHECK(old_time < utcTime());
}
/// test Timer
BOOST_AUTO_TEST_CASE(testTimer)
{
    Timer timer;
    usleep(10);
    double elapsed = timer.elapsed();
    BOOST_CHECK(elapsed >= 0.00001);
    timer.restart();
}

class FakeCheckInvariants : public HasInvariants
{
public:
    FakeCheckInvariants(bool _hasInvariants = true) : m_hasInvariants(_hasInvariants) {}
    virtual bool invariants() const { return m_hasInvariants; }
    void setHasInvariants(bool _hasInvariants) { m_hasInvariants = _hasInvariants; }
    void check() { DEV_INVARIANT_CHECK_HERE; }

private:
    bool m_hasInvariants;
};
BOOST_AUTO_TEST_SUITE_END()
}  // namespace test
}  // namespace dev
