## Gotchas & Lessons Learned

These are real problems encountered during Milestone 2 development. Each one
cost hours to days to diagnose the first time; noting them here so the next person
(or the next session) doesn't repeat the journey.

### 1. `configtxgen -outputAnchorPeersUpdate` is deprecated and unreliable

The `configtx.yaml` field `AnchorPeers:` does **not** end up in the channel
genesis block. It is only used by `configtxgen -outputAnchorPeersUpdate`, which
is deprecated and produces `.tx` files that fail with `BAD_REQUEST` on any
channel that already exists.

**Symptom:**

```yaml
Error: channel create configuration tx file not found
open .../channel-artifacts/ElectoralCommissionMSPanchors.tx: no such file or directory
or
Error: got unexpected status: BAD_REQUEST -- error applying config update
```

**Fix:** use the `configtxlator` workflow to fetch the current channel config,
add the anchor peer, and submit the update. See the anchor-peers section below
for the exact commands.

**Verify it worked:**

```yaml
docker logs peer0.electoralcommission.example.com 2>&1 | grep -i anchor | tail -5
```

Should show `Learning about the configured anchor peers of ...` — **not**
`No configured anchor peers of ...`.

Without anchor peers, gossip cannot discover peers in other organizations,
service discovery sees only the gateway peer, and every endorsement fails with:
'no combination of peers can be derived which satisfy the endorsement policy'

### 2. `extra_hosts` is fragile — rely on Docker's embedded DNS

Adding `extra_hosts` entries to `docker-compose.yml` hardcodes container IPs.
Docker reassigns IPs on every `down`/`up` cycle, so the entries go stale
silently and tests fail with `connection refused` against an IP that no longer
exists.

**Symptom:**

```yaml
docker inspect peer0.electoralcommission.example.com ... → 172.18.0.4
docker exec fabric-test-runner getent hosts peer0... → 172.18.0.2
```

The `/etc/hosts` entry shadows Docker DNS, so the container resolves the wrong
IP.

**Fix:** remove all `extra_hosts` from the compose file. Docker's embedded
resolver at `127.0.0.11` dynamically resolves container names on the same
network — that's all we need.

### 3. Health checks must match the container's capabilities

The Fabric orderer image is minimal: **no `wget`, no `curl`**. A readiness probe
that runs `wget` inside the orderer container will always fail, even when the
orderer is healthy.

**Fix:** either poll the orderer's log for `Start accepting requests as Raft
leader`, or run the probe from a container that has HTTP tools (e.g.
`fabric-cli`, which is based on `fabric-tools` and includes `curl`).

### 4. Fabric 2.5.15 is required for Docker Engine 29

Fabric ≤2.5.9 ships a Docker client that is incompatible with Docker Engine 29.
Peer chaincode installs fail with:

```yaml
docker build failed: docker image build failed:
write unix @->/var/run/docker.sock: write: broken pipe
```

**Fix:** pin `FABRIC_VERSION="2.5.15"` (or newer) in `scripts/env.sh` and use
`hyperledger/fabric-*-2.5.15` images in `docker-compose.yml`.

### 5. Docker socket mount path must match `CORE_VM_ENDPOINT`

The peer runs chaincode builds via Docker. The socket must be mounted at the
exact path the peer expects:

```yaml
environment:
  - CORE_VM_ENDPOINT=unix:///var/run/docker.sock
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```
if the mount is /host/var/run/docker.sock but CORE_VM_ENDPOINT is
unix:///var/run/docker.sock (or vice versa), chaincode install fails with
no such file or directory.

### 6. Root-owned artifacts break subsequent runs

Docker writes files into bind-mounted directories as root. On the next run,
network-down.sh --clean can't delete them and cryptogen/configtxgen can't
overwrite them.

**Symptom:** 

```yaml
rm: cannot remove '.../crypto-config/...': Permission denied
```

**Fix:** clean with sudo:

```yaml
sudo rm -rf crypto-config channel-artifacts chaincode-build
```

Before re-running network-up.sh.

### 7. Orderer Raft leader election takes a few seconds

The orderer's port may be open before the Raft leader has been elected. Channel
creation during this window fails with:

```yaml
Error: got unexpected status: SERVICE_UNAVAILABLE -- no Raft leader
```

**Fix:** wait for Start accepting requests as Raft leader in the orderer logs
before creating the channel. network-up.sh does this via wait_for_orderer_ready.
 
### 8. go.mod Go version vs test-runner container

The go directive in go.mod declares the minimum Go version required to
build the module. If it's set to a version newer than the test-runner image
(golang:1.27, etc.), the container can't run tests.

**Fix:** keep go.mod at a version that matches the golang:X.Y image tag in
docker-compose.yml. If a dependency requires a newer Go, bump both together.

### 9. grpc.NewClient requires an explicit resolver scheme

grpc.NewClient (gRPC-Go ≥1.63) defaults to the dns resolver, which does
not handle bare hostnames reliably. It fails with either:

```yaml
name resolver error: produced zero addresses
or
dial tcp: lookup peer0... on 127.0.0.11:53: no such host
```

**Fix:** either prefix the target with passthrough:/// (skip resolution) or
dns:/// (explicit DNS). For Docker-internal hostnames with working DNS,
passthrough:/// is the reliable choice:

```go
target := "passthrough:///" + cfg.PeerEndpoint
conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
```

### 10. Fabric Gateway endorsement requires CORE_PEER_GATEWAY_ENABLED=true

The Gateway SDK connects to :7051 on each peer. The peer must have the Gateway
service enabled (default in 2.5.x, but explicitly set it in docker-compose.yml
to avoid surprises):

```yaml
environment:
  - CORE_PEER_GATEWAY_ENABLED=true
