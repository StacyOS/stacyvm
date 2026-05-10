---
title: "StacyVM Documentation"
description: "Self-hosted sandbox infrastructure for autonomous software systems."
---

# StacyVM

StacyVM gives AI agents isolated, disposable execution environments with provider options for Docker, Firecracker, PRoot, and remote workers.

## Start Here

- [README](README.md) for the product overview and quick start.
- [Deployment](docs/deployment.md) for production installation and operations.
- [API Reference](docs/api.md) for REST endpoints and examples.
- [Public Support Matrix](docs/public-support-matrix.md) for supported runtime claims.
- [Runtime Certification](docs/runtime-certification.md) for host-level readiness checks.

## Current Public Release

Use the latest signed GitHub release and verify artifacts before installing in production:

```bash
scripts/post-release-validate.sh v0.14.4
```

For Docker-focused public self-serve deployments, keep the support claim tied to the certified Linux Docker path unless you have separate host certification evidence for Firecracker, gVisor/Kata, or PRoot.
