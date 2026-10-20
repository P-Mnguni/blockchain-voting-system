#!/usr/bin/env bash
# network-down.sh — stop containers and (optionally) wipe state.
#
# Usage:
#   ./network-down.sh           # stop containers, keep crypto + ledger data
#   ./network-down.sh --clean   # also remove volumes and generated artifacts

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/env.sh"

CLEAN=0
for arg in "$@"; do
    [[ "$arg" == "--clean" ]] && CLEAN=1
done

log "Stopping containers"
docker compose -f "${COMPOSE_FILE}" down --remove-orphans

if [[ ${CLEAN} -eq 1 ]]; then
    log "Removing volumes"
    docker compose -f "${COMPOSE_FILE}" down -v --remove-orphans

    log "Removing generated artifacts"
    rm -rf "${CRYPTO_CONFIG_DIR}" "${CHANNEL_ARTIFACTS_DIR}" "${CHAINCODE_BUILD_DIR}"

    log "Clean teardown complete"
else
    log "Containers stopped. Crypto material and ledger data preserved."
    log "Run with --clean to remove all generated state."
fi