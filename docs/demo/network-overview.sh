#!/bin/bash
# Demo: the network is real — 4-org Fabric, all peers up, chaincode committed.
#
# Usage (host): bash docs/demo/network-overview.sh
# Run from the repo root: cd ~/blockchain-voting-system
set -e

clear
echo "=== The Network ==="
echo "4-org Hyperledger Fabric 2.5, single dev host."
echo ""
sleep 2

echo "\$ docker ps"
echo ""
docker ps --format 'table {{.Names}}\t{{.Status}}' \
    | grep -E "orderer|peer0|voting-service|tally-service|fabric-cli" \
    | sort
sleep 4

echo ""
echo "  4 peers + 1 orderer + 2 backend services."
echo ""
sleep 2

echo "\$ peer channel list"
echo ""
docker exec \
    -e CORE_PEER_TLS_ENABLED=true \
    -e CORE_PEER_LOCALMSPID=ElectoralCommissionMSP \
    -e CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/electoralcommission.example.com/peers/peer0.electoralcommission.example.com/tls/ca.crt \
    -e CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp \
    -e CORE_PEER_ADDRESS=peer0.electoralcommission.example.com:7051 \
    fabric-cli peer channel list 2>/dev/null
sleep 3

echo ""
echo "\$ peer lifecycle chaincode querycommitted --channelID electionchannel --name ballot"
echo ""
docker exec \
    -e CORE_PEER_TLS_ENABLED=true \
    -e CORE_PEER_LOCALMSPID=ElectoralCommissionMSP \
    -e CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/electoralcommission.example.com/peers/peer0.electoralcommission.example.com/tls/ca.crt \
    -e CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp \
    -e CORE_PEER_ADDRESS=peer0.electoralcommission.example.com:7051 \
    fabric-cli peer lifecycle chaincode querycommitted \
        --channelID electionchannel --name ballot 2>/dev/null
sleep 4

echo ""
echo "Every transaction requires endorsement from a majority of the four orgs."
echo "No single organization can act alone."
sleep 3
