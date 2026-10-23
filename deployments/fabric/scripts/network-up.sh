#!/usr/bin/env bash
# network-up.sh — bring up the Fabric dev network end to end.
#
# What it does:
#   1. Generate crypto material (cryptogen)
#   2. Generate genesis block + channel tx (configtxgen)
#   3. Start orderer + peers
#   4. Create the channel (via EC peer)
#   5. Join all four peers to the channel
#   6. Update the anchor peer for each org
#
# Re-run safe: if artifacts already exist, it will not regenerate them
# unless you pass --force.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/env.sh"

FORCE=0
for arg in "$@"; do
    [[ "$arg" == "--force" ]] && FORCE=1
done

# --- 1. Generate crypto material ---
if [[ -d "${CRYPTO_CONFIG_DIR}" && ${FORCE} -eq 0 ]]; then
    log "Crypto material already exists; skipping cryptogen (use --force to regenerate)"
else
    log "Generating crypto material with cryptogen"
    rm -rf "${CRYPTO_CONFIG_DIR}"
    docker run --rm \
        -v "${FABRIC_ROOT}:/work" \
        -w /work \
        "hyperledger/fabric-tools:${FABRIC_VERSION}" \
        cryptogen generate --config=crypto-config.yaml --output=crypto-config
fi

# --- 2. Generate channel artifacts ---
mkdir -p "${CHANNEL_ARTIFACTS_DIR}"
if [[ -f "${CHANNEL_ARTIFACTS_DIR}/genesis.block" && ${FORCE} -eq 0 ]]; then
    log "Channel artifacts already exist; skipping configtxgen (use --force to regenerate)"
else
    log "Generating genesis block"
    docker run --rm \
        -v "${FABRIC_ROOT}:/work" \
        -w /work \
        -e FABRIC_CFG_PATH=/work \
        "hyperledger/fabric-tools:${FABRIC_VERSION}" \
        configtxgen -profile ElectionOrdererGenesis \
        -channelID system-channel \
        -outputBlock ./channel-artifacts/genesis.block

    log "Generating channel transaction"
    docker run --rm \
        -v "${FABRIC_ROOT}:/work" \
        -w /work \
        -e FABRIC_CFG_PATH=/work \
        "hyperledger/fabric-tools:${FABRIC_VERSION}" \
        configtxgen -profile ElectionChannel \
        -channelID "${CHANNEL_NAME}" \
        -outputCreateChannelTx ./channel-artifacts/${CHANNEL_NAME}.tx

    # Anchor peer updates for each org
    for pair in \
        "ElectoralCommissionMSP:${CHANNEL_ARTIFACTS_DIR}/ElectoralCommissionMSPanchors.tx" \
        "PartyAMSP:${CHANNEL_ARTIFACTS_DIR}/PartyAMSPanchors.tx" \
        "PartyBMSP:${CHANNEL_ARTIFACTS_DIR}/PartyBMSPanchors.tx" \
        "JudiciaryMSP:${CHANNEL_ARTIFACTS_DIR}/JudiciaryMSPanchors.tx"
    do
        msp="${pair%%:*}"
        out="${pair##*:}"
        log "Generating anchor peer update for ${msp}"
        docker run --rm \
            -v "${FABRIC_ROOT}:/work" \
            -w /work \
            -e FABRIC_CFG_PATH=/work \
            "hyperledger/fabric-tools:${FABRIC_VERSION}" \
            configtxgen -profile ElectionChannel \
            -channelID "${CHANNEL_NAME}" \
            -asOrg "${msp}" \
            -outputAnchorPeersUpdate "${out}"
    done
fi

# --- 3. Start containers ---
log "Starting containers"
docker compose -f "${COMPOSE_FILE}" up -d

log "Waiting for orderer to be ready..."
# wait_for_port localhost "${ORDERER_PORT}" 60
# Wait until the orderer's log shows it has accepted the Raft leader role.
wait_for_orderer_ready() {
    local timeout="${1:-90}"
    local start
    start=$(date +%s)
    while true; do
        if docker logs orderer.electoralcommission.example.com 2>&1 | \
            grep -q "Start accepting requests as Raft leader"; then
            log "Orderer is ready (Raft leader elected)"
            return 0
        fi
        if (( $(date +%s) - start > timeout )); then
            err "Timed out waiting for orderer Raft leader"
            return 1
        fi
        sleep 2
    done
}

for pair in \
    "${PEER_EC_HOST}:${PEER_EC_PORT}" \
    "${PEER_PARTY_A_HOST}:${PEER_PARTY_A_PORT}" \
    "${PEER_PARTY_B_HOST}:${PEER_PARTY_B_PORT}" \
    "${PEER_JUDICIARY_HOST}:${PEER_JUDICIARY_PORT}"
do
    host="${pair%%:*}"; port="${pair##*:}"
    # log "Waiting for ${host}:${port}..."
    # wait_for_port localhost "${port}" 60
    log "Waiting for orderer to be ready (Raft leader election)..."
    wait_for_orderer_ready 90
done

# --- 4. Helper: run a peer CLI command as a given org ---
peer_cli() {
    local msp="$1"; shift
    local msp_dir
    local peer_addr
    case "$msp" in
        ElectoralCommissionMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp
            peer_addr="${PEER_EC_HOST}:${PEER_EC_PORT}"
            ;;
        PartyAMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/partya.example.com/users/Admin@partya.example.com/msp
            peer_addr="${PEER_PARTY_A_HOST}:${PEER_PARTY_A_PORT}"
            ;;
        PartyBMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/partyb.example.com/users/Admin@partyb.example.com/msp
            peer_addr="${PEER_PARTY_B_HOST}:${PEER_PARTY_B_PORT}"
            ;;
        JudiciaryMSP)
            msp_dir=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/judiciary.example.com/users/Admin@judiciary.example.com/msp
            peer_addr="${PEER_JUDICIARY_HOST}:${PEER_JUDICIARY_PORT}"
            ;;
        *) err "Unknown MSP: $msp"; return 1 ;;
    esac
    local tls_root
    tls_root="$(dirname "$(dirname "${msp_dir}")")/peers/$(basename "${peer_addr%%:*}" | sed 's/peer0\.//;s/:.*//')/tls/ca.crt"
    # simpler: derive TLS root from org path
    tls_root="$(echo "${msp_dir}" | sed 's|/users/.*||')/peers/$(echo "${peer_addr%%:*}" | sed 's|peer0\.||;s|:.*||')/tls/ca.crt"
    # Actually, we can point at the org's peer TLS ca directly:
    tls_root="$(echo "${msp_dir}" | sed 's|/users/.*||')"
    # fall through to the peer's own TLS dir
    local peer_name
    peer_name="${peer_addr%%:*}"
    tls_root="$(echo "${msp_dir}" | sed 's|/users/.*||')/peers/${peer_name}/tls/ca.crt"

    docker exec \
        -e CORE_PEER_TLS_ENABLED=true \
        -e CORE_PEER_LOCALMSPID="${msp}" \
        -e CORE_PEER_TLS_ROOTCERT_FILE="${tls_root}" \
        -e CORE_PEER_MSPCONFIGPATH="${msp_dir}" \
        -e CORE_PEER_ADDRESS="${peer_addr}" \
        fabric-cli \
        bash -c "$*"
}

# --- 5. Create channel (EC is the creator) ---
log "Creating channel ${CHANNEL_NAME}"
peer_cli ElectoralCommissionMSP \
    "peer channel create \
        -o ${ORDERER_HOST}:${ORDERER_PORT} \
        --ordererTLSHostnameOverride ${ORDERER_HOST} \
        -c ${CHANNEL_NAME} \
        -f /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts/${CHANNEL_NAME}.tx \
        --outputBlock /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts/${CHANNEL_NAME}.block \
        --tls \
        --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/electoralcommission.example.com/orderers/${ORDERER_HOST}/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem"

# --- 6. Join all peers ---
for msp in ElectoralCommissionMSP PartyAMSP PartyBMSP JudiciaryMSP; do
    log "Joining ${msp} to channel"
    peer_cli "$msp" \
        "peer channel join \
            -b /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts/${CHANNEL_NAME}.block"
done

# --- 6b. Apply anchor peer updates ---
log "Applying anchor peer updates"
for msp in ElectoralCommissionMSP PartyAMSP PartyBMSP JudiciaryMSP; do
    log "  → ${msp}"
    peer_cli "$msp" \
        "peer channel update \
            -o ${ORDERER_HOST}:${ORDERER_PORT} \
            --ordererTLSHostnameOverride ${ORDERER_HOST} \
            -c ${CHANNEL_NAME} \
            -f /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts/${msp}anchors.tx \
            --tls \
            --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/electoralcommission.example.com/orderers/${ORDERER_HOST}/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem"
done

log "Network is up. Channel: ${CHANNEL_NAME}"
log "Next: ./scripts/deploy-chaincode.sh"