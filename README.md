# immich-flows

Workflow automation for [Immich](https://immich.app). Runs as a drop-in proxy in front of Immich, with flows defined in YAML or a visual editor.

> **Status:** early development. Not usable yet. See [milestones](https://github.com/serraniel/immich-flows/milestones).

## Features (planned)

- Triggers: asset uploaded, trashed/deleted, duplicates resolved, tag assigned/removed
- Actions: assign/remove/read tags, compute file hashes (MD5, SHA-1, SHA-256, SHA-512), parse filenames
- Hashes the **original** upload before any converter (e.g. [immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer)) touches it
- Flow editor: YAML and visual, same model
- Bulk tag tools: rename, move (`XX` → `YY/XX`), merge
- Targets the latest Immich major only (currently v3)

## Install

Add to your Immich `docker-compose.yml` and remove the `ports` section from `immich-server`:

```yaml
services:
  immich-flows:
    image: ghcr.io/serraniel/immich-flows:latest
    ports:
      - "2283:2283"
    environment:
      IMMICH_FLOWS_UPSTREAM_URL: http://immich-server:2283
    volumes:
      - ./flows:/data
    depends_on:
      - immich-server
    restart: always
```

With a converter (immich-flows → upload optimizer → Immich): see [examples/compose.yaml](examples/compose.yaml).

Image tags: `latest`, `X.Y.Z`, `X.Y`, `edge` (main branch).

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `IMMICH_FLOWS_UPSTREAM_URL` | required | Where proxied requests go (Immich or the next proxy) |
| `IMMICH_FLOWS_IMMICH_URL` | upstream | Direct Immich URL for API calls |
| `IMMICH_FLOWS_LISTEN` | `:2283` | Listen address |
| `IMMICH_FLOWS_DATA_DIR` | `/data` | Database and flow files |
| `IMMICH_FLOWS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Docs

- [Architecture](docs/architecture.md)
- [Flows](docs/flows.md)
- [Requirements](docs/requirements.md)

## License

[AGPL-3.0-or-later](LICENSE). Running a modified version as a network service requires publishing its source.
