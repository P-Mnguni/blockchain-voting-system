#!/bin/bash
# Demo: a tampered tally — the verifier catches it.
#
# Usage (host): bash docs/demo/tamper-demo.sh <tampered-election-id>
# Run from the repo root: cd ~/blockchain-voting-system
set -e

ELECTION_ID="$1"
if [ -z "$ELECTION_ID" ]; then
  echo "Usage: $0 <tampered-election-id>"
  exit 1
fi

CRYPTO=deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com

clear
echo "=== Same Verifier, Tampered Result ==="
echo "Someone published a wrong tally on-chain. Can the verifier catch it?"
echo ""
sleep 2

echo "\$ verifier tally --election=$ELECTION_ID --verbose"
echo ""
sleep 1

# The verifier will exit non-zero because verification fails.
set +e
go run ./cmd/verifier tally \
    --election="$ELECTION_ID" \
    --verbose \
    --msp-id=ElectoralCommissionMSP \
    --endpoint=localhost:7051 \
    --hostname=peer0.electoralcommission.example.com \
    --tls-ca=${CRYPTO}/peers/peer0.electoralcommission.example.com/tls/ca.crt \
    --cert=${CRYPTO}/users/Admin@electoralcommission.example.com/msp/signcerts/Admin@electoralcommission.example.com-cert.pem \
    --key=${CRYPTO}/users/Admin@electoralcommission.example.com/msp/keystore/priv_sk
EXIT_CODE=$?
set -e

sleep 4
echo ""
if [ $EXIT_CODE -ne 0 ]; then
  echo "Exit code: $EXIT_CODE  — verification failed, as it should."
else
  echo "Exit code: 0  — BUG: tampered tally should not verify."
fi
echo ""
echo "The ZK proofs make lying detectable. A tampered result cannot pass."
sleep 4
