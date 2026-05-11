# StacyVM TypeScript Code Runner Example

This example is a small Express application that runs submitted JavaScript code in a disposable StacyVM sandbox.

## Prerequisites

- Node.js 18+
- A running StacyVM server
- Docker provider access on the StacyVM host
- `STACYVM_API_KEY` when server auth is enabled

## Install

```bash
npm install
```

## Run

```bash
export STACYVM_URL="http://localhost:7423"
export STACYVM_API_KEY="sk_test_YOUR_API_KEY"
export STACYVM_IMAGE="node:20"
npm run dev
```

## Try It

```bash
curl -sS -X POST http://localhost:8081/run-javascript \
  -H "Content-Type: application/json" \
  -d '{"code":"console.log([10, 20, 12].reduce((a, b) => a + b, 0));","timeout":"10s"}'
```

Expected response:

```json
{
  "exit_code": 0,
  "stdout": "42\n",
  "stderr": "",
  "duration": "120ms"
}
```

Use this as a starting point, not as a public service as-is. Add app authentication, request limits, audit logging, and quota attribution before exposing it to users.
