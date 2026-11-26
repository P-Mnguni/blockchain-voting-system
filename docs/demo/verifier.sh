#!/bin/bash
# Demo: independent verifier — no secret key, anyone can run it.
#
# Usage (host): bash docs/demo/verifier.sh <election-id>
# Run from the repo root: cd ~/blockchain-voting-system
set -e

ELECTION_ID="$1"
if [ -z "$ELECTION_ID" ]; then
  echo "Usage: $0 <election-id>"
  exit 1
fi

CRYPTO=deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com

clear
echo "=== Election Verifier ==="
echo "Stateless. Read-only. No secret key."
echo ""
sleep 2

echo "\$ verifier tally --election=$ELECTION_ID --verbose"
echo ""
sleep 1

go run ./cmd/verifier tally \
    --election="$ELECTION_ID" \
    --verbose \
    --msp-id=ElectoralCommissionMSP \
    --endpoint=localhost:7051 \
    --hostname=peer0.electoralcommission.example.com \
    --tls-ca=${CRYPTO}/peers/peer0.electoralcommission.example.com/tls/ca.crt \
    --cert=${CRYPTO}/users/Admin@electoralcommission.example.com/msp/signcerts/Admin@electoralcommission.example.com-cert.pem \
    --key=${CRYPTO}/users/Admin@electoralcommission.example.com/msp/keystore/priv_sk

sleep 4
echo ""
echo "Anyone with the ledger can reproduce this verification."
sleep 3
