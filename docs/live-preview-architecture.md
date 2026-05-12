# Production Live Preview Architecture for StacyVM (Implemented)

## Objective
To implement a production-grade, highly scalable "Live Preview" system for 100+ concurrent users entirely powered by **Traefik**. This design supports seamless local development (`*.localhost`) and production environments (`*.stacyide.xyz`), using Traefik's native dynamic discovery.

## Status: Implemented (Phase 1 & 2)
StacyVM now supports automatic Traefik integration for the Docker provider. Preview URLs can be generated via the SDK using `sandbox.getPreviewUrl(port)`. **Today only port 3000 is routed** (see Traefik labels below); the SDK signature accepts any int, but URLs for other ports will not resolve. Configurable ports are tracked on the roadmap.

## Background & Motivation
StacyVM sandboxes need a way to expose an internal web server (today: port 3000, e.g., a Next.js dev server) to external users. Traefik is an industry-standard dynamic reverse proxy that integrates natively with Docker. By using Traefik, we eliminate custom proxy code and leverage robust features like automatic SSL (Let's Encrypt), WebSocket support, and zero-downtime configuration updates.

## Architecture

The architecture adapts based on the StacyVM provider (Docker vs. Firecracker).

### Phase 1: Native Docker Integration (Done)
For the Docker provider, Traefik discovers sandboxes automatically by reading the Docker daemon socket (`/var/run/docker.sock`).

**1. StacyVM Core Modification (Go):**
`internal/providers/docker.go` injects Traefik labels during `Spawn`. These labels define the routing rules and the internal target port.

```go
// Example injected labels:
Labels: map[string]string{
    "traefik.enable": "true",
    // Route for port 3000
    fmt.Sprintf("traefik.http.routers.%s-3000.rule", id): fmt.Sprintf("Host(`3000-%s.%s`)", id, previewDomain),
    fmt.Sprintf("traefik.http.services.%s-3000.loadbalancer.server.port", id): "3000",
}
```

**2. Traefik Deployment:**
Traefik runs as a container with access to `/var/run/docker.sock`. It instantly detects new sandboxes and updates its routing table.

### Phase 2: Workflow & Domain Configuration (Done)

**A. Local Development (Laptop)**
*   **Domain:** Set StacyVM `preview_domain` to `localhost` in `stacyvm.yaml` (or via ENV).
*   **Setup:** Run `docker compose up -d` in the root of the project to start both StacyVM and Traefik together.
*   **Access:** Visit `http://3000-sb-123.localhost`.
 The browser resolves `*.localhost` to your machine, Traefik intercepts it, and forwards it to the sandbox container.

**B. Production Export (`*.stacyide.xyz`)**
*   **Domain:** Set StacyVM `preview_domain` to `stacyide.xyz` in `stacyvm.yaml`.
*   **Setup:** Deploy Traefik on public ports `80/443`. Configure Traefik with an ACME resolver for Wildcard SSL.
*   **DNS:** Point a Wildcard A record (`*.stacyide.xyz`) to the server IP.
*   **Access:** Users visit `https://3000-sb-123.stacyide.xyz`. Traefik handles SSL termination and routes traffic to the internal sandbox IP.

### Phase 3: Supporting Firecracker (Pending)
Firecracker microVMs are not Docker containers and cannot be discovered via the Docker socket.
1. **IP Exposure:** Modify the Firecracker provider to capture and return the guest's internal IP address.
2. **Dynamic Configuration:** When a Firecracker sandbox is spawned, the StacyVM orchestrator pushes a routing rule to Traefik via its **HTTP API** or a Key-Value store (Redis/Etcd):
   `Rule: Host("3000-sb-123.stacyide.xyz") -> Forward: http://172.17.0.5:3000`

## Implementation Details

### 1. API & Model Updates
- Added `PreviewDomain` to `orchestrator.Sandbox` and `orchestrator.SandboxInfo`.
- Docker provider generates a deterministic name/ID for containers to make routing predictable.

### 2. Configuration
- Added `preview_domain` to `server` block in `stacyvm.yaml` (defaults to `localhost`).

### 3. Frontend / SDK
- Exposed `sandbox.getPreviewUrl(port)` in JS/TS SDK.
- Exposed `sandbox.get_preview_url(port)` in Python SDK.
