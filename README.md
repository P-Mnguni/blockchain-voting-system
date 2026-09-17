# Blockchain Voting System

A blockchain-based national election system designed for multi-party trust,
built on Hyperledger Fabric with end-to-end cryptographic verifiability.

This repository is being built incrementally.
**Milestone 1 (cryptographic foundation) is complete.
**Milestone 2 (Fabric chaincode) is next.

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

### Testing

```bash
# Run everything
make test

# Verbose
make test -v

# Coverage report (HTML)
make coverage

# Benchmarks
make bench

# Race detector
make race

# Full quality gate (vet + race)
make quality
```

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

### Roadmap

- **Milestone 1** - Cryptographic foundation (*complete*)
- **Milestone 2** - Fabric chaincode + dev network
- **Milestone 3** - Identity & casting services
- **Milestone 4** - Tally & public verifier portal
- **Milestone 5** - Production readiness (scale, HSM, audit)

### Status 

Pre-production: Do not use for real elections. Independent security audit required
before any deployment

### License

See `LICENSE`

