#!/usr/bin/env bash
# deploy-chaincode.sh — package, install, approve, and commit the ballot chaincode.
#
# Assumes network-up.sh has already been run.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/env.sh"

CHAINCODE_SRC="${REPO_ROOT}/chaincode/ballot"
CC_PACKAGE="${CHAINCODE_BUILD_DIR}/${CHAINCODE_NAME}.tar.gz"

mkdir -p "${CHAINCODE_BUILD_DIR}"

# --- 1. Package ---
log "Packaging chaincode (Go) from ${CHAINCODE_SRC}"
rm -f "${CC_PACKAGE}"

# The chaincode module is separate; go mod tidy ensures deps are vendored.
( cd "${CHAINCODE_SRC}" && go mod tidy && go build ./... )

docker run --rm \
    -v "${REPO_ROOT}:/work" \
    -w /work/chaincode/ballot \
    "hyperledger/fabric-tools:${FABRIC_VERSION}" \
    peer lifecycle chaincode package "/work/chaincode-build/${CHAINCODE_NAME}.tar.gz" \
        --path . \
        --lang golang \
        --label "${CHAINCODE_NAME}_${CHAINCODE_VERSION}"

# --- 2. Helper to run peer CLI as an org (same logic as network-up.sh) ---
peer_cli() {
    local msp="$1"; shift
    local msp_dir peer_addr peer_name tls_root
    case "$msp" in
        ElectoralCommissionMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp
            peer_addr="${PEER_EC_HOST}:${PEER_EC_PORT}"
            peer_name="peer0.electoralcommission.example.com"
            ;;
        PartyAMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/partya.example.com/users/Admin@partya.example.com/msp
            peer_addr="${PEER_PARTY_A_HOST}:${PEER_PARTY_A_PORT}"
            peer_name="peer0.partya.example.com"
            ;;
        PartyBMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/partyb.example.com/users/Admin@partyb.example.com/msp
            peer_addr="${PEER_PARTY_B_HOST}:${PEER_PARTY_B_PORT}"
            peer_name="peer0.partyb.example.com"
            ;;
        JudiciaryMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/judiciary.example.com/users/Admin@judiciary.example.com/msp
            peer_addr="${PEER_JUDICIARY_HOST}:${PEER_JUDICIARY_PORT}"
            peer_name="peer0.judiciary.example.com"
            ;;
        *) err "Unknown MSP: $msp"; return 1 ;;
    esac

    # derive org directory (strip /users/Admin.../msp)
    local org_dir
    org_dir="$(echo "${msp_dir}" | sed 's|/users/.*||')"
    tls_root="${org_dir}/peers/${peer_name}/tls/ca.crt"

    docker exec \
        -e CORE_PEER_TLS_ENABLED=true \
        -e CORE_PEER_LOCALMSPID="${msp}" \
        -e CORE_PEER_TLS_ROOTCERT_FILE="${tls_root}" \
        -e CORE_PEER_MSPCONFIGPATH="${msp_dir}" \
        -e CORE_PEER_ADDRESS="${peer_addr}" \
        fabric-cli \
        bash -c "$*"
}

# --- 3. Install on every peer ---
log "Installing chaincode on all peers"
for msp in ElectoralCommissionMSP PartyAMSP PartyBMSP JudiciaryMSP; do
    log "  → ${msp}"
    peer_cli "$msp" \
        "peer lifecycle chaincode install /opt/gopath/src/github.com/blockchain-voting-system/chaincode-build/${CHAINCODE_NAME}.tar.gz"
done

# --- 4. Get package ID ---
log "Resolving package ID"
PKG_ID=""
for msp in ElectoralCommissionMSP PartyAMSP PartyBMSP JudiciaryMSP; do
    OUT="$(peer_cli "$msp" "peer lifecycle chaincode queryinstalled" || true)"
    PKG_ID="$(echo "$OUT" | grep -o "Package ID: [^,]*" | head -n1 | sed 's/Package ID: //')"
    if [[ -n "$PKG_ID" ]]; then
        break
    fi
done
if [[ -z "$PKG_ID" ]]; then
    err "Could not determine package ID"
    exit 1
fi
log "Package ID: ${PKG_ID}"

# --- 5. Approve for each org ---
log "Approving chaincode for each org"
for msp in ElectoralCommissionMSP PartyAMSP PartyBMSP JudiciaryMSP; do
    log "  → ${msp}"
    peer_cli "$msp" \
        "peer lifecycle chaincode approveformyorg \
            -o ${ORDERER_HOST}:${ORDERER_PORT} \
            --ordererTLSHostnameOverride ${ORDERER_HOST} \
            --channelID ${CHANNEL_NAME} \
            --name ${CHAINCODE_NAME} \
            --version ${CHAINCODE_VERSION} \
            --package-id ${PKG_ID} \
            --sequence ${CHAINCODE_SEQUENCE} \
            --tls \
            --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/electoralcommission.example.com/orderers/${ORDERER_HOST}/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem"
done

# --- 6. Check commit readiness ---
log "Checking commit readiness"
peer_cli ElectoralCommissionMSP \
    "peer lifecycle chaincode checkcommitreadiness \
        --channelID ${CHANNEL_NAME} \
        --name ${CHAINCODE_NAME} \
        --version ${CHAINCODE_VERSION} \
        --sequence ${CHAINCODE_SEQUENCE} \
        --output json"

# --- 7. Commit ---
log "Committing chaincode to ${CHANNEL_NAME}"
PEER_TLS_ARGS=""
for msp in ElectoralCommissionMSP PartyAMSP PartyBMSP JudiciaryMSP; do
    case "$msp" in
        ElectoralCommissionMSP) host="${PEER_EC_HOST}"; port="${PEER_EC_PORT}"; org_dir="electoralcommission.example.com" ;;
        PartyAMSP)              host="${PEER_PARTY_A_HOST}"; port="${PEER_PARTY_A_PORT}"; org_dir="partya.example.com" ;;
        PartyBMSP)              host="${PEER_PARTY_B_HOST}"; port="${PEER_PARTY_B_PORT}"; org_dir="partyb.example.com" ;;
        JudiciaryMSP)           host="${PEER_JUDICIARY_HOST}"; port="${PEER_JUDICIARY_PORT}"; org_dir="judiciary.example.com" ;;
    esac
    peer_name="${host}"
    PEER_TLS_ARGS="${PEER_TLS_ARGS} --peerAddresses ${host}:${port} --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/${org_dir}/peers/${peer_name}/tls/ca.crt"
done

peer_cli ElectoralCommissionMSP \
    "peer lifecycle chaincode commit \
        -o ${ORDERER_HOST}:${ORDERER_PORT} \
        --ordererTLSHostnameOverride ${ORDERER_HOST} \
        --channelID ${CHANNEL_NAME} \
        --name ${CHAINCODE_NAME} \
        --version ${CHAINCODE_VERSION} \
        --sequence ${CHAINCODE_SEQUENCE} \
        ${PEER_TLS_ARGS} \
        --tls \
        --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/electoralcommission.example.com/orderers/${ORDERER_HOST}/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem"

log "Chaincode ${CHAINCODE_NAME} is deployed and committed on ${CHANNEL_NAME}."