# Product Requirements Document: StacyOS Tunneling & Deployment Fabric

## Problem Statement

Users building AI agents—whether locally on their laptops or inside isolated execution environments like `stacyvm` sandboxes—face significant friction when trying to share, test, or integrate those agents with external services. They must configure cloud infrastructure, manage inbound firewall rules, set up reverse proxies, or manage SSL certificates. They need a simple, zero-configuration way to expose their agents securely to the internet so they can focus entirely on the agent's core product logic rather than deployment operations.

## Solution

The solution is the StacyOS Tunneling & Deployment Fabric, a new managed SaaS platform operating alongside (but decoupled from) the open-source `stacyvm` engine. It provides a globally available Edge Gateway (`stacyos.xyz`) and an integrated Client SDK/CLI. Developers use a single workflow to boot their agent and simultaneously establish a secure, outbound multiplexed connection (gRPC or HTTP/2) to the Edge Gateway. The Gateway then routes public internet traffic directly back to the agent. This "reverse tunnel" approach requires no inbound ports, works identically on a local laptop or inside a sandbox, and paves the way for a unified "make and deploy" ecosystem for low-cost agents.

## User Stories

1. As an agent developer, I want to run a single CLI command to start my agent and expose it, so that I don't have to manage separate tunneling and application processes.
2. As a framework integrator, I want an embeddable SDK (Node/Python) to start the tunnel programmatically, so that my agent framework can handle public routing out-of-the-box.
3. As a developer testing webhooks, I want my tunnel to automatically provision a public HTTPS URL (e.g., `agent-123.stacyos.xyz`), so that external services can send webhooks to my local agent.
4. As an enterprise user, I want the ability to reserve persistent custom subdomains, so that my agent's endpoint remains stable across restarts.
5. As a security-conscious user, I want the tunnel to operate purely via outbound connections, so that I don't have to modify my local firewall or VPC security groups.
6. As a high-traffic agent owner, I want the tunnel to support HTTP/2 multiplexing, so that multiple concurrent requests are handled efficiently without connection exhaustion.
7. As an admin managing a fleet of agents, I want a control plane dashboard, so that I can revoke API keys, monitor active tunnels, and view basic traffic analytics.
8. As a user deploying in `stacyvm`, I want the tunnel client to work perfectly inside a Firecracker microVM, so that my sandboxed workloads are instantly reachable from the internet.

## Implementation Decisions

- **System Architecture**: The system will be divided into three primary components: an Edge Gateway (Ingress), a Control Plane API, and a Tunnel Client (SDK/CLI).
- **Core Protocol**: The connection between the Tunnel Client and the Edge Gateway will use gRPC/HTTP2 multiplexing. This provides lower latency and native stream multiplexing compared to WebSockets, allowing efficient handling of both HTTP and raw TCP traffic.
- **Client Distribution**: The client will be distributed both as a standalone CLI binary and as a native SDK (Node/Python) to support the "integrated process" vision, enabling users to create and deploy agents from one place.
- **Authentication**: Clients will authenticate to the Control Plane using scoped API tokens. Upon authentication, the Control Plane will negotiate endpoint assignment and coordinate routing rules with the Edge Gateway.
- **Routing**: The Edge Gateway will dynamically map incoming SNI (Server Name Indication) hostnames to the corresponding active gRPC stream, routing traffic seamlessly.
- **Decoupling**: The tunneling fabric will not be hardcoded into the open-source `stacyvm` orchestrator. It will run as a user-space workload within the sandbox or on a user's machine.

## Testing Decisions

- Good tests for this project will focus on the external behavior of the tunneling fabric—specifically verifying that a public request successfully reaches the local application and the response is correctly returned.
- **Component Tests**:
  - The Edge Gateway's routing logic will be tested using mock client streams to ensure correct SNI parsing and stream forwarding.
  - The Client SDK will be tested against a mock Control Plane to verify connection backoff, retry logic, and API token validation.
- **End-to-End Tests**:
  - An integration test suite will spin up a local "echo" server, use the SDK to establish a tunnel to a local instance of the Edge Gateway, and make curl requests to verify the full round-trip flow.
- **Prior Art**: Testing will mirror distributed systems patterns, using isolated container networks to simulate internet boundaries and verify reverse tunneling behavior.

## Out of Scope

- Modifying the existing `stacyvm` control plane or worker RPC to act as the public ingress gateway for this new service. `stacyvm` remains an execution engine; StacyOS Tunneling is a network overlay.
- Advanced API gateway features like payload transformation, caching, or complex GraphQL federation.
- Providing actual agent reasoning models or LLM infrastructure; this is strictly the networking/deployment layer.

## Further Notes

- This PRD represents the first phase of building a larger "make and deploy" platform for low-cost agents. The tunneling MVP establishes the vital capability of reachability.
- Future enhancements may include Edge access controls (e.g., adding an OAuth proxy layer in front of the tunnel) and Anycast routing for global latency reduction.