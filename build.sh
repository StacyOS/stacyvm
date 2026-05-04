#!/bin/bash
set -euo pipefail

export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"

echo "Building stacyvm..."
go build -o stacyvm ./cmd/stacyvm

echo "Building stacyvm-agent (static linux/amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/stacyvm-agent ./cmd/stacyvm-agent

echo "Done. Run with: ./stacyvm serve"