```

### 11. Anchor peer update — the working configtxlator workflow

Because configtxgen -outputAnchorPeersUpdate is broken for existing channels,
this is the procedure that actually works for each org (<MSP> = the org you're
setting the anchor peer for, <PEER> = its peer host, <PORT> = its peer port):

```bash
# 1. Fetch current config (as the org whose admin will sign the update)
docker exec \
    -e CORE_PEER_TLS_ENABLED=true \
    -e CORE_PEER_LOCALMSPID=<MSP> \
    -e CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/<ORG>.example.com/peers/<PEER>/tls/ca.crt \
    -e CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/<ORG>.example.com/users/Admin@<ORG>.example.com/msp \
    -e CORE_PEER_ADDRESS=<PEER>:<PORT> \
    fabric-cli peer channel fetch config \
        /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts/config_block.pb \
        -o orderer.electoralcommission.example.com:7050 \
        --ordererTLSHostnameOverride orderer.electoralcommission.example.com \
        -c electionchannel \
        --tls \
        --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/electoralcommission.example.com/orderers/orderer.electoralcommission.example.com/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem

# 2. Decode, inject anchor peer, re-encode
docker exec fabric-cli bash -c '
cd /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
jq .data.data[0].payload.data.config config_block.json > config.json
cp config.json config_copy.json
jq '\''.channel_group.groups.Application.groups.<MSP>.values += {"AnchorPeers":{"mod_policy":"Admins","value":{"anchor_peers":[{"host":"<PEER>","port":<PORT>}]},"version":"0"}}'\'' config_copy.json > modified_config.json
configtxlator proto_encode --input config.json --type common.Config --output config.pb
configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
configtxlator compute_update --channel_id electionchannel --original config.pb --updated modified_config.pb --output config_update.pb
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json
echo "{\"payload\":{\"header\":{\"channel_header\":{\"channel_id\":\"electionchannel\",\"type\":2}},\"data\":{\"config_update\":$(cat config_update.json)}}}" | jq . > config_update_in_envelope.json
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb
'

# 3. Submit the update (as the same org admin)
docker exec \
    -e CORE_PEER_TLS_ENABLED=true \
    -e CORE_PEER_LOCALMSPID=<MSP> \
    -e CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/<ORG>.example.com/peers/<PEER>/tls/ca.crt \
    -e CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/<ORG>.example.com/users/Admin@<ORG>.example.com/msp \
    -e CORE_PEER_ADDRESS=<PEER>:<PORT> \
    fabric-cli peer channel update \
        -o orderer.electoralcommission.example.com:7050 \
        --ordererTLSHostnameOverride orderer.electoralcommission.example.com \
        -c electionchannel \
        -f /opt/gopath/src/github.com/hyperledger/fabric/peer/channel-artifacts/config_update_in_envelope.pb \
        --tls \
        --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/electoralcommission.example.com/orderers/orderer.electoralcommission.example.com/msp/tlscacerts/tlsca.electoralcommission.example.com-cert.pem
```

Repeat for each org. Verify with:

```yaml
docker logs peer0.<org>.example.com 2>&1 | grep -i anchor | tail -5
```

All four orgs must appear as Learning about ... on every peer. Only then
will endorsement succeed.

## Where to Put It

Append this section to the existing `deployments/fabric/README.md`, after the "Troubleshooting" section (or replace that section — this is a superset).

## Also Worth Doing

Add a short "Known limitations" note at the top of `deployments/fabric/README.md` clarifying this is **dev-only**:

```markdown
> **Dev network only.** This configuration is for local development and
> integration testing. It uses single-node Raft, `cryptogen` (not Fabric CA),
> plaintext ledger state, and no HSM. Do not deploy this topology anywhere
> that matters. Production hardening is Milestone 5.
```