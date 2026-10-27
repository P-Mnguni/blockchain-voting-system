# Blockchain Voting System

A blockchain-based national election system designed for multi-party trust,
built on Hyperledger Fabric with end-to-end cryptographic verifiability.

This repository is being built incrementally.
**Milestone 1 (cryptographic foundation) is complete.
**Milestone 2 (Fabric chaincode) is complete.
**Milestone 3 (Identity & Casting Services) is in progress.

---

## Milestone 1 - Cryptographic Foundation

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

graph TD
    chaincode/ballot/ --> main.go
    chaincode/ballot/ --> ballot.go
    chaincode/ballot/ --> models.go
    chaincode/ballot/ --> ballot_test.go
    chaincode/ballot/ --> mock/stub.go

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

## Milestone 3 — Identity & Casting Services 🚧

In progress. Two HTTP services that turn the ballot chaincode into an actual
voting system:

- Identity Service — verifies a voter's eID, issues a blind-signature
credential bound to voter + election + expiry
- Casting Service — verifies the credential, submits the encrypted ballot
to Fabric, returns a non-vote-revealing receipt
- Client-side encryption means the casting service never sees plaintext votes.
- Status: Sub-step 3A (core logic + unit tests) complete. Sub-steps 3B (HTTP
services + integration) and 3C (client library + E2E) pending.

### Roadmap

- **Milestone 1** - Cryptographic foundation (*complete*)
- **Milestone 2** - Fabric chaincode + dev network (*complete*)
- **Milestone 3** - Identity & casting services
- **Milestone 4** - Tally & public verifier portal
- **Milestone 5** - Production readiness (scale, HSM, audit)

### Testing

```bash
# Crypto and package unit tests (no network needed)
go test ./pkg/... ./test/integration/...

# Chaincode unit tests
cd chaincode/ballot && go test ./... && cd ../..

# Fabric integration tests (requires running network)
docker exec -it fabric-test-runner bash
cd /work
FABRIC_INTEGRATION=1 go test ./test/fabric/... -v -count=1
```

### Status 

Pre-production: Do not use for real elections. Independent security audit required
before any deployment

### License

See `LICENSE`

