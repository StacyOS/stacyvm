# Enterprise Production Signoff Runbook

This runbook covers the evidence-collection steps that operators must run on
their own infrastructure before a StacyVM enterprise multi-worker deployment is
considered signed off for production. All automated CI gates (cluster
conformance, mTLS smoke, runtime certification) must pass first.

## Prerequisites

- StacyVM binary built for the target platform (`make build` or release artifact).
- A Postgres cluster accessible from the control plane.
- At least one worker host with the target runtime installed (Docker, Firecracker, etc.).
- A PKI that can issue TLS certificates (corporate CA, Vault, cert-manager, etc.).

---

## 1. Worker RPC mTLS smoke with deployment-issued certificates

CI validates mTLS with ephemeral certificates. This step proves the same path
works with your actual PKI.

### Prerequisites

Issue three certificates from your deployment CA:

| File | Subject | SAN |
|---|---|---|
| `ca.crt` | Your CA certificate | — |
| `worker.crt` / `worker.key` | Worker RPC server cert | `IP:<worker-ip>` or `DNS:<worker-hostname>` |
| `cp.crt` / `cp.key` | Control-plane client cert | `CN=stacyvm-control-plane` |

### Run

```bash
scripts/smoke-remote-worker.sh ./stacyvm --mtls \
  --ca-cert     /path/to/ca.crt       \
  --server-cert /path/to/worker.crt   \
  --server-key  /path/to/worker.key   \
  --client-cert /path/to/cp.crt       \
  --client-key  /path/to/cp.key
```

### Expected output

```
==> Remote worker smoke PASSED [mTLS]
    mTLS certs used:
      CA:     /path/to/ca.crt
      server: /path/to/worker.crt
      client: /path/to/cp.crt
```

### What it proves

- Control plane authenticates to worker RPC over TLS (mutual auth).
- Worker presents a valid server cert signed by the deployment CA.
- Sandbox spawn, status, exec, and destroy all succeed over the mTLS channel.

### Record

Retain the script output (or a screenshot) as evidence. Reference it in your
change-management ticket using the format:

```
mTLS smoke: PASSED
Binary:     stacyvm <version>
CA:         <issuer CN>
Date:       <YYYY-MM-DD>
Operator:   <name / email>
```

---

## 2. Runtime certification on each worker host

Run this on **every** worker host for **every** runtime it will serve. The
report becomes the durable evidence artifact.

### Docker / gVisor / Kata

```bash
# Host-level checks + StacyVM integration smoke.
scripts/certify-runtime.sh docker \
  --stacyvm-url  https://<control-plane-host>:7423 \
  --stacyvm-api-key "$STACYVM_API_KEY"             \
  --format markdown                                  \
  --output $(hostname)-docker-certification.md

# Review the report.
cat $(hostname)-docker-certification.md
```

For gVisor or Kata, replace `docker` with `gvisor` or `kata`. The script will
check for the runtime in `docker info` and attempt `docker run --runtime=runsc`.

### Firecracker

```bash
export STACYVM_FIRECRACKER_KERNEL=/var/lib/stacyvm/vmlinux.bin

scripts/certify-runtime.sh firecracker \
  --stacyvm-url  https://<control-plane-host>:7423 \
  --stacyvm-api-key "$STACYVM_API_KEY"             \
  --format markdown                                  \
  --output $(hostname)-firecracker-certification.md
```

### Auto-start mode (no external server needed)

If you want to certify the binary itself rather than a running cluster:

```bash
scripts/certify-runtime.sh docker \
  --stacyvm-bin ./stacyvm          \
  --format markdown                 \
  --output $(hostname)-docker-certification.md
```

### What the report covers

| Check | Meaning |
|---|---|
| `docker.cli` | Docker CLI found in PATH |
| `docker.daemon` | Docker daemon reachable |
| `docker.seccomp` | seccomp advertised by docker info |
| `docker.run` | `docker run alpine echo ok` succeeds |
| `stacyvm.ready` | StacyVM API responds to `/api/v1/ready` |
| `stacyvm.provider_health` | Provider health endpoint returns healthy |
| `stacyvm.spawn` | Sandbox spawned via target runtime |
| `stacyvm.exec` | Command executed in sandbox (exit 0) |
| `stacyvm.destroy` | Sandbox destroyed |

### Record

Retain the Markdown report. Every worker host that serves production traffic
must have a report on file before go-live. Reference them in your
change-management ticket:

```
Runtime certification:
  Host:      worker-01.prod.example.com
  Runtime:   docker (gVisor)
  Report:    worker-01-docker-certification.md
  Status:    PASS
  Date:      <YYYY-MM-DD>
  Operator:  <name / email>
```

---

## 3. Postgres migration rehearsal

Run before every binary upgrade that includes a database schema change.

```bash
stacyvm db pg-rehearse --dsn "$STACYVM_DATABASE_DSN"
```

### Expected output

```
connection: OK
schema_migrations: N applied — versions [1 2 3 ... N]
tables: all 16 expected tables present
pg-rehearse: PASS — schema is production-aligned
```

If any tables are missing, run the new binary once with
`STACYVM_DATABASE_DSN` set and the server will apply migrations automatically
on startup. Then re-run `pg-rehearse` to confirm.

---

## 4. OIDC/SSO sign-off

For deployments using `auth.oidc_enabled`, validate the configuration before
exposing to users.

```bash
stacyvm config lint --production --file stacyvm.yaml
```

All `auth.oidc_*` checks must be `[PASS]`. Common failure modes:

| Lint output | Fix |
|---|---|
| `OIDC issuer is not set` | Add `auth.oidc_issuer` pointing to your IdP |
| `no OIDC verification key configured` | Add `auth.oidc_jwks_url` or `auth.oidc_public_key_file` |
| `OIDC audience not set` | Add `auth.oidc_audience` matching your IdP's client audience |
| `no OIDC group-to-role mappings` | Add at least `auth.oidc_admin_groups` |

Then mint a test token from your IdP and verify it is accepted:

```bash
TOKEN="<id_token_from_your_idp>"
curl -H "Authorization: Bearer $TOKEN" https://<control-plane>:7423/api/v1/sandboxes
# Expected: 200 with sandbox list (empty is fine)
```

---

## 5. Worker identity certification

```bash
scripts/certify-worker-identity.sh <worker-id> \
  --format markdown \
  --output worker-identity-certification.md
```

This verifies signed token issuance, inspection, verification, and revocation
without writing token values to the report. Retain the report alongside the
runtime certification.

---

## Signoff checklist

Copy this into your change-management ticket before go-live:

```
[ ] stacyvm config lint --production passes with no FAILs
[ ] stacyvm upgrade rehearse passes (binary + config + database)
[ ] stacyvm db pg-rehearse passes (if Postgres)
[ ] Worker RPC mTLS smoke with deployment-issued certs: PASSED
[ ] Runtime certification report on file for every worker host
[ ] Worker identity certification report on file for every worker ID
[ ] OIDC test token accepted by the production control plane
[ ] stacyvm doctor --production passes on the control-plane host
[ ] stacyvm support bundle generates without token/key leakage
```
