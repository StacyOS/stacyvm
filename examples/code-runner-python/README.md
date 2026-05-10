# StacyVM Python Code Runner Example

This example is a small FastAPI application that runs submitted Python code in a disposable StacyVM sandbox.

## Prerequisites

- Python 3.9+
- A running StacyVM server
- Docker provider access on the StacyVM host
- `STACYVM_API_KEY` when server auth is enabled

## Install

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
```

## Run

```bash
export STACYVM_URL="http://localhost:7423"
export STACYVM_API_KEY="sk_test_YOUR_API_KEY"
export STACYVM_IMAGE="python:3.12"
uvicorn app:app --reload --port 8080
```

## Try It

```bash
curl -sS -X POST http://localhost:8080/run-python \
  -H "Content-Type: application/json" \
  -d '{"code":"print(sum([10, 20, 12]))","timeout":"10s"}'
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
