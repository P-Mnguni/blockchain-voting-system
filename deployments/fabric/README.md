## Gotchas & Lessons Learned

These are the real problems encountered across Milestones 2–4. Each one cost
hours to diagnose the first time. Documenting them here so the next person
(or the next session) doesn't repeat the journey.

They're grouped by category rather than chronology, so you can find the one
that matches your current symptom.

---

### Network Setup & Configuration

#### 1. `configtxgen -outputAnchorPeersUpdate` is deprecated and unreliable

The `configtx.yaml` field `AnchorPeers:` does **not** end up in the channel
genesis block. It is only used by `configtxgen -outputAnchorPeersUpdate`, which
is deprecated and produces `.tx` files that fail with `BAD_REQUEST` on any
channel that already exists.

**Symptoms:**

Error: channel create configuration tx file not found
open `.../channel-artifacts/ElectoralCommissionMSPanchors.tx: no such file or directory`

Error: got unexpected status: `BAD_REQUEST --` error applying config update
to existing channel 'electionchannel': error validating ReadSet:
proposed update requires that key [Group] /Channel/Application be at
version 0, but it is currently at version 1


**Fix:** Use the `configtxlator` workflow to fetch the current channel config,
add the anchor peer, and submit the update. This is what `network-up.sh` does
via the `apply_anchor_peer` function.

**Verify it worked:**
`docker logs peer0.electoralcommission.example.com 2>&1 | grep -i anchor | tail -5`

Should show `Learning about the configured anchor peers of ...` - **not**
`No configured anchor peers of ...`.

Without anchor peers, gossip cannot discover peers in other organizations,
service discovery sees only the gateway peer, and every endorsement fails with:

no combination of peers can be derived which satisfy the endorsement policy


#### 2. `extra_hosts` is fragile — rely on Docker's embedded DNS

Adding `extra_hosts` entries to `docker-compose.yml` hardcodes container IPs.
Docker reassigns IPs on every `down`/`up` cycle, so the entries go stale
silently and tests fail with `connection refused` against an IP that no
longer exists.

**Symptom:**

`docker inspect peer0.electoralcommission.example.com` ... → 172.18.0.4
`docker exec fabric-test-runner getent hosts peer0`... → 172.18.0.2

The `/etc/hosts` entry shadows Docker DNS, so the container resolves the
wrong IP.

**Fix:** Remove all `extra_hosts` from the compose file. Docker's embedded
resolver at `127.0.0.11` dynamically resolves container names on the same
network — that's all we need.

#### 3. Health checks must match the container's capabilities

The Fabric orderer image is minimal: **no `wget`, no `curl`**. A readiness
probe that runs `wget` inside the orderer container will always fail, even
when the orderer is healthy.

**Fix:** Either poll the orderer's log for `Start accepting requests as Raft
leader`, or run the probe from a container that has HTTP tools (e.g.
`fabric-cli`, which is based on `fabric-tools` and includes `curl`).

#### 4. Fabric 2.5.15 is required for Docker Engine 29

Fabric ≤2.5.9 ships a Docker client that is incompatible with Docker Engine 29.
Peer chaincode installs fail with:

`docker build failed: docker image build failed:`
`write unix @->/var/run/docker.sock: write: broken pipe`


**Fix:** Pin `FABRIC_VERSION="2.5.15"` (or newer) in `scripts/env.sh` and use
`hyperledger/fabric-*-2.5.15` images in `docker-compose.yml`.

#### 5. Docker socket mount path must match `CORE_VM_ENDPOINT`

The peer runs chaincode builds via Docker. The socket must be mounted at the
exact path the peer expects:

```yaml
environment:
  - CORE_VM_ENDPOINT=unix:///var/run/docker.sock
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

If the mount is `/host/var/run/docker.sock` but `CORE_VM_ENDPOINT` is
`unix:///var/run/docker.sock` (or vice versa), chaincode install fails with:

`no such file or directory: /var/run/docker.sock`

or

`container ... is not running`

#### 6. Orderer Raft leader election takes a few seconds

The orderer's port may be open before the Raft leader has been elected. Channel
creation during this window fails with:

`Error: got unexpected status: SERVICE_UNAVAILABLE -- no Raft leader`

**Fix:** Wait for Start accepting requests as Raft leader in the orderer
logs before creating the channel. `network-up.sh` does this via
`wait_for_orderer_ready`.

#### 7. Peer MSP must have admincerts/ populated

