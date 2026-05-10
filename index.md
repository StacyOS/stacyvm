---
title: "StacyVM"
description: "Self-hosted sandbox infrastructure for running agent code in isolated, disposable environments."
---

# StacyVM

StacyVM gives your applications disposable execution environments for AI agents, code runners, browser previews, and automation workflows. You keep control of the runtime, network, credentials, and audit trail while developers get a simple API and SDK.

<CardGroup cols={2}>
  <Card title="What is StacyVM?" icon="blocks" href="/docs/getting-started/what-is-stacyvm">
    Understand the product, use cases, advantages, and where it fits.
  </Card>
  <Card title="Quickstart" icon="rocket" href="/docs/getting-started/quickstart">
    Start a local server, create a sandbox, run code, and destroy it.
  </Card>
  <Card title="Architecture" icon="network" href="/docs/architecture/system-overview">
    See the control plane, scheduler, workers, providers, store, and sandbox lifecycle.
  </Card>
  <Card title="Example applications" icon="square-terminal" href="/docs/tutorials/code-runner">
    Build Python and TypeScript services that run code inside StacyVM.
  </Card>
  <Card title="Python SDK" icon="package" href="/docs/sdks/python">
    Use StacyVM from Python services, jobs, and agent frameworks.
  </Card>
  <Card title="TypeScript SDK" icon="braces" href="/docs/sdks/typescript">
    Use StacyVM from Node.js, web backends, and TypeScript agents.
  </Card>
</CardGroup>

## What You Can Build

- Run untrusted or generated code in short-lived sandboxes.
- Give coding agents a filesystem, shell, and live preview URL without exposing your host.
- Route sandbox work across Docker, Firecracker, PRoot, local providers, or remote workers.
- Track quotas, audit events, runtime health, and production readiness evidence.
- Integrate through REST, Python, or TypeScript.

## First Sandbox

<CodeGroup>
```bash cURL
curl -sS -X POST http://localhost:7423/api/v1/sandboxes \
  -H "Content-Type: application/json" \
  -H "X-API-Key: sk_test_YOUR_API_KEY" \
  -d '{"image":"python:3.12","ttl":"10m"}'
```

```python Python
from stacyvm import Client

client = Client(
    base_url="http://localhost:7423",
    api_key="sk_test_YOUR_API_KEY",
)

with client.spawn(image="python:3.12", ttl="10m") as sandbox:
    result = sandbox.exec("python3 -c 'print(40 + 2)'")
    print(result.stdout)
```

```typescript TypeScript
import { Client } from "stacyvm";

const client = new Client({
  baseUrl: "http://localhost:7423",
  apiKey: "sk_test_YOUR_API_KEY",
});

await client.withSandbox({ image: "node:20", ttl: "10m" }, async (sandbox) => {
  const result = await sandbox.exec("node -e 'console.log(40 + 2)'");
  console.log(result.stdout);
});
```
</CodeGroup>

## Recommended Path

<Steps>
  <Step title="Install StacyVM">
    Follow the [installation guide](/docs/getting-started/installation) for local Docker, binary, or source builds.
  </Step>
  <Step title="Run the quickstart">
    Use the [quickstart](/docs/getting-started/quickstart) to validate spawn, exec, files, and cleanup.
  </Step>
  <Step title="Pick an integration">
    Choose the [Python SDK](/docs/sdks/python), [TypeScript SDK](/docs/sdks/typescript), or [REST API](/docs/rest/sandboxes).
  </Step>
  <Step title="Prepare production">
    Use the [deployment guide](/docs/deployment), [support matrix](/docs/public-support-matrix), and [runtime certification](/docs/runtime-certification) before making public runtime claims.
  </Step>
</Steps>

For public deployments, only claim support for runtimes you have certified on the target host.
