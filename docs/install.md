# Install

immich-flows sits between clients and Immich and forwards everything. Clients (web, mobile, CLI) keep using the same URL.

```
client → immich-flows :2283 → [other proxies] → immich-server :2283
```

## Compose

Add to Immich's `docker-compose.yml` and remove `ports` from `immich-server`, so all traffic goes through immich-flows:

```yaml
services:
  immich-flows:
    image: ghcr.io/serraniel/immich-flows:latest
    ports:
      - "2283:2283"
    environment:
      IMMICH_FLOWS_UPSTREAM_URL: http://immich-server:2283
    volumes:
      - ./immich-flows:/data
    depends_on:
      - immich-server
    restart: always
```

```sh
mkdir -p immich-flows/flows && sudo chown -R 65532:65532 immich-flows   # container runs as UID 65532
docker compose up -d
```

Then open `https://<your-immich>/_flows/`, click **Connect** and add flows (UI or `immich-flows/flows/*.yaml`).

## With an upload converter

immich-flows must be first in the chain to hash the bytes the client sent. Set `IMMICH_FLOWS_IMMICH_URL` so API calls skip the converter: [examples/compose.yaml](../examples/compose.yaml).

## Behind a reverse proxy

Point the reverse proxy at immich-flows instead of `immich-server`. Same requirements as Immich: large uploads, WebSocket, long timeouts.

Caddy:

```
photos.example.com {
	reverse_proxy immich-flows:2283
}
```

nginx (see also [Immich's guide](https://docs.immich.app/administration/reverse-proxy/)):

```nginx
location / {
    proxy_pass http://immich-flows:2283;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 50000M;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;
}
```

immich-flows passes `Host` through and sets `X-Forwarded-*`. Uploads and downloads are streamed, not buffered.

## Configuration

Environment variables: [README](../README.md#configuration).

`/data` contains:

| Path | |
| --- | --- |
| `immich-flows.db` | SQLite: job queue, run log, connections |
| `secret.key` | encrypts stored Immich API keys (unless `IMMICH_FLOWS_SECRET` is set) |
| `flows/` | flow files |

Back up `/data` together. Without the secret, stored API keys cannot be decrypted: users must connect again.

## Upgrades

- immich-flows supports only the latest Immich major. A new Immich major is supported by a new immich-flows release; until then, stay on the previous Immich major.
- Within a major, read the immich-flows release notes before updating Immich.
- Tags: `latest`, `X.Y.Z`, `X.Y`, `edge` (main branch, unstable). Pin `X.Y` to get only fixes.
- Database migrations run at start. No downgrades: back up `/data` before upgrading.
- Breaking changes are listed in the [release notes](https://github.com/serraniel/immich-flows/releases).

## Uninstall

Point clients (or the reverse proxy) back to `immich-server` and remove the service. In Immich, delete the `immich-flows` API keys (Account settings → API keys). Tags and metadata written by flows stay.
