FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" -o /stacyvm ./cmd/stacyvm
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /stacyvm-agent ./cmd/stacyvm-agent

FROM alpine:3.20

RUN apk add --no-cache ca-certificates docker-cli
COPY --from=builder /stacyvm /usr/local/bin/stacyvm
COPY --from=builder /stacyvm-agent /usr/local/bin/stacyvm-agent

EXPOSE 7423

ENTRYPOINT ["stacyvm"]
CMD ["serve"]
