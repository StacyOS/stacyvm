.PHONY: build build-agent build-android build-agent-arm64 test lint clean serve dev release-build release-build-all web build-desktop

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
DIST_DIR ?= dist

# Wails needs the webkit2gtk build tag on Linux only; macOS (WKWebView) and
# Windows (WebView2) use their native webview. Detect the host so that
# `make build-desktop` works on every OS.
UNAME_S := $(shell uname -s)
WAILS_TAGS := $(if $(filter Linux,$(UNAME_S)),-tags webkit2_41,)

# Build the server/CLI binary (which includes the TUI and Web UI)
build: web
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o stacyvm ./cmd/stacyvm

# Build the desktop app using Wails. The frontend (web/out) is installed and
# built by the frontend:install/frontend:build steps in desktop/wails.json, so
# no `web` prerequisite is needed here.
build-desktop:
	cd desktop && wails build $(WAILS_TAGS)

# Build the guest agent (static, linux/amd64)
build-agent:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/stacyvm-agent ./cmd/stacyvm-agent

# Build for Android/ARM64 (static)
build-android: web
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/stacyvm-linux-arm64 ./cmd/stacyvm

# Build the guest agent (static, linux/arm64)
build-agent-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/stacyvm-agent-arm64 ./cmd/stacyvm-agent

# Build everything
build-all: build build-desktop build-agent

# Run all tests
test:
	go test ./...

# Run tests with verbose output
test-v:
	go test -v ./...

# Build the web frontend
web:
	cd web && npm install && npm run build

# Start the server
serve: build
	./stacyvm serve

# Check local prerequisites, build, and start a development server
dev:
	./scripts/dev.sh

# Clean build artifacts
clean:
	rm -f stacyvm stacyvm-agent
	rm -rf bin/ web/dist/ $(DIST_DIR)/

# Run go vet
lint:
	go vet ./...

# Build static release binaries + checksums (amd64 only)
# Depends on `web` so the embedded UI (web/out) is the real build, not the placeholder.
release-build: web
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(DIST_DIR)/stacyvm-linux-amd64 ./cmd/stacyvm
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/stacyvm-agent-linux-amd64 ./cmd/stacyvm-agent
	cd $(DIST_DIR) && sha256sum stacyvm-linux-amd64 stacyvm-agent-linux-amd64 > checksums.txt

# Build release binaries for all architectures (amd64 + arm64)
release-build-all: release-build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(DIST_DIR)/stacyvm-linux-arm64 ./cmd/stacyvm
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST_DIR)/stacyvm-agent-linux-arm64 ./cmd/stacyvm-agent
	cd $(DIST_DIR) && sha256sum stacyvm-linux-amd64 stacyvm-agent-linux-amd64 stacyvm-linux-arm64 stacyvm-agent-linux-arm64 > checksums.txt
