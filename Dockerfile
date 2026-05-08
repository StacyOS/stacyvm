FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w -X main.version=$VERSION" -o /stacyvm ./cmd/stacyvm
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o /stacyvm-agent ./cmd/stacyvm-agent

FROM alpine:3.20

RUN apk add --no-cache ca-certificates docker-cli
COPY --from=builder /stacyvm /usr/local/bin/stacyvm
COPY --from=builder /stacyvm-agent /usr/local/bin/stacyvm-agent

EXPOSE 7423

ENTRYPOINT ["stacyvm"]
CMD ["serve"]
