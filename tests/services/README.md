# Service Integration Tests

Exercise `identity-service` (port 8080) and `casting-service` (port 8081),
which both run inside the single `voting-service` container.

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

4. An election registered and in VOTING phase with ID matching $ELECTION_ID
(default: election-2025).