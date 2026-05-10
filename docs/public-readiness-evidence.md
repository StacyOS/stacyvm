# Public Readiness Evidence

Use this runbook to create the final evidence bundle before announcing StacyVM as public self-serve production-ready.

The codebase can be public-readiness complete before a release is public-launch complete. Public launch also needs proof from the actual tag, host runtimes, network, and staging environment.

## Candidate Evidence

For a branch or release candidate, run:

```bash
STACYVM_AUTH_API_KEY=replace-with-32-byte-key \
STACYVM_AUTH_ADMIN_API_KEY=replace-with-different-32-byte-key \
scripts/public-readiness-evidence.sh --output public-readiness-evidence.md
```

This verifies:

- shell syntax for public install, release, readiness, upgrade, cluster, and runtime scripts
- `stacyvm config lint --production` against the production config
- full Go test suite
- web production build
- public release sanity build and checksum verification
- upgrade and migration sanity

The report verdict is **PUBLIC SELF-SERVE CANDIDATE** when local gates pass but tag/host-gated evidence is skipped.

## Announcement Evidence

After publishing a real GitHub release tag and choosing the runtime claims for launch, run the full gate:

```bash
STACYVM_AUTH_API_KEY=replace-with-32-byte-key \
STACYVM_AUTH_ADMIN_API_KEY=replace-with-different-32-byte-key \
STACYVM_POST_RELEASE_VERSION=v0.0.0 \
STACYVM_VALIDATE_INSTALLER=true \
STACYVM_RUNTIME_CERTIFY=docker \
STACYVM_RUN_CLUSTER_CONFORMANCE=true \
scripts/public-readiness-evidence.sh --output public-readiness-evidence.md
```

Add runtimes only when the target host can certify them:

```bash
STACYVM_RUNTIME_CERTIFY=docker,gvisor,kata,firecracker
```

Do not claim Firecracker production readiness from a non-Linux or non-KVM host. Do not claim PRoot as a production isolation boundary.

The report verdict is **PUBLIC SELF-SERVE READY** only when all required local, tag, runtime, and cluster gates pass without skips.

## Required Attachments

Keep these artifacts with the release:

- `public-readiness-evidence.md`
- output from `scripts/post-release-validate.sh <version>`
- runtime certification Markdown for every runtime claimed publicly
- live Postgres contract evidence for cluster or multi-worker claims
- target-network worker RPC mTLS smoke output when enterprise/multi-worker support is claimed
- staging install rehearsal notes from published artifacts
- redacted support bundle generated from the staging deployment

## Final Approval Rule

Public announcement is allowed only when:

- the readiness evidence verdict is **PUBLIC SELF-SERVE READY**
- release assets are signed and checksum-verified
- every public runtime claim has host certification evidence
- production config lint passes with real secrets supplied through environment or secret files
- `server.cors_allowed_origins` contains exact trusted origins, not `*`
- rollback, backup/restore, and upgrade rehearsal have been exercised in staging

If any of those are missing, announce the build as a release candidate or technical preview instead.
