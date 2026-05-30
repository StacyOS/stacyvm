# Build the embedded web UI (static export). The Go binary embeds web/out via
# `//go:embed all:out`, so it must be built before `go build`. The web export is
# OS/arch-independent, so build it once on the native BUILDPLATFORM.
FROM --platform=$BUILDPLATFORM node:20-slim AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Overlay the freshly built web assets (the in-repo web/out is only a placeholder).
COPY --from=frontend /web/out ./web/out
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w -X main.version=$VERSION" -o /stacyvm ./cmd/stacyvm
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o /stacyvm-agent ./cmd/stacyvm-agent

FROM alpine:3.20

RUN apk add --no-cache ca-certificates docker-cli
COPY --from=builder /stacyvm /usr/local/bin/stacyvm
COPY --from=builder /stacyvm-agent /usr/local/bin/stacyvm-agent

EXPOSE 7423

ENTRYPOINT ["stacyvm"]
CMD ["serve"]
