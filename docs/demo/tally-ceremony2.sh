#!/bin/bash
# Demo: the tally ceremony — threshold decryption with ZK proofs.
#
# Usage (host): bash docs/demo/tally-ceremony.sh <election-id> <container-ceremony-dir>
# Run from the repo root: cd ~/blockchain-voting-system
#
# Example:
#   bash docs/demo/tally-ceremony.sh tally-test-XXX /ceremony/e2e-XXX
set -e

ELECTION_ID="$1"
CEREMONY_DIR="$2"
TALLY_URL="${TALLY_URL:-http://localhost:8082}"

if [ -z "$ELECTION_ID" ] || [ -z "$CEREMONY_DIR" ]; then
  echo "Usage: $0 <election-id> <container-ceremony-dir>"
  exit 1
fi

clear
echo "=== The Tally Ceremony ==="
echo "Encrypted ballots become a verifiable result."
echo ""
sleep 2

echo "Step 1 — Ballots on the ledger (still encrypted):"
echo ""
docker exec fabric-cli peer chaincode query \
    -C electionchannel -n ballot \
    -c "{\"Args\":[\"GetEncryptedTally\",\"$ELECTION_ID\"]}" 2>/dev/null \
    | jq -r '"  ballots: \(.ballotCount)    candidates: \(.candidates | join(", "))"'
sleep 3

echo ""
echo "Step 2 — Run the ceremony: load 3-of-5 shares, decrypt the sum,"
echo "         generate a ZK proof per candidate, publish on-chain."
echo ""
sleep 2

curl -s -X POST "$TALLY_URL/tally/$ELECTION_ID" \
    -H "Content-Type: application/json" \
    -d "{\"ceremonyDir\":\"$CEREMONY_DIR\"}" \
    | jq -r '.candidateResults[] | "  \(.candidate): \(.sum)"'
sleep 3

echo ""
echo "Step 3 — Verify independently: no secret key, public data only."
echo ""
sleep 1

go run ./cmd/verifier tally \
    --election="$ELECTION_ID" \
    --msp-id=ElectoralCommissionMSP \
    --endpoint=localhost:7051 \
    --hostname=peer0.electoralcommission.example.com \
    --tls-ca=deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com/peers/peer0.electoralcommission.example.com/tls/ca.crt \
    --cert=deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp/signcerts/Admin@electoralcommission.example.com-cert.pem \
    --key=deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp/keystore/priv_sk

sleep 4
