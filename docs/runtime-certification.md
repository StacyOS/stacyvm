# Runtime Certification

Phase 7 treats runtime certification as a required host-level check before a
provider is marked production-ready.

Run dependency checks:

```sh
scripts/certify-runtime.sh all
scripts/certify-runtime.sh docker
scripts/certify-runtime.sh firecracker
scripts/certify-runtime.sh proot
```

Generate a durable artifact for release or host signoff:

```sh
scripts/certify-runtime.sh docker --format markdown --output docker-certification.md
scripts/certify-runtime.sh firecracker --format json --output firecracker-certification.json
```

The script exits non-zero when any required check fails. Warnings are included in
the artifact but do not fail the command. Attach the generated artifact to the
release checklist, support ticket, or infrastructure change record for the host
being certified.

For Firecracker and PRoot, set optional paths to make host validation stricter:

```sh
STACYVM_FIRECRACKER_KERNEL=/var/lib/stacyvm/vmlinux.bin \
  scripts/certify-runtime.sh firecracker --format markdown --output firecracker-certification.md

STACYVM_PROOT_ROOTFS=/var/lib/stacyvm/rootfs \
STACYVM_PROOT_WORKSPACE_BASE=/var/lib/stacyvm/workspaces \
  scripts/certify-runtime.sh proot --format markdown --output proot-certification.md
```

## Certification Matrix

| Runtime | Checks | Production signoff |
|---|---|---|
| Docker | CLI, daemon reachability, seccomp visibility | Pass on target host, then run provider conformance with Docker enabled |
| gVisor | Docker daemon reachability and runtime discovery for `runsc`/gVisor | Pass discovery and run Docker provider with runtime configured |
| Kata | Docker daemon reachability and runtime discovery for Kata | Pass discovery and run Docker provider with runtime configured |
| Firecracker | Binary, `/dev/kvm`, optional kernel path | Pass on Linux/KVM host with configured kernel/rootfs/agent |
| PRoot | `proot` binary, optional rootfs/workspace paths | Pass with configured rootfs and workspace base |

`stacyvm doctor --production` remains the operator-facing readiness command.
The certification script is the lower-level host check for runtime dependencies
that may not exist in CI or on developer laptops.

## Required Phase 8 Signoff Artifacts

Before calling a single-node host production-ready, collect:

- `stacyvm config lint --production --file <config>`
- `stacyvm upgrade rehearse --config <config> --database <db> --backup-output <path>`
- `stacyvm doctor --production`
- `scripts/certify-runtime.sh <runtime> --format markdown --output <runtime>-certification.md`
- Provider conformance or smoke output for the configured runtime.

Store these artifacts with the deployment record. Do not treat a runtime as
certified because CI passed on another host; runtime certification is per-host
and depends on kernel, daemon, KVM, rootfs, and installed runtime state.
