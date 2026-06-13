# cloudflare-simple-ddns

`cloudflare-simple-ddns` ist eine kleine selbst gehostete Go-Anwendung, die in festen Intervallen die öffentliche IPv4- und/oder IPv6-Adresse des Hosts ermittelt und die passenden Cloudflare-DNS-Records aktualisiert.

## Beispiel `domains.json`

```json
[
  {"domain":"example.com","proxied":false,"ip_version":4},
  {"domain":"*.example.com","proxied":true,"ip_version":4},
  {"domain":"ipv6.example.com","proxied":false,"ip_version":6}
]
```

Optional kann jeder Eintrag zusätzlich `ttl` enthalten. Ohne Angabe wird `1` verwendet, also Cloudflare Auto TTL.

Gültig sind `ttl=1` oder Werte im Cloudflare-Bereich `60` bis `86400`.

## Beispiel `docker-compose.yaml`

```yaml
services:
  cloudflare-simple-ddns:
    build: .
    container_name: cloudflare-simple-ddns
    restart: unless-stopped
    user: "1000:1000"
    environment:
      CLOUDFLARE_API_TOKEN: ${CLOUDFLARE_API_TOKEN}
      RUN_INTERVAL: 15m
    volumes:
      - ./config:/config:ro
```

## Environment Variables

- `CLOUDFLARE_API_TOKEN`: Wird für `Authorization: Bearer ...` gegen die Cloudflare API verwendet.
- `RUN_INTERVAL`: Unterstützt Go-Duration-Syntax wie `30s`, `5m`, `15m` oder `1h`.

## Benötigte Cloudflare-Token-Rechte

- `Zone / Zone / Read`
- `Zone / DNS / Edit`

Der Token sollte nur auf die tatsächlich betroffenen Zonen eingeschränkt werden.

## Start

```bash
cp .env.example .env
docker compose up -d --build
docker compose logs -f
```
