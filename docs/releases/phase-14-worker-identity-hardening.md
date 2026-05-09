# Phase 14 Worker Identity Hardening

Phase 14 begins the worker identity hardening lane for production multi-worker StacyVM deployments. The goal of this slice is to move beyond static shared worker credentials while keeping the existing Phase 11-13 worker runtime compatible.

## What Changed

### Signed worker tokens

- Added HMAC-SHA256 signed worker token support.
- Added the `stacyvm-worker-v1.<payload>.<signature>` token format.
- Added signed token claims for:
  - `worker_id`
  - audience through `aud`
  - optional worker `scopes`
  - issued-at time through `iat`
  - expiry time through `exp`
- Enforced signed-token expiry before accepting worker requests.
- Enforced that signed `worker_id` must match the `X-Worker-ID` request header.
- Enforced token audience separation between worker-to-control-plane routes and control-plane-to-worker RPC.
- Filtered signed-token scopes so tokens cannot grant user, API, or admin scopes.
- Added `stacyvm worker token <worker-id>` to issue signed worker tokens from the CLI.

### Configuration

- Added `auth.worker_signing_key`.
- Added `auth.worker_signing_keys` for old verification keys during rotation.
- Kept `auth.worker_token` for shared-token staging compatibility.
- Kept `auth.worker_tokens` for per-worker static token migration paths.
- Updated `stacyvm config lint --production` so a strong `auth.worker_signing_key` satisfies production-aligned worker credential checks.

### Worker runtime

- Added dynamic worker token generation for `stacyvm worker` heartbeat and lease-renewal calls.
- When no static `--worker-token` or `auth.worker_token` is configured, a worker can derive short-lived signed control-plane tokens from `auth.worker_signing_key`.
- Worker RPC servers now accept signed control-plane-to-worker tokens.
- Control planes can mint short-lived worker RPC tokens from `auth.worker_signing_key` when no shared `auth.worker_token` is configured.
- Existing static token behavior is unchanged.

### Rotation

- New signed tokens are minted with `auth.worker_signing_key`.
- Old tokens can continue verifying through `auth.worker_signing_keys` during a rotation window.
- The documented rotation sequence is:
  - promote the new key into `auth.worker_signing_key`
  - move the old key into `auth.worker_signing_keys`
  - restart or reload workers
  - wait for old token TTLs to expire
  - remove the old key from `auth.worker_signing_keys`

### Worker RPC mTLS

- Added `worker.rpc_tls` configuration for enterprise worker RPC networks.
- Added TLS server support for `stacyvm worker --listen`.
- Added mTLS client support for control-plane calls to worker RPC.
- Added worker RPC mTLS conformance that completes a real RPC call with generated CA, server, and client certificates.
- Added config lint checks for worker server certificates, client CA verification, control-plane client certificates, worker CA verification, and unsafe `insecure_skip_verify`.
- Documented how worker-side and control-plane-side certificate settings are used.

### Documentation

- Updated the README configuration example.
- Updated the worker RPC contract with signed-token semantics.
- Updated the API docs for worker heartbeat and lease-renewal headers.
- Updated the cluster conformance matrix to mark signed worker tokens as the public/enterprise worker identity path.
- Updated production readiness notes to reflect signed worker tokens and the remaining issuer/rotation and mTLS work.

## Code Areas Changed

- `internal/api/middleware`: signed token creation, verification, worker scope filtering, and worker auth config.
- `internal/api`: server worker auth wiring for `auth.worker_signing_key`.
- `internal/config`: config schema and defaults for primary and rotation worker signing keys.
- `internal/worker`: dynamic token callback support for worker heartbeat and lease renewal, plus worker RPC TLS client/server helpers.
- `internal/orchestrator`: worker RPC client TLS wiring for remote worker calls.
- `cmd/stacyvm`: `serve`, `worker`, `worker token`, and `config lint` wiring.
- `docs`: worker identity and conformance documentation.

## Compatibility

The new signed-token path is additive:

- Existing `auth.worker_token` deployments continue to work.
- Existing `auth.worker_tokens.<worker_id>` deployments continue to work.
- Signed tokens can be introduced gradually by setting `auth.worker_signing_key`.
- Key rotation can be introduced gradually by adding old keys to `auth.worker_signing_keys`.
- Worker RPC mTLS is opt-in; local HTTP worker RPC remains available for local development and internal staging.

## Remaining Phase 14 Direction

- Run worker RPC mTLS smoke tests with real certificates in the target enterprise network.
