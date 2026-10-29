# Service Integration Tests

Exercise the voting service's two HTTP endpoints:
- **identity** at `:8080` — `POST /register`, `GET /healthz`
- **casting** at `:8081` — `POST /cast`, `GET /healthz`

Both run inside the single `voting-service` container.

## Prerequisites

1. Fabric network up:
   ```bash
   cd deployments/fabric && ./scripts/network-up.sh
    ```

2. Chaincode deployed:
    ```bash
    ./scripts/deploy-chaincode.sh
    ```

3. Voting service started:

    ```bash
    docker compose -f docker-compose.yml up -d voting-service
    ```

The tests provision their own election via the Fabric Gateway SDK. No
manual peer chaincode invoke needed. Each test run uses a fresh election ID
(e.g. svc-test-1730300000000000000) so runs don't collide.

## Running

Inside the fabric-test-runner container:

    ```bash
    docker exec -it fabric-test-runner bash
    cd /work

    IDENTITY_URL=http://voting-service:8080 \
    CASTING_URL=http://voting-service:8081 \
    FABRIC_MSP_ID=ElectoralCommissionMSP \
    FABRIC_ENDPOINT=peer0.electoralcommission.example.com:7051 \
    FABRIC_HOSTNAME=peer0.electoralcommission.example.com \
    FABRIC_TLS_CA=/work/deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com/peers/peer0.electoralcommission.example.com/tls/ca.crt \
    FABRIC_CERT=/work/deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp/signcerts/Admin@electoralcommission.example.com-cert.pem \
    FABRIC_KEY=/work/deployments/fabric/crypto-config/peerOrganizations/electoralcommission.example.com/users/Admin@electoralcommission.example.com/msp/keystore/priv_sk \
    FABRIC_CHANNEL=electionchannel \
    FABRIC_CHAINCODE=ballot \
    go test ./test/services/... -v -count=1
    ```

All FABRIC_* variables are required — the tests provision elections via
the Gateway SDK, so they need direct Fabric connectivity.

## Test Coverage

| Test | What it proves |
|------|----------------|
| TestHealthz | Both services respond on /healthz |
| TestRegisterAndCast | Full register → cast flow succeeds end-to-end |
| TestCastRejectsDuplicateCredential | Chaincode + service reject double-spend |
| TestCastRejectsGarbageCredential | Malformed credentials rejected before reaching Fabric |
| TestRegisterRejectsUnknownVoter | eID verification rejects unregistered voters |

## Notes

- The mock eID registry accepts exactly: voter-001, voter-002, voter-003.
- The identity service's in-memory voter store resets on restart. Since each 
test provisions a fresh election, cross-test collisions aren't a problem.

- The provisioned elections stay on the ledger forever. That's fine for dev 
— the ledger grows slowly, and each run gets a unique ID. If you want to wipe them, docker compose down -v and rebuild.

