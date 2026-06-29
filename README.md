# cloudflare-simple-ddns

`cloudflare-simple-ddns` is a small self-hosted Go application that periodically detects the host's public IPv4 and/or IPv6 address and updates the corresponding Cloudflare DNS records.

## Example `domains.json`

```json
[
  { "domain": "example.com", "proxied": false, "ip_version": 4 },
  { "domain": "*.example.com", "proxied": true, "ip_version": 4 },
  { "domain": "ipv6.example.com", "proxied": false, "ip_version": 6 }
]
```

Each entry may optionally include `ttl`. If omitted, `1` is used, which means Cloudflare Auto TTL.

Valid values are `ttl=1` or values within Cloudflare's supported range from `60` to `86400`.

## Example `docker-compose.yaml`

```yaml
services:
  cloudflare-simple-ddns:
    build: .
    container_name: cloudflare-simple-ddns
    restart: unless-stopped
    user: "1000:1000"
    environment:
      TZ: Europe/Berlin
      CLOUDFLARE_API_TOKEN: ${CLOUDFLARE_API_TOKEN}
      RUN_INTERVAL: ${RUN_INTERVAL}
    volumes:
      - ./config/domains.json:/config/domains.json:ro
```

## Environment Variables

* `CLOUDFLARE_API_TOKEN`: Used as `Authorization: Bearer ...` when calling the Cloudflare API.
* `RUN_INTERVAL`: Supports Go duration syntax such as `30s`, `5m`, `15m`, or `1h`.

## Required Cloudflare Token Permissions

* `Zone / Zone / Read`
* `Zone / DNS / Edit`

The token should be restricted to only the zones that actually need to be updated.

## Getting Started

```bash
cp .env.example .env
docker compose up -d --build
docker compose logs -f
```
