#!/bin/bash

set -e
LOG_ERROR() {
    local content=${1}
    echo -e "\033[31m ${content}\033[0m"
}

LOG_INFO() {
    local content=${1}
    echo -e "\033[32m ${content}\033[0m"
}

init() {
    cat >ipconf <<EOF
127.0.0.1:2 agencyA 1,2
127.0.0.1:2 agencyB 1
EOF
    bash build_chain.sh -T -f ipconf -e ../bin/fisco-bcos -v "${fisco_version}"
    bash check_node_config.sh -p nodes/127.0.0.1/node0
    cd nodes/127.0.0.1
}

is_raft=0

send_transaction()
{
    LOG_INFO "==============send a transaction"
    if [ $(bash .transTest.sh | grep "result" | wc -l) -ne 1 ]; then
        LOG_ERROR "send transaction failed!"
        exit 1
    fi
    if [ ${is_raft} -eq 0 ];then
        sleep 2
    else
        sleep 10
    fi
    LOG_INFO "==============send a transaction is ok"
}

check_reports()
{
    local num=$1
    local target=$2
    local errorMessage=$3
    local successdMessage=$4
    local total=$(cat node*/log/* | grep Report | grep "num=${num}" | wc -l)
    if [ "${total}" -ne "${target}" ]; then
        LOG_ERROR "${errorMessage} ${total} != ${target}"
        cat node*/log/*
        cat node*/nohup.out
        exit 1
    else
        LOG_INFO "${successdMessage}"
    fi
}

check_sync_consensus()
{
    LOG_INFO "***************check_sync_consensus"
    bash start_all.sh && sleep 2
    bash ../../check_node_config.sh -p node0
    sleep 5
    send_transaction
    LOG_INFO "[round1]==============send a transaction is ok"
    LOG_INFO "[round1]==============check report block"
    check_reports 1 4 "check report block failed!" "[round1]==============check report block is ok"

    LOG_INFO "[round1]==============check sync block"
    bash stop_all.sh && sleep 2
    rm -rf node0/data node*/log
    bash start_all.sh && sleep 10
    check_reports 1 4 "[round1] sync block failed!" "[round1]==============check sync block is ok"

    LOG_INFO "[round2]==============restart all node"
    bash stop_all.sh
    rm -rf node*/log
    bash start_all.sh && sleep 1

    send_transaction
    LOG_INFO "[round2]==============send a transaction is ok"

    LOG_INFO "[round2]==============check report block"
    check_reports 2 4 "[round2] check report block failed!" "[round2]==============check report block is ok"

    LOG_INFO "[round2]==============check sync block"
    bash stop_all.sh
    rm -rf node0/data node*/log
    bash start_all.sh && sleep 12
    check_reports 2 4 "[round2] sync block failed!" "[round2]==============check sync block is ok"

    bash stop_all.sh
}

check_consensus_and_sync()
{
    local sleep_seconds=${1}
    bash start_all.sh && sleep 2
    send_transaction
    check_reports 1 4 "check report block failed!" "==============check report block is ok"
    bash stop_all.sh
    rm -rf node0/data/ node*/log
    rm -rf node1/data/block
    bash start_all.sh && sleep "${sleep_seconds}"
    check_reports 1 4 "sync block failed!" "==============check sync block is ok"
    bash stop_all.sh
}

check_binarylog()
{
    LOG_INFO "***************check_binarylog"
    rm -rf node*/data node*/log
    local sed_cmd="sed -i"
    if [ "$(uname)" == "Darwin" ];then
        sed_cmd="sed -i .bkp"
    fi
    ${sed_cmd} "s/binary_log=false/binary_log=true/" node0/conf/group.1.ini
    ${sed_cmd} "s/binary_log=false/binary_log=true/" node1/conf/group.1.ini
    check_consensus_and_sync 12
}

check_raft()
{
    LOG_INFO "***************check_raft"
    rm -rf node*/log node*/data
    local sed_cmd="sed -i"
    if [ "$(uname)" == "Darwin" ];then
        sed_cmd="sed -i .bkp"
    fi
    ${sed_cmd} "s/consensus_type=pbft/consensus_type=raft/" node*/conf/group.1.genesis
    # test resolve host
    ${sed_cmd} "s/127.0.0.1:/localhost:/g" node*/config.ini
    check_consensus_and_sync 10
}

check_rpbft()
{
    LOG_INFO "***************check_rpbft"
    rm -rf node*/log node*/data
    local sed_cmd="sed -i"
    if [ "$(uname)" == "Darwin" ];then
        sed_cmd="sed -i .bkp"
    fi
    ${sed_cmd} "s/consensus_type=raft/consensus_type=rpbft/" node*/conf/group.1.genesis
    ${sed_cmd} "s/epoch_sealer_num=4/epoch_sealer_num=2/" node*/conf/group.1.genesis
    ${sed_cmd} "s/epoch_block_num=1000/epoch_block_num=2/" node*/conf/group.1.genesis
    ${sed_cmd} "s/supported_version=${fisco_version}/supported_version=2.5.0/g" node*/config.ini
    check_consensus_and_sync 12
}

check_vrf_rpbft()
{
    LOG_INFO "***************check vrf_based_rpbft"
    rm -rf node*/log node*/data
    local sed_cmd="sed -i"
    if [ "$(uname)" == "Darwin" ];then
        sed_cmd="sed -i .bkp"
    fi
    ${sed_cmd} "s/supported_version=2.5.0/supported_version=${fisco_version}/g" node*/config.ini
    check_consensus_and_sync 12
}

check_ipv6_pbft()
{
    local last_consensus_algorithm="$1"
    LOG_INFO "***************check pbft when enable ipv6"
    rm -rf node*/log node*/data
    local sed_cmd="sed -i"
    if [ "$(uname)" == "Darwin" ];then
        sed_cmd="sed -i .bkp"
    fi
    # test ipv6
    ${sed_cmd} "s/127.0.0.1:/\[::1\]:/g" node*/config.ini
    ${sed_cmd} "s/0.0.0.0/::/g" node*/config.ini
    ${sed_cmd} "s/consensus_type=${last_consensus_algorithm}/consensus_type=pbft/" node*/conf/group.1.genesis
    check_consensus_and_sync 12 
}

fisco_version=$(../bin/fisco-bcos -v | grep -o "2\.[0-9]\.[0-9]" | head -n 1)
if [ -z "${fisco_version}" ];then LOG_ERROR "get fisco_version failed" && ../bin/fisco-bcos -v ;fi
init
check_sync_consensus
check_binarylog
is_raft=1
check_raft
is_raft=0
check_rpbft
check_vrf_rpbft
check_ipv6_pbft "rpbft"