When `NodeOUs.Enable: true`, cryptogen creates the `admincerts/` directory
but leaves it empty, expecting NodeOU identification to do the work.
However, some Fabric code paths (ACL checks on lifecycle operations) still
require the admin certificate to be present in the directory.

**Symptom:** `Error: failed to create signed transaction: proposal response was not successful`,
error code 500, msg Failed to authorize invocation due to failed ACL check:
`[expected MSP ID PartyAMSP, received ElectoralCommissionMSP]`

**Fix:** After cryptogen, copy each org's admin cert into both the Admin MSP
and the peer MSP:

```bash
for org in electoralcommission partya partyb judiciary; do
  admin_cert="crypto-config/peerOrganizations/${org}.example.com/users/Admin@${org}.example.com/msp/signcerts/Admin@${org}.example.com-cert.pem"
  mkdir -p "crypto-config/peerOrganizations/${org}.example.com/users/Admin@${org}.example.com/msp/admincerts"
  cp "$admin_cert" "crypto-config/peerOrganizations/${org}.example.com/users/Admin@${org}.example.com/msp/admincerts/"
  mkdir -p "crypto-config/peerOrganizations/${org}.example.com/peers/peer0.${org}.example.com/msp/admincerts"
  cp "$admin_cert" "crypto-config/peerOrganizations/${org}.example.com/peers/peer0.${org}.example.com/msp/admincerts/"
done
```

Then restart the peers. This should be automated in `network-up.sh` after
cryptogen.

### Chaincode Lifecycle & Deployment

#### 8. Always clear `chaincode-build/` before deploying

The deploy script uses `chaincode-build/ballot.tar.gz` if it already exists -
it does not rebuild it on every run. If you change the chaincode source
(or go.mod) and then deploy, the peer installs the stale tarball.

**Symptoms:**

