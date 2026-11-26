#!/bin/bash
# Demo: 4-org Fabric network in action — multi-org endorsement,
# block propagation, and fault tolerance.
#
# Usage (host): bash docs/demo/network-topology.sh
# Run from the repo root: cd ~/blockchain-voting-system
set -e

CHANNEL=electionchannel
CHAINCODE=ballot
ORDERER=orderer.electoralcommission.example.com:7050
CRYPTO_DIR=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto
CAFILE=$CRYPTO_DIR/ordererOrganizations/electoralcommission.example.com/orderers/orderer.electoralcommission.example.com/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem

# Helper: run peer CLI as a given org
peer_cli() {
    local msp="$1"; shift
    local msp_dir peer_addr tls_cert
    case "$msp" in
        ElectoralCommissionMSP)
            msp_dir=$CRYPTO_DIR/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp
            peer_addr=peer0.electoralcommission.example.com:7051
            tls_cert=$CRYPTO_DIR/peerOrganizations/electoralcommission.example.com/peers/peer0.electoralcommission.example.com/tls/ca.crt
            ;;
        PartyAMSP)
            msp_dir=$CRYPTO_DIR/peerOrganizations/partya.example.com/users/Admin@partya.example.com/msp
            peer_addr=peer0.partya.example.com:8051
            tls_cert=$CRYPTO_DIR/peerOrganizations/partya.example.com/peers/peer0.partya.example.com/tls/ca.crt
            ;;
        PartyBMSP)
            msp_dir=$CRYPTO_DIR/peerOrganizations/partyb.example.com/users/Admin@partyb.example.com/msp
            peer_addr=peer0.partyb.example.com:9051
            tls_cert=$CRYPTO_DIR/peerOrganizations/partyb.example.com/peers/peer0.partyb.example.com/tls/ca.crt
            ;;
        JudiciaryMSP)
            msp_dir=$CRYPTO_DIR/peerOrganizations/judiciary.example.com/users/Admin@judiciary.example.com/msp
            peer_addr=peer0.judiciary.example.com:10051
            tls_cert=$CRYPTO_DIR/peerOrganizations/judiciary.example.com/peers/peer0.judiciary.example.com/tls/ca.crt
            ;;
    esac
    docker exec \
        -e CORE_PEER_TLS_ENABLED=true \
        -e CORE_PEER_LOCALMSPID="$msp" \
        -e CORE_PEER_TLS_ROOTCERT_FILE="$tls_cert" \
        -e CORE_PEER_MSPCONFIGPATH="$msp_dir" \
        -e CORE_PEER_ADDRESS="$peer_addr" \
        fabric-cli bash -c "$*"
}

# Helper: query chain height from a given peer
get_height() {
    local msp="$1"
    peer_cli "$msp" \
        "peer channel getinfo -c $CHANNEL" 2>/dev/null \
        | grep -o '{"height":[0-9]*' \
        | grep -o '[0-9]*$'
}

# Helper: print heights of all four peers
print_heights() {
    local ec a b j
    ec=$(get_height ElectoralCommissionMSP)
    a=$(get_height PartyAMSP)
    b=$(get_height PartyBMSP)
    j=$(get_height JudiciaryMSP)
    printf "         EC: %-3s  PartyA: %-3s  PartyB: %-3s  Judiciary: %-3s\n" "$ec" "$a" "$b" "$j"
}

# Helper: submit a test transaction
submit_test_tx() {
    local election_id="$1"
    peer_cli ElectoralCommissionMSP \
        "peer chaincode invoke \
         -o $ORDERER \
         --ordererTLSHostnameOverride orderer.electoralcommission.example.com \
         -C $CHANNEL -n $CHAINCODE \
         --peerAddresses peer0.electoralcommission.example.com:7051 \
         --tlsRootCertFiles $CRYPTO_DIR/peerOrganizations/electoralcommission.example.com/peers/peer0.electoralcommission.example.com/tls/ca.crt \
         --peerAddresses peer0.partya.example.com:8051 \
         --tlsRootCertFiles $CRYPTO_DIR/peerOrganizations/partya.example.com/peers/peer0.partya.example.com/tls/ca.crt \
         --peerAddresses peer0.partyb.example.com:9051 \
         --tlsRootCertFiles $CRYPTO_DIR/peerOrganizations/partyb.example.com/peers/peer0.partyb.example.com/tls/ca.crt \
         --tls --cafile $CAFILE \
         -c '{\"Args\":[\"RegisterElection\",\"$election_id\",\"Network Demo\",\"[\\\"A\\\",\\\"B\\\"]\",\"demo-pk\"]}'" \
        > /dev/null 2>&1
}

TS=$(date +%s)

clear
echo "=== Four Organizations. One Ledger. No One in Charge. ==="
echo ""
sleep 2

echo "All four peers running the same blockchain at the same height:"
echo ""
print_heights
sleep 4

echo ""
echo "Submitting a transaction..."
echo "  Endorsed by: ElectoralCommission, PartyA, PartyB (3-of-4 majority)"
sleep 1
submit_test_tx "netdemo-a-$TS"
sleep 2
echo "  Block committed to all four peers:"
echo ""
print_heights
sleep 4

echo ""
echo "Now — what if one organization goes offline?"
echo ""
sleep 1
echo "  Stopping peer0.judiciary..."
docker stop peer0.judiciary.example.com > /dev/null 2>&1
sleep 2

echo ""
echo "  Submitting another transaction (3 orgs still active)..."
echo "  Endorsed by: ElectoralCommission, PartyA, PartyB (still 3-of-4 majority)"
sleep 1
submit_test_tx "netdemo-b-$TS"
sleep 2

echo ""
echo "  Block committed to the three active peers:"
echo ""
printf "         EC: %-3s  PartyA: %-3s  PartyB: %-3s  Judiciary: (offline)\n" \
    "$(get_height ElectoralCommissionMSP)" \
    "$(get_height PartyAMSP)" \
    "$(get_height PartyBMSP)"
sleep 3

echo ""
echo "  Restarting peer0.judiciary..."
docker start peer0.judiciary.example.com > /dev/null 2>&1
sleep 12

echo ""
echo "  peer0.judiciary caught up automatically:"
echo ""
print_heights
sleep 4

echo ""
echo "No single org can unilaterally rewrite state or forge endorsements."
echo "A majority is required for every transaction, and the network tolerates"
echo "any single peer going offline."
sleep 5
