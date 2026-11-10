# Blockchain Voting System

A blockchain-based national election system designed for multi-party trust,
built on Hyperledger Fabric with end-to-end cryptographic verifiability.

This repository is being built incrementally.
**Milestone 1 (cryptographic foundation) is complete.
**Milestone 2 (Fabric chaincode) is complete.
**Milestone 3 (Identity & Casting Services) is complete.
**Milestone 4 (Tally Service & Public Verifier) is in progress.

---
## War Stories

This project hit and fixed 20+ non-obvious issues across the milestones.
The full list is in [deployments/fabric/README.md](deployments/fabric/README.md#gotchas--lessons-learned).

The most interesting:

- **The Silent Upgrade** — a chaincode upgrade appeared to work but kept
  running the old code. Two hours of debugging led to a one-line fix in
  the package ID resolution.
- **The Memory Wall** — chaincode installs kept timing out until we
  realized old dev-peer containers were eating RAM.
- **The Schema Rejection** — a nullable struct field silently broke every
  E2E test with a cryptic validation error.


## Milestone 1 - Cryptographic Foundation ✅

The cryptographic primitives that everything else builds on. All are 
independently testable and have no external dependencies.

### What's included

#### Shamir's Secret Sharing - `pkg/crypto/shamir/`
- Split any secret into `n` shares, reconstruct from any `threshold` of them
- Field arithmetic over GF(256)
- Used for threshold decryption: no single party ever holds the full key

#### Blind Signatures - `pkg/crypto/blind/`
- RSA-based Chaum blind signatures
- Issuer signs a blinded voter ID and never sees the unblinded credential
- Used to issue unlinkable voting credentials

#### Homomorphic Encryption - `pkg/crypto/elgamal/`
- Additive ElGamal encryption
- **Votes are encoded as group elements `g^vote`** (not raw `0`/`1`)
- Individual ballots can be homomorphically summed without decrypting them
- `EncryptVote` / `DecryptVote` are the preferred high-level APIs

#### Zero-Knowledge Proofs - `pkg/crypto/zkproofs/`
- **Chaum-Pedersen proofs** of correct decryption
- Prove `log_g(Y) == log_{C1}(C2/m)` without revealing the private key
- Used for public, trustless tally verification

### Design Notes

**Why encode votes as `g^vote`?**
In ElGamal, the plaintext must be a non-zero group element. If we encrypted 
raw `0`, the ciphertext component `C2 = 0 * y^r = 0` would collapse and homomorphic 
addition would zero out the accumulator. Encoding vote `v` as `g^v mod p` preserves 
the group structure: multiplying ciphertexts yields `g^(v1 + v2)`, and the small 
integer sum is recovered by solving a bounded discrete log (trivial for election-sized sum).

**Why Chaum-Pedersen for proofs?**
It is the minimal, standard proof that a ciphertext decrypts to a claimed plaintext decrypts 
to a claimed plaintext under a claimed public key. It produces a single challenge per proof, 
is non-interactive via Fiat-Shamir, and requires only two group exponentiation checks to verify.

### Layout

```
pkg/crypto/
|-- crypto.go                       # Re-exports the public API
|-- shamir/                         # Threshold secret sharing
|-- blind/                          # Blind signatures
|-- elgamal/                        # Homomorphic encryption
|-- zkproofs/                       # Chaum-Pedersen proofs
test/integration/
|-- crypto_test.go                  # Integration test across all primitives
```

---

## Milestone 2 — Fabric Chaincode & Dev Network ✅

A working 4-organization Hyperledger Fabric 2.5.15 network with a ballot
chaincode that enforces election lifecycle, double-spend prevention, and
homomorphic tallying — all validated end-to-end by integration tests.

### What's Included

#### Ballot Chaincode — `chaincode/ballot/`
Six contract functions implementing the election lifecycle:

| Function | Purpose |
|----------|---------|
| `RegisterElection` | Create an election with candidates and homomorphic public key |
| `StartVoting` | Transition REGISTRATION → VOTING |
| `SubmitBallot` | Cast an encrypted ballot with a spent-token check |
| `GetBallot` | Read a single encrypted ballot |
| `HasTokenBeenSpent` | Double-vote prevention check |
| `FreezeElection` | Transition VOTING → FROZEN |
| `GetEncryptedTally` | Homomorphic sum over all ballots |

The chaincode **never decrypts anything** — it only multiplies ciphertexts,
which limits what a compromised or buggy contract can do.

#### 4-Org Dev Network — `deployments/fabric/`

| Org | Role | Peer |
|-----|------|------|
| Electoral Commission | Chain admin, orderer | `peer0.electoralcommission.example.com:7051` |
| Party A | Observer | `peer0.partya.example.com:8051` |
| Party B | Observer | `peer0.partyb.example.com:9051` |
| Judiciary | Independent audit | `peer0.judiciary.example.com:10051` |

- **Channel:** `electionchannel`
- **Endorsement policy:** MAJORITY (3-of-4 orgs)
- **Consensus:** Raft (single node, dev only)
- **Anchor peers:** configured via `configtxlator` config updates (see gotchas)

#### Integration Tests — `test/fabric/`
Five tests exercising the chaincode against the live network via the
Fabric Gateway SDK:

| Test | Proves |
|------|--------|
| `TestHealthCheck` | Gateway → peer → orderer → commit pipeline |
| `TestFullElectionFlow` | Register → start → 10 ballots → freeze → tally |
| `TestDoubleSpendRejected` | Chaincode rejects reused tokens |
| `TestSubmitAfterFreezeRejected` | Chaincode rejects post-freeze ballots |
| `TestSubmitBeforeVotingRejected` | Chaincode rejects ballots in REGISTRATION |

### Running Milestone 2

```bash
# Bring up the network (generates crypto, creates channel, joins peers)
cd deployments/fabric
./scripts/network-up.sh

# Deploy the chaincode (package, install, approve, commit)
./scripts/deploy-chaincode.sh

# Run the integration tests (from inside the test-runner container)
docker exec -it fabric-test-runner bash
cd /work
FABRIC_INTEGRATION=1 go test ./test/fabric/... -v -count=1
```

### Gotchas

See deployments/fabric/README.md for a "Gotchas & Lessons Learned" section
covering the non-obvious issues we hit during Milestone 2:

- configtxgen -outputAnchorPeersUpdate is deprecated and unusable for
existing channels — use configtxlator instead
- extra_hosts hardcodes IPs that change on network recreation — rely on 
Docker's embedded DNS
- Fabric ≤2.5.9 has a broken Docker client with Docker Engine 29 — use 2.5.15+
- Orderer health checks must match container capabilities (no wget/curl)
- grpc.NewClient needs an explicit resolver scheme (passthrough:///)

## Layout

chaincode/ballot/                # Chaincode (own Go module)

├── main.go

├── ballot.go

├── models.go

├── ballot_test.go              # 13 unit tests via mock stub

└── mock/stub.go

deployments/fabric/              # Dev network

├── docker-compose.yml           # 1 orderer, 4 peers, CLI, test-runner

├── configtx.yaml

├── crypto-config.yaml

├── scripts/

│   ├── env.sh

│   ├── network-up.sh

│   ├── network-down.sh

│   └── deploy-chaincode.sh

└── README.md                    # Includes gotchas learned during Milestone 2

test/fabric/                     # Gateway SDK integration tests

├── gateway_test.go

└── helpers.go

## Milestone 3 — Identity & Casting Services ✅

Two HTTP services (running in a single binary) plus a client library that
turn the ballot chaincode into an actual voting system.

### What's Included

#### Voting Service — `cmd/voting-service/`
Single Go binary exposing two HTTP APIs on different ports:

| Port | Endpoint | Purpose |
|------|----------|---------|
| 8080 | `POST /register` | Verify voter, issue a blind-signed credential |
| 8080 | `GET /healthz` | Liveness |
| 8081 | `POST /cast` | Accept encrypted ballot + credential, submit to Fabric |
| 8081 | `GET /healthz` | Liveness |

The two services share one issuer key in memory. Milestone 4 splits them
into independent processes with a proper key exchange.

#### Identity Service — `pkg/identity/`
- `Service.Register()` — verifies eID, issues a **self-verifying credential**
  bound to `(voterID, electionID, expiry)`
- `Credential` — encodes `base64(voterHash || electionHash || expiry) + "." +
  hex(signature)`. No shared state needed by the casting service: it verifies
  the blind signature against the issuer's public key.
- `EIDProvider` — pluggable interface; `MockEID` accepts voter-001/002/003
- `VoterStore` — interface; in-memory impl for Milestone 3

#### Casting Service — `pkg/casting/`
- `Service.Cast()` — verifies the credential signature, checks election
  binding + expiry, queries the chaincode for double-spend, submits the
  encrypted ballot
- **Never sees plaintext votes** — the client encrypts before submission

#### Client Library — `pkg/client/`
- `Client.Register()` — HTTP POST to identity service
- `Client.Cast(vote int64)` — **encrypts locally** with the election's
  homomorphic public key, then POSTs the ciphertext
- `SerializePublicKey` / `DeserializePublicKey` — JSON round-trip for the
  ElGamal public key stored in `Election.HomomorphicPubKey`

#### Fabric Client — `pkg/fabricclient/`
Thin wrapper around the Fabric Gateway SDK. Extracted from the Milestone 2
test code so both services and tests use the same client.

### Testing

Two tiers:

- **Unit tests** (`go test ./pkg/...`) — no network required
- **Integration + E2E tests** (`test/services/`) — require the live network

The service tests **provision their own election** via the Fabric Gateway SDK
and generate a real ElGamal keypair. Each test uses a fresh election ID, so
runs are isolated.

### Running

```bash
# Network up
cd deployments/fabric && ./scripts/network-up.sh

# Chaincode deployed (v1.2)
./scripts/deploy-chaincode.sh

# Voting service
docker compose -f docker-compose.yml up -d --build voting-service

# Run tests from inside the test-runner container
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

### Design Notes

*Why one binary for two services?* The identity service signs credentials
and the casting service verifies them. Sharing the issuer key requires either
serialization (brittle) or a shared process (simple). Milestone 4 introduces
a proper key exchange mechanism, and the two services split then.

*Why no shared token store in casting?* The credential is self-verifying
(the blind signature proves the issuer produced it), so the casting service
only needs the issuer's public key. The chaincode's HasTokenBeenSpent
check provides double-spend protection. No coordination between the two
services beyond the public key.

*Why does the client encrypt?* The architecture requires that the server
never sees plaintext votes. The client library encrypts with the election's
homomorphic public key before sending. The casting service only handles
ciphertext.

### Layout

cmd/voting-service/

├── main.go

├── identity_handler.go

└── casting_handler.go

pkg/

├── identity/          # Credential issuance, eID, voter store

├── casting/           # Credential verification, ballot submission

├── client/            # Go client with client-side encryption

├── fabricclient/      # Fabric Gateway wrapper

└── config/            # Env-var config loader

test/services/

├── integration_test.go   # HTTP endpoint tests

├── e2e_test.go           # Full voter journey

├── provision.go          # Test-side election provisioning

└── helpers.go

## Milestone 4 — Tally Service & Public Verifier ✅

The half of the system that makes the votes count: threshold decryption
and public verification. This is where the architecture doc's trust model
becomes real.

### What's Included

#### Threshold Key Ceremony — `pkg/keyceremony/`
- Trusted dealer generates an ElGamal keypair
- Private scalar is split into N shares via Shamir (threshold-of-total)
- Each share is encrypted with AES-GCM (PBKDF2-SHA256-derived key)
- `ceremony.json` records threshold, total, timestamp
- Production replaces the trusted dealer with a real DKG ceremony

#### Chaincode Tally Functions — `chaincode/ballot/` (v1.5)
- `PublishTally`: writes per-candidate results + ZK proofs to the ledger
- `GetTally`: retrieves the published result
- `GetEncryptedTally`: per-candidate homomorphic sum
- `GetAllBallots`: enumerates ballots for verification

#### Tally Service — `pkg/tally/` + `cmd/tally-service/`
Full ceremony orchestration:
1. Fetch encrypted tally from chaincode
2. Load keyholder shares from disk
3. Reconstruct private key from threshold shares
4. Decrypt per-candidate sums
5. Generate Chaum-Pedersen proof per candidate
6. Publish result + proofs via `PublishTally`

HTTP API: `POST /tally/{electionID}`, `GET /tally/{electionID}`

#### Public Verifier — `pkg/verifier/` + `cmd/verifier/`
Stateless, key-free verification:
- Fetches election + tally + encrypted ciphertexts from ledger
- Verifies each ZK proof against the on-chain ciphertext
- Recomputation: fetches all ballots, sums them, compares to on-chain tally
- Ballot lookup: confirms a specific voter receipt is on the ledger

CLI: `verifier tally --election=X`, `verifier ballot --election=X --ballot-id=Y`
Web: `verifier serve --port=8090` (minimal HTML UI)

### Why It Matters

The verifier uses **no secret keys**. Anyone can run it and independently
confirm the election result. That's the trust property:

> No single party can decrypt votes. Anyone can verify the result.

### Testing

```bash
# Ceremony + tally unit tests
go test ./pkg/keyceremony/... ./pkg/tally/... ./pkg/verifier/... -v

# Full E2E (requires network + services)
docker exec -it fabric-test-runner bash
cd /work
IDENTITY_URL=http://voting-service:8080 \
CASTING_URL=http://voting-service:8081 \
TALLY_URL=http://tally-service:8082 \
# ... (other FABRIC_* env vars)
go test ./test/tally/... -v -count=1
```

### Layout
pkg/

├── keyceremony/       # Trusted dealer + share storage

├── tally/             # Ceremony orchestration

└── verifier/          # Public verification logic

cmd/

├── tally-service/     # HTTP wrapper around pkg/tally

└── verifier/          # CLI + web UI

test/tally/            # Full E2E test

### Demo
```
=== Election tally-test-1793980625502831325 ===
Phase: TALLIED
Ballots cast: 15

Per-candidate results:
  ✓ Alice        sum=10
  ✓ Bob          sum=5

Proofs verified: ✓
Ciphertext match: ✓

=== VERIFIED ===
```

### Status 

Pre-production: Do not use for real elections. Independent security audit required
before any deployment

### Roadmap

**Find:**
```markdown
- [x] **Milestone 1** — Cryptographic foundation
- [x] **Milestone 2** — Fabric chaincode + dev network
- [x] **Milestone 3** — Identity & casting services 
- [x] **Milestone 4** — Tally & public verifier portal (in progress)
- [ ] **Milestone 5** — Production readiness (scale, HSM, audit)
```

### License

See `LICENSE`

