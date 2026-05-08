# Runtime Certification

Phase 7 treats runtime certification as a required host-level check before a
provider is marked production-ready.

Run:

```sh
scripts/certify-runtime.sh all
scripts/certify-runtime.sh docker
scripts/certify-runtime.sh firecracker
scripts/certify-runtime.sh proot
```

## Certification Matrix

| Runtime | Checks | Production signoff |
|---|---|---|
| Docker | CLI, daemon reachability, seccomp visibility | Pass on target host, then run provider conformance with Docker enabled |
| gVisor | Docker runtime discovery for `runsc`/gVisor | Pass discovery and run Docker provider with runtime configured |
| Kata | Docker runtime discovery for Kata | Pass discovery and run Docker provider with runtime configured |
| Firecracker | Binary and `/dev/kvm` presence | Pass on Linux/KVM host with configured kernel/rootfs/agent |
| PRoot | `proot` binary presence | Pass with configured rootfs and workspace base |

`stacyvm doctor --production` remains the operator-facing readiness command.
The certification script is the lower-level host check for runtime dependencies
that may not exist in CI or on developer laptops.
