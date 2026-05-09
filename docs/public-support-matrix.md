# Public Support Matrix

This matrix sets expectations for public self-serve StacyVM installs. It separates generally supported workflows from host-gated runtime certification so operators know what can be used immediately, what must be validated on their own infrastructure, and what remains experimental.

## Support Levels

| Level | Meaning |
|---|---|
| Supported | Covered by CI, documented setup, release verification, and support-bundle workflows. |
| Host-certified | Supported after `scripts/certify-runtime.sh` passes on the target host and the report is retained. |
| Preview | Usable for evaluation, but not recommended for production workloads without maintainer review. |
| Experimental | Available for development or constrained environments; not a production isolation boundary. |
| Planned | Production design exists, but implementation is not complete enough for self-serve users. |

## Runtime And Deployment Matrix

| Mode | Support level | Public self-serve status | Required evidence | Limitations |
|---|---|---|---|---|
| Local mock provider | Supported | Development only | `go test ./...` or CI pass | No sandbox isolation; not a production runtime. |
| Single-node Docker/runc | Supported | Technical production with hardened config | `stacyvm config lint --production`, `stacyvm doctor --production`, support bundle | Isolation is container-based; operators must keep Docker, kernel, and seccomp policy patched. |
| Single-node Docker with gVisor | Host-certified | Recommended container hardening path | Runtime certification report for `gvisor` | Requires host runtime installation and Docker runtime wiring outside StacyVM. |
| Single-node Docker with Kata | Host-certified | VM-backed container path | Runtime certification report for `kata` | Requires host runtime installation, VM support, and capacity planning. |
| Firecracker | Host-certified | VM isolation path for Linux/KVM hosts | Runtime certification report for `firecracker` | Requires Linux, KVM, kernel/rootfs/agent assets, and host networking setup. |
| PRoot | Experimental | Development and restricted hosts only | Runtime certification report for `proot` if used | Not a VM or container isolation boundary; production use is not recommended. |
| E2B/custom provider | Preview | Integration-specific | Provider health, conformance results, and provider-specific logs | External provider availability, auth, and isolation guarantees are outside StacyVM's direct control. |
| Multi-worker cluster | Planned | Not public self-serve yet | N/A | Worker registry, placement, leases, and RPC contract exist; Postgres store, network worker transport, OIDC/RBAC, and cluster conformance are still required. |

## Public Install Requirements

Before treating a self-serve install as supported, operators should capture:

- Release verification output from `scripts/verify-release.sh <version> <arch>` or installer output with Sigstore verification.
- `stacyvm config lint --production --file <config>` output with production environment variables loaded.
- `stacyvm upgrade rehearse --config <config> --database <db>` output before binary or image replacement.
- `stacyvm doctor --production` output from the target host.
- `stacyvm support bundle --output support.json` when opening a support issue.
- Runtime certification output for gVisor, Kata, Firecracker, or PRoot hosts.

GitHub bug and production support issue templates ask for this same evidence. Reports without the relevant artifacts may need an extra triage round before maintainers can reproduce or classify the issue.

## Known Public Limitations

- SQLite is the supported single-node store. Durable leases exist for the local foundation; Postgres-backed cluster semantics are still required for multi-worker production.
- API keys and admin keys are supported today. OIDC, SSO, and RBAC remain planned enterprise work.
- Docker/runc is convenient and supported with hardened settings, but it is not equivalent to VM isolation.
- Firecracker production readiness is host-gated because KVM, kernel, rootfs, agent, and networking setup vary by host.
- PRoot is useful where Docker/KVM are unavailable, but it should not be presented as a production isolation boundary.
- Release signatures prove artifact provenance from the StacyVM GitHub Actions release workflow; they do not certify a host runtime.

## Support Triage Links

| Symptom | First remediation path |
|---|---|
| Install verification fails | [releasing.md](releasing.md) |
| Production config lint fails | [deployment.md](deployment.md) |
| Upgrade rehearsal fails | [deployment.md#upgrade-rehearsal-and-rollback](deployment.md#upgrade-rehearsal-and-rollback) |
| Runtime health fails | [runtime-certification.md](runtime-certification.md) |
| Runtime behavior differs across providers | [runtime-conformance.md](runtime-conformance.md) |
| Admin or auth hardening question | [security-governance.md](security-governance.md) |
| Operator diagnostics needed | [deployment.md#support-bundles](deployment.md#support-bundles) |

## Post-Tag Release Verification

CI builds release binaries and validates checksums before code is merged into a release branch. After a real version tag is published, maintainers should run the public verifier against the GitHub release assets:

```bash
scripts/post-release-validate.sh <version>
```

On a clean Linux host, also run the installer in verify-only mode with signatures required:

```bash
STACYVM_VALIDATE_INSTALLER=true scripts/post-release-validate.sh <version>
```

For a final install smoke, run `scripts/install.sh` once with default checksum verification and once with `STACYVM_REQUIRE_SIGNATURES=true` plus `cosign` installed.
