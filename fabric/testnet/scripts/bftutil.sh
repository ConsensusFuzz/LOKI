#
# Copyright IBM Corp All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#

ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

setOrdererGlobals() {
  CORE_PEER_LOCALMSPID="OrdererMSP"
  CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/users/Admin@example.com/msp
}

removeOSN() {
    cat config.json |  jq 'del(.channel_group.groups.Orderer.values.ConsensusType.value.metadata.consenters[4]) ' > modified_config.json
}

addOSN() {
    tlscert=`base64 /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer5.example.com/tls/server.crt | sed ':a;N;$!ba;s/\n//g'`
    ecert=`base64 /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/signcerts/orderer5.example.com-cert.pem | sed ':a;N;$!ba;s/\n//g'`
    cat config.json |  jq '.channel_group.groups.Orderer.values.ConsensusType.value.metadata.consenters += [{"msp_id": "OrdererMSP", "identity": "'$ecert'", "consenter_id": 5, "client_tls_cert": "'$tlscert'", "host": "orderer5.example.com", "port": 7050, "server_tls_cert": "'$tlscert'"}] ' > modified_config.json
}

committeeConfig() {
    set -x
    channel=mychannel

    setOrdererGlobals

    echo "fetching config block"
	  CORE_PEER_LOCALMSPID="OrdererMSP"
	  CORE_PEER_TLS_ROOTCERT_FILE=$ORDERER_CA
	  CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/users/Admin@example.com/msp
    peer channel fetch config configBlock.pb -o orderer.example.com:7050 -c mychannel --tls --cafile $ORDERER_CA
    echo "converting config to JSON"
    configtxlator proto_decode --input configBlock.pb --type common.Block | jq '.data.data[0].payload.data.config' > config.json

    echo "Editing the channel configuration"
    cat config.json |  jq '.channel_group.groups.Orderer.values.ConsensusType.value.metadata.options.committee_config = {"committee_minimum_lifespan":3,"failed_total_nodes_percentage":20,"inverse_failure_chance":10000}' > modified_config.json
    configtxlator proto_encode --input config.json --type common.Config --output config.pb
    configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
    configtxlator compute_update --channel_id mychannel --original config.pb --updated modified_config.pb --output update-config.pb
    configtxlator proto_decode --input update-config.pb --type common.ConfigUpdate | jq . > committee.json
    echo '{"payload":{"header":{"channel_header":{"channel_id":"mychannel", "type":2}},"data":{"config_update":'$(cat committee.json)'}}}' | jq . > committee-update.json
    configtxlator proto_encode --input committee-update.json --type common.Envelope --output commttee-update.pb

    peer channel signconfigtx -f commttee-update.pb
    peer channel update -f commttee-update.pb -c mychannel -o orderer.example.com:7050 --tls --cafile $ORDERER_CA
}


channelConfig() {
    set -x
    channel=$1
    configFunc=$2

    setOrdererGlobals

    echo "fetching config block"
	CORE_PEER_LOCALMSPID="OrdererMSP"
	CORE_PEER_TLS_ROOTCERT_FILE=$ORDERER_CA
	CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/users/Admin@example.com/msp
    peer channel fetch config configBlock.pb -o orderer.example.com:7050 -c ${channel} --tls --cafile $ORDERER_CA
    echo "converting config to JSON"
    configtxlator proto_decode --input configBlock.pb --type common.Block | jq '.data.data[0].payload.data.config' > config.json

    echo "Editing the channel configuration"
    eval $configFunc
    echo "computing the config delta"
    configtxlator proto_encode --input config.json --type common.Config --output config.pb
    configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
    configtxlator compute_update --channel_id ${channel} --original config.pb --updated modified_config.pb --output orderer5AdditionOrRemoval.pb
    configtxlator proto_decode --input orderer5AdditionOrRemoval.pb --type common.ConfigUpdate | jq . > orderer5AdditionOrRemoval.json
    echo '{"payload":{"header":{"channel_header":{"channel_id":"'$channel'", "type":2}},"data":{"config_update":'$(cat orderer5AdditionOrRemoval.json)'}}}' | jq . > orderer5AdditionOrRemovalInEnvelope.json
    configtxlator proto_encode --input orderer5AdditionOrRemovalInEnvelope.json --type common.Envelope --output orderer5AdditionOrRemovalInEnvelope.pb
    peer channel signconfigtx -f orderer5AdditionOrRemovalInEnvelope.pb
    peer channel update -f orderer5AdditionOrRemovalInEnvelope.pb -c ${channel} -o orderer.example.com:7050 --tls --cafile $ORDERER_CA
}