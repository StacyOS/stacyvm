# Phase 14 Worker Identity Hardening

Phase 14 begins the worker identity hardening lane for production multi-worker StacyVM deployments. The goal of this slice is to move beyond static shared worker credentials while keeping the existing Phase 11-13 worker runtime compatible.

## What Changed

### Signed worker tokens

- Added HMAC-SHA256 signed worker token support.
- Added the `stacyvm-worker-v1.<payload>.<signature>` token format.
- Added signed token claims for:
  - `worker_id`
  - token ID through `jti`
  - audience through `aud`
  - optional worker `scopes`
  - issued-at time through `iat`
  - optional not-before time through `nbf`
  - expiry time through `exp`
- Enforced signed-token expiry before accepting worker requests.
- Enforced signed-token not-before and issued-at validation with clock-skew tolerance.
- Enforced a 15 minute max signed worker token lifetime when `iat` is present.
- Enforced that signed `worker_id` must match the `X-Worker-ID` request header.
- Enforced token audience separation between worker-to-control-plane routes and control-plane-to-worker RPC.
- Added `auth.worker_revoked_token_ids` emergency revocation for signed worker token IDs.
- Added worker token issuer `--format json`, `--token-id`, and `--not-before` options for incident-response runbooks.
- Added `stacyvm worker token inspect <token>` to recover unverified signed-token metadata and `jti` values during incident response.
- Added `stacyvm worker token verify <token>` to validate signed tokens against active and rotation keys, expected worker IDs, expected audiences, and revoked token IDs.
- Added `stacyvm worker token rotation-plan` to print a no-secret signing-key rotation checklist, config sketch, and validation commands.
- Added worker secret file flags for token issuance, token verification, and worker runtime startup so operators can use secret-mounted files instead of command-line or environment secrets.
- Filtered signed-token scopes so tokens cannot grant user, API, or admin scopes.
- Added `stacyvm worker token <worker-id>` to issue signed worker tokens from the CLI.

### Configuration

- Added `auth.worker_signing_key`.
- Added `auth.worker_signing_keys` for old verification keys during rotation.
- Kept `auth.worker_token` for shared-token staging compatibility.
- Kept `auth.worker_tokens` for per-worker static token migration paths.
- Updated `stacyvm config lint --production` so a strong `auth.worker_signing_key` satisfies production-aligned worker credential checks.
- Added config lint warnings when revoked signed-token IDs are configured without signed worker-token verification.
- Added config lint warnings for shared worker tokens left enabled beside signed worker tokens, duplicate rotation keys, and rotation keys that repeat the active signing key.

### Worker runtime

- Added dynamic worker token generation for `stacyvm worker` heartbeat and lease-renewal calls.
- When no static `--worker-token` or `auth.worker_token` is configured, a worker can derive short-lived signed control-plane tokens from `auth.worker_signing_key`.
- Workers can read static worker tokens from `--worker-token-file` or signing keys from `--worker-signing-key-file`.
- Worker RPC servers now accept signed control-plane-to-worker tokens.
- Control planes can mint short-lived worker RPC tokens from `auth.worker_signing_key` when no shared `auth.worker_token` is configured.
- Existing static token behavior is unchanged.

### Rotation

- New signed tokens are minted with `auth.worker_signing_key`.
- Old tokens can continue verifying through `auth.worker_signing_keys` during a rotation window.
- Operators can generate a concrete no-secret rollout checklist with `stacyvm worker token rotation-plan`.
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
- Documented the issue, inspect, verify, and revoke operator runbook for worker tokens.
- Updated the API docs for worker heartbeat and lease-renewal headers.
- Updated the cluster conformance matrix to mark signed worker tokens as the public/enterprise worker identity path.
- Added `scripts/certify-worker-identity.sh` for host-level signed-token lifecycle signoff with text, JSON, or Markdown report output.
- Updated production readiness notes to reflect signed worker tokens, worker identity certification reporting, worker RPC mTLS wiring, and the remaining target-network/runtime signoff work.
- Added cluster conformance coverage for signed-token migration lint warnings.
- Added cluster conformance coverage for worker identity certification report generation.

## Code Areas Changed

- `internal/api/middleware`: signed token creation, verification, worker scope filtering, and worker auth config.
- `internal/api`: server worker auth wiring for `auth.worker_signing_key`.
- `internal/config`: config schema and defaults for primary and rotation worker signing keys.
- `internal/worker`: dynamic token callback support for worker heartbeat and lease renewal, plus worker RPC TLS client/server helpers.
- `internal/orchestrator`: worker RPC client TLS wiring for remote worker calls.
- `cmd/stacyvm`: `serve`, `worker`, `worker token`, and `config lint` wiring.
- `scripts`: worker identity certification smoke.
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