`Failed to normalize chaincode path: 'go list' failed with: ... signal: killed`
(the stale tarball references a Go toolchain the builder doesn't have)

Function X not found in contract (running old code under new label)

Silent schema mismatches when structs changed

**Fix:** Delete the build dir before every deploy:

```bash
sudo rm -rf deployments/fabric/chaincode-build
./scripts/deploy-chaincode.sh
```

This is a manual step (not automated in the script yet). Add it to your
pre-deploy checklist.

#### 9. Chaincode upgrades need memory headroom

Chaincode builds spike to ~1.5 GB per peer, sequentially across four peers.
On a 7.6 GB VM with 4 peers + orderer + services, that's the tightest moment.

**Symptom:**

```text
Error: chaincode install failed with status: 500 -
error in simulation: failed to execute transaction ...:
error sending: timeout expired while executing transaction
```

or

```text
go: downloading go1.27.0 (linux/amd64): signal: killed
```

**Fix before any chaincode upgrade:**

1. Kill old dev-peer containers:

```bash
docker ps -aq --filter "name=dev-peer.*ballot_OLDVERSION" | xargs -r docker rm -f
docker images --filter=reference='*ballot_OLDVERSION*' -q | xargs -r docker rmi -f
```

2. Stop non-essential services:

```bash
docker compose -f docker-compose.yml stop voting-service test-runner tally-service
```

3. Ensure swap is on: `free -h` should show 4+ GB swap
4. Bump `CORE_CHAINCODE_EXECUTETIMEOUT=1800s` in `docker-compose.yml`
5. Deploy, then restart services

#### 10. Resolve the chaincode package ID by label, not by position

When you install v1.0 and v1.1 of a chaincode, peer lifecycle chaincode queryinstalled lists them alphabetically - `ballot_1.0` comes first. If your deploy script grabs the first Package ID: it sees, it will always pick
the older version.

**Symptom:** the upgrade completes, querycommitted shows the new version,
but the running chaincode is the old code (a new function is missing).

**Wrong:**
```bash
PKG_ID="$(peer lifecycle chaincode queryinstalled | grep -o 'Package ID: [^,]*' | head -n1)"
```

**Right:**
```bash
EXPECTED_LABEL="${CHAINCODE_NAME}_${CHAINCODE_VERSION}"
PKG_ID="$(peer lifecycle chaincode queryinstalled | grep -o "Package ID: [^,]*, Label: ${EXPECTED_LABEL}$" | head -n1 | sed 's/Package ID: //;s/, Label: .*//')"
```

This is what `deploy-chaincode.sh` does. Without it, upgrades silently run
the old code.

#### 11. Peer runtime caches old chaincode even after commit

`querycommitted` reads the channel config and correctly reports the new version.
But the peer's runtime may still serve the old chaincode container if it
was loaded before the upgrade.

**Symptom:**
```text
querycommitted → Version: 1.1
peer chaincode query GetElection → "Function GetElection not found"
```

(The function exists in v1.1 but not v1.0 - proving the old code is running.)

**Fix:**

1. Kill old chaincode containers:
```bash
docker ps -aq --filter "name=dev-peer.*ballot_OLDVERSION" | xargs -r docker rm -f
docker images --filter=reference='*ballot_OLDVERSION*' -q | xargs -r docker rmi -f
```

2. Restart the peers so they reload the lifecycle cache:
```bash
docker compose -f docker-compose.yml restart peer0.*
```

3. First invocation will build the new version.

### Chaincode Schema & Contract Design

#### 12. Nullable fields break chaincode schema validation

Fabric's contract API generates a JSON schema for every returned struct. Fields
marked as required by the schema cannot be `nil` in the response, or the
transaction is rejected with:
```text
Error handling success response. Value did not match schema:
    1. return: <field> is required
```

**What happened:** `Election.TallyResult *TallyResult` was added in v1.3, but
when the election wasn't tallied yet, `TallyResult` was `nil`. The `omitempty`
tag doesn't help - Fabric's schema validator ignores it.

**Fix:** Don't put nullable fields in structs that cross the chaincode boundary.
Use separate `GetX` functions for optional data. We removed
`Election.TallyResult` and use `GetTally()` instead.

#### 13. Contract struct field types affect the JSON schema

If a chaincode struct field is `[]byte`, Go's JSON marshaller serializes it
as a byte array (`[72,101,108,108,111,...]`). If clients expect a string
(e.g. for a JSON-encoded ciphertext blob), Fabric's schema validator rejects
the response before it reaches the client:
```text
Error handling success response. Value did not match schema:
    1. return.encryptedVote: Invalid type. Expected: array, given: string
```

**Rule:** use `string` for any chaincode struct field that already holds a
JSON-encoded blob (ciphertexts, receipts, signatures). Only use `[]byte`
when the field is truly binary.

This bit us in Milestone 3: `Ballot.EncryptedVote` was `[]byte`, so
`GetBallot` returned a byte array while clients parsed it as a string.
Changed to `string` in chaincode v1.2, then to `[]string` (multi-candidate)
in v1.5.

#### 14. Chaincode module's go.mod must match the builder's Go version

The chaincode builder runs inside `fabric-ccenv` (which bundles a specific Go
toolchain). If the chaincode's `go.mod` declares a newer Go version than
the builder has, the builder tries to download the newer toolchain - which
often OOM-kills.

**Symptom:**
```text
go: downloading go1.27.0 (linux/amd64): signal: killed
```

**Fix:** The chaincode module (`chaincode/ballot/go.mod`) should declare a Go
version that matches the builder. `fabric-ccenv:2.5` has Go 1.21/1.22. Set
`go 1.22` (or lower) in the chaincode's `go.mod` - the main module can
stay at whatever version your host runs.

**Check the builder's version:**
```bash
docker run --rm --entrypoint sh hyperledger/fabric-ccenv:2.5 -c "go version"
```

#### 15. `approveformyorg` targets the org's own peer only

`approveformyorg` must be submitted by each org's admin to that org's own peer.
Do not add `--peerAddresses` flags - that's for `commit`, which needs 3
peers to satisfy LifecycleEndorsement.

**Symptom if you get this wrong:**
```text
Failed to deserializing proposal creator during channelless check policy with policy [Admins]:
[expected MSP ID PartyAMSP, received ElectoralCommissionMSP]
```

**Correct structure:**

- `approveformyorg`: no `--peerAddresses` (one org, one peer)
- `commit`: 3+ `--peerAddresses` (satisfies MAJORITY policy)
- `approveformyorg` requires the peer's MSP to have the admin cert (gotcha #7)

#### 16. `LifecycleEndorsement` requires MAJORITY of orgs

The default `LifecycleEndorsement` policy is `MAJORITY Endorsement`. With 4
orgs, that's 3-of-4 endorsements required for `commit`. If only one peer
endorses, the transaction is marked invalid.

**Symptom:**
```text
ENDORSEMENT_POLICY_FAILURE -- implicit policy evaluation failed
- 1 sub-policies were satisfied, but this policy requires 3 of the
'Endorsement' sub-policies to be satisfied
```

**Fix:** Submit commit with `--peerAddresses` for at least 3 orgs. This is
what `deploy-chaincode.sh` does via `PEER_TLS_ARGS`.

### Testing & Development Workflow

#### 17. Filesystem permissions matter for crypto operations

Cryptogen and Docker write files as root. Subsequent operations as your
user fail with `Permission denied`.

**Symptom:**
```text
rm: cannot remove '.../crypto-config/...': Permission denied
cp: cannot create regular file '.../admincerts/...': Permission denied
```

**Fix:** After any crypto/config regeneration, take ownership:
```bash
sudo chown -R $USER:$USER crypto-config channel-artifacts chaincode-build
```

#### 18. Ceremony directories are created by root inside containers

The test-runner container runs as root, and the `ceremony/` directory is a
bind mount. Test runs create subdirectories owned by root on the host. This
breaks `go build ./...` (which traverses all directories) with:
```text
pattern ./...: open deployments/fabric/ceremony/...: permission denied
```

**Fixes:**

1. Tests should clean up after themselves via `t.Cleanup`. Already added to
`test/tally/provision.go`.

2. If old directories exist:
```bash
sudo rm -rf deployments/fabric/ceremony/*/
sudo chown $USER:$USER deployments/fabric/ceremony/
```

#### 19. Only test on Linux (native ext4)

`chmod` semantics, file permissions, and case sensitivity differ between
Windows and Linux. Tests that check file modes (e.g.
`TestSaveShareFilePermissions`) pass on Linux but fail on Windows (or on
VirtualBox shared folders mounted with `vboxsf`).

**Rule:** Run `go test ./...` and all integration tests inside the Ubuntu
VM, not on Windows. Editing on Windows is fine; testing must be on Linux.

For tests that verify Unix-specific behavior, guard with `runtime.GOOS`:
```go
if runtime.GOOS != "linux" {
    t.Skipf("permission checks require Linux (running on %s)", runtime.GOOS)
}
```

#### 20. gRPC clients need an explicit resolver scheme

`grpc.NewClient` (gRPC-Go ≥1.63) defaults to the `dns` resolver, which does
not handle bare hostnames reliably.

**Symptoms:**
```text
rpc error: code = Unavailable desc = name resolver error: produced zero addresses
```

or

```text
dial tcp: lookup peer0... on 127.0.0.11:53: no such host
```

**Fix:** Prefix the target with passthrough:/// for Docker-internal
hostnames (which the OS resolves via Docker DNS):
```go
target := "passthrough:///" + cfg.PeerEndpoint
conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
```

#### 21. Commit timeouts need to be generous on a dev VM

With 4 peers + orderer + chaincode containers + services all on one VM, commit
latency can spike past the default 30s. Gateway clients need longer timeouts:
```go
client.WithEvaluateTimeout(30 * time.Second),
client.WithEndorseTimeout(120 * time.Second),
client.WithSubmitTimeout(30 * time.Second),
client.WithCommitStatusTimeout(120 * time.Second),
```

**Symptom if too short:**
```text
commit status error: rpc error: code = DeadlineExceeded desc = context deadline exceeded
```

### Summary Table

| # | Category | Symptom |
|---|----------|---------|
| 1 | Network | `BAD_REQUEST` on anchor peer update, `no combination of peers` on endorse |
| 2 | Network | `connection refused` to stale IP, `getent hosts` returns wrong IP |
| 3 | Network | Orderer readiness probe times out despite healthy orderer |
| 4 | Network | `broken pipe` on chaincode install with Docker Engine 29 |
| 5 | Network | `container is not running` or `no such file` on chaincode install |
| 6 | Network | `SERVICE_UNAVAILABLE -- no Raft leader` on channel create |
| 7 | Network | `failed ACL check: expected MSP ID X, received Y` |
| 8 | Chaincode | `signal: killed` during build, `Function X not found` after upgrade |
| 9 | Chaincode | `timeout expired while executing transaction` on install |
| 10 | Chaincode | Upgrade completes but old code runs |
| 11 | Chaincode | `querycommitted` says v1.1, query says "function not found" |
| 12 | Schema | `return: <field> is required` on transaction response |
| 13 | Schema | `Expected: array, given: string` (or vice versa) |
| 14 | Chaincode | `go: downloading go1.XX: signal: killed` |
| 15 | Lifecycle | `expected MSP ID X, received Y` on approve |
| 16 | Lifecycle | `requires 3 sub-policies` on commit |
| 17 | Filesystem | `Permission denied` on generated files |
| 18 | Filesystem | `go build ./...` fails on ceremony dir |
| 19 | Testing | Permission tests pass on Linux, fail on Windows |
| 20 | gRPC | `name resolver error` or `no such host`
| 21 | gRPC | `DeadlineExceeded` on commit