#!/usr/bin/env bash
# Shared environment variables for the Fabric dev network.
# Source this from other scripts: `source "$(dirname "$0")/env.sh"`

set -euo pipefail

# --- Paths ---
# FABRIC_ROOT is the directory containing this script's parent (deployments/fabric)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export FABRIC_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
export REPO_ROOT="$(cd "${FABRIC_ROOT}/../.." && pwd)"

# Generated artifacts (gitignored)
export CRYPTO_CONFIG_DIR="${FABRIC_ROOT}/crypto-config"
export CHANNEL_ARTIFACTS_DIR="${FABRIC_ROOT}/channel-artifacts"
export CHAINCODE_BUILD_DIR="${REPO_ROOT}/chaincode-build"

# --- Docker images (pinned) ---
export FABRIC_VERSION="2.5.15"
export FABRIC_CA_VERSION="1.5.13"

# --- Network identity ---
export CHANNEL_NAME="electionchannel"
export CHAINCODE_NAME="ballot"
export CHAINCODE_VERSION="1.0"
export CHAINCODE_SEQUENCE="1"

# --- Orderer ---
export ORDERER_HOST="orderer.electoralcommission.example.com"
export ORDERER_PORT="7050"

# --- Peers ---
# Each org runs one peer. Ports below match docker-compose.yml.
export PEER_EC_HOST="peer0.electoralcommission.example.com"
export PEER_EC_PORT="7051"
export PEER_PARTY_A_HOST="peer0.partya.example.com"
export PEER_PARTY_A_PORT="8051"
export PEER_PARTY_B_HOST="peer0.partyb.example.com"
export PEER_PARTY_B_PORT="9051"
export PEER_JUDICIARY_HOST="peer0.judiciary.example.com"
export PEER_JUDICIARY_PORT="10051"

# --- MSP IDs ---
export MSP_EC="ElectoralCommissionMSP"
export MSP_PARTY_A="PartyAMSP"
export MSP_PARTY_B="PartyBMSP"
export MSP_JUDICIARY="JudiciaryMSP"

# --- Endorsement policy (4-of-4 for now; 3-of-4 is a one-line change) ---
export ENDORSEMENT_POLICY="AND('${MSP_EC}.peer', '${MSP_PARTY_A}.peer', '${MSP_PARTY_B}.peer', '${MSP_JUDICIARY}.peer')"

# --- Docker Compose file ---
export COMPOSE_FILE="${FABRIC_ROOT}/docker-compose.yml"

# --- Helpers ---
log() { printf "\n\033[1;34m==>\033[0m %s\n" "$*"; }
warn() { printf "\n\033[1;33m!!>\033[0m %s\n" "$*"; }
err() { printf "\n\033[1;31m!!!\033[0m %s\n" "$*" >&2; }

# Wait until a TCP port is accepting connections.
wait_for_port() {
    local host="$1" port="$2" timeout="${3:-30}"
    local start
    start=$(date +%s)
    while ! (echo > "/dev/tcp/${host}/${port}") >/dev/null 2>&1; do
        if (( $(date +%s) - start > timeout )); then
            err "Timed out waiting for ${host}:${port}"
            return 1
        fi
        sleep 1
    done
}

# Wait until a peer's logs show it has joined the channel.
wait_for_channel_join() {
    local container="$1" timeout="${2:-60}"
    local start
    start=$(date +%s)
    while ! docker logs "${container}" 2>&1 | grep -q "Received block"; do
        if (( $(date +%s) - start > timeout )); then
            err "Timed out waiting for ${container} to join channel"
            return 1
        fi
        sleep 1
    done
}