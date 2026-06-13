FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/cloudflare-simple-ddns ./cmd/cloudflare-simple-ddns

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /out/cloudflare-simple-ddns /usr/local/bin/cloudflare-simple-ddns

ENTRYPOINT ["/usr/local/bin/cloudflare-simple-ddns"]
