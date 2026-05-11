# Runtime Conformance Matrix

This matrix describes what operators should validate before treating a StacyVM runtime provider as production-ready on a host class. The shared provider contract is documented in `docs/provider-contract.md`; this guide focuses on deployment conformance.

## Summary

| Runtime | Host requirement | Production status | Required validation |
|---|---|---|---|
| Docker with `runc` | Docker daemon and socket access | Default broad-compatibility path | Provider health, lifecycle, exec, files, live preview, reconciliation |
| Docker with gVisor `runsc` | Docker daemon plus installed `runsc` runtime | Stronger container isolation | Same as Docker plus runtime selection and syscall compatibility |
| Docker with Kata | Docker daemon plus installed Kata runtime and virtualization support | VM-backed container isolation | Same as Docker plus nested virtualization/runtime availability |
| Firecracker | Linux, `/dev/kvm`, Firecracker binary, kernel, rootfs, networking, `stacyvm-agent` | Highest-isolation target | Full lifecycle and file/exec conformance on real Linux/KVM host |
| PRoot | `proot` binary, rootfs with expected tools, writable workspace base | Restricted-host fallback | Lifecycle, exec, files, limits, and rootfs language/tool availability |
| E2B | E2B API key and network access | Hybrid/cloud burst option | API reachability, lifecycle, exec, files, and failure mapping |
| Custom | Reachable provider HTTP service | Bring-your-own runtime | Contract conformance against the custom backend |

## Baseline Checks

Run these checks for every runtime:

```bash
make test
scripts/smoke-deployment.sh http://127.0.0.1:7423 "$STACYVM_API_KEY"
curl -fsS -H "X-API-Key: $STACYVM_API_KEY" http://127.0.0.1:7423/api/v1/providers
curl -fsS -H "X-API-Key: $STACYVM_API_KEY" http://127.0.0.1:7423/api/v1/ready
```

For a deployed service, use `STACYVM_SMOKE_URL` instead of positional arguments:

```bash
STACYVM_SMOKE_URL=https://stacyvm.example.com STACYVM_API_KEY=sk-live scripts/smoke-deployment.sh
```

## Docker

Required host state:

- Docker daemon is running.
- StacyVM can access the configured Docker socket.
- The sandbox network exists when `providers.docker.network_mode` is a named network.
- Traefik or another reverse proxy can reach sandbox containers for live preview.

Recommended validation:

```bash
docker info
docker network inspect stacyvm-network
STACYVM_DOCKER_INTEGRATION=1 STACYVM_PROVIDERS_DEFAULT=docker make test
```

Runtime behavior to verify:

- `GET /api/v1/providers/docker` reports healthy.
- Spawn an `alpine:latest` sandbox.
- Execute `echo ok`.
- Write, read, list, move, chmod, stat, glob, and delete a file.
- Destroy the sandbox.
- Restart StacyVM and confirm orphaned StacyVM containers reconcile correctly.

## Docker gVisor

Required host state:

- Docker daemon is running.
- `runsc` is installed and registered as a Docker runtime.
- StacyVM config sets `providers.docker.runtime: "runsc"`.

Recommended validation:

```bash
docker info | grep -A5 Runtimes
docker run --rm --runtime=runsc alpine:latest echo ok
```

Runtime behavior to verify:

- Docker provider health remains healthy with `runtime=runsc`.
- Basic spawn, exec, file operations, destroy, and live preview still pass.
- Workloads that need unusual syscalls are tested explicitly because gVisor changes syscall behavior.

## Docker Kata

Required host state:

- Kata runtime is installed and registered with Docker.
- Host supports the virtualization mode required by the Kata installation.
- StacyVM config sets `providers.docker.runtime` to the registered Kata runtime name.

Recommended validation:

```bash
docker info | grep -A5 Runtimes
docker run --rm --runtime=kata-runtime alpine:latest echo ok
```

Runtime behavior to verify:

- Docker provider health remains healthy with the Kata runtime.
- Spawn, exec, file operations, destroy, and live preview pass.
- Cold-start latency and memory overhead are measured against operator SLOs.

## Firecracker

Required host state:

- Linux host with `/dev/kvm` available.
- Firecracker binary installed and executable.
- Kernel image exists at `providers.firecracker.kernel_path`.
- Rootfs image exists for the requested sandbox image or template.
- `stacyvm-agent` is available at `providers.firecracker.agent_path`.
- Networking setup permits guest communication.

Recommended validation:

```bash
test -e /dev/kvm
firecracker --version
test -f /var/lib/stacyvm/vmlinux.bin
test -x /usr/local/bin/stacyvm-agent
```

Runtime behavior to verify:

- `GET /api/v1/providers/firecracker` reports healthy.
- Full provider conformance passes on the Linux/KVM host.
- Snapshot restore paths work for prepared rootfs images.
- Destroy cleans up processes, sockets, tap devices, and temporary runtime files.
- Reconciliation correctly handles stale persisted sandboxes after a StacyVM restart.

## PRoot

Required host state:

- `proot` binary is installed.
- Rootfs exists at `providers.proot.rootfs_path`.
- Workspace base is writable by the StacyVM process.
- Rootfs contains the languages and binaries advertised by `providers.proot.languages`.

Recommended validation:

```bash
proot --version
test -d /var/lib/stacyvm/rootfs
test -w /var/lib/stacyvm/workspaces
```

Runtime behavior to verify:

- `GET /api/v1/providers/proot` reports healthy.
- Basic lifecycle, exec, and file operations pass against the real rootfs.
- Configured memory and disk caps are understood as operational controls, not VM-grade isolation.
- Rootfs language availability matches templates and SDK examples.

## E2B And Custom Providers

Required host state:

- Outbound network access to the provider.
- API keys configured through environment variables or a secret manager.
- Provider-specific base URL configured.

Runtime behavior to verify:

- Provider health returns actionable errors when credentials or network are wrong.
- Lifecycle, exec, streaming exec, files, and destroy match `docs/provider-contract.md`.
- Provider errors map to typed StacyVM errors instead of leaking backend-specific response bodies.

## Signoff Template

Use this checklist before marking a runtime production-ready:

```text
Runtime:
Host OS/kernel:
StacyVM version:
Config file:
Provider health endpoint:
Smoke script result:
Lifecycle conformance:
Exec conformance:
File conformance:
Streaming conformance:
Live preview:
Restart reconciliation:
Known host caveats:
Owner/signoff:
Date:
```

For an auditable host artifact, generate the signoff scaffold directly:

```bash
scripts/certify-runtime.sh docker --format markdown --output docker-certification.md
scripts/certify-runtime.sh firecracker --format markdown --output firecracker-certification.md
scripts/certify-runtime.sh proot --format markdown --output proot-certification.md
```

The generated report includes host metadata, dependency checks, overall status,
and an operator signoff section. Attach provider conformance logs and smoke
script output next to that artifact for final production approval.
