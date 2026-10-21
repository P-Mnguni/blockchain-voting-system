# Fabric Integration Tests

Tests in this package talk to the live 4-org Fabric dev network in
`deployments/fabric/`.

## Prerequisites

1. The Fabric network is up:
   ```bash
   cd deployments/fabric
   ./scripts/network-up.sh
   ```

2. The ballot chaincode is deployed:
    ```bash
    ./scripts/deploy-chaincode.sh
    ```

## Running

From the repo root:
    ```bash
    FABRIC_INTEGRATION=1 go test ./test/fabric/... -v
    ```

## What's Tested

- **TestHealthCheck** - Gateway → peer → orderer → commit pipeline works
- **TestFullElectionFlow** - Register → start → 10 ballots → freeze → tally; homomorphic sum verified
- **TestDoubleSpendRejected** - Chaincode rejects a second ballot with the same token
- **TestSubmitAfterFreezeRejected** - Chaincode rejects ballots after FreezeElection
- **TestSubmitBeforeVotingRejected** - Chaincode rejects ballots in REGISTRATION phase

Each run uses a unique election ID (test-election-<nanos>), so tests don't collide on ledger state and can be re-run freely.

## Debugging

- connection refused — network isn't up. Run ./scripts/network-up.sh.
- TLS handshake failed — peer cert SAN mismatch. Verify crypto-config.yaml includes localhost and 127.0.0.1 SANs (already committed).
- chaincode not found — chaincode isn't committed. Run ./scripts/deploy-chaincode.sh.
- endorsement policy failure — one of the four peers is unreachable. Check docker ps and peer logs.