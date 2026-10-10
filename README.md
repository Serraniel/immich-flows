# immich-flows

Workflow automation for [Immich](https://immich.app). Runs as a drop-in proxy in front of Immich, with flows defined in YAML or a visual editor.

> **Status:** early development, pre-1.0: flow syntax and API may change. See [milestones](https://github.com/serraniel/immich-flows/milestones).

## Features

- Triggers: asset uploaded, trashed/deleted, duplicates resolved, tag assigned/removed
- Actions: assign/remove/read tags, compute file hashes (MD5, SHA-1, SHA-256, SHA-512), parse filenames
- Hashes the **original** upload before any converter (e.g. [immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer)) touches it
- Flow editor: YAML with completion and validation, or visual; both edit the same file
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
      - ./immich-flows:/data   # flows in ./immich-flows/flows/*.yaml
    depends_on:
      - immich-server
    restart: always
```

The container runs as UID `65532`. For bind mounts: `mkdir -p immich-flows/flows && sudo chown -R 65532:65532 immich-flows`.

With a converter, behind a reverse proxy, backups and upgrades: [docs/install.md](docs/install.md).

Image tags: `latest`, `X.Y.Z`, `X.Y`, `edge` (main branch).

## Setup

Open `https://<your-immich>/_flows/` and click **Connect**. This creates an Immich API key for immich-flows with only the permissions flows need (assets: read/update/download, tags, duplicates: read). Flows only run for connected users; the UI shows a warning until you connect, and the log notes skipped runs once per user.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `IMMICH_FLOWS_UPSTREAM_URL` | required | Where proxied requests go (Immich or the next proxy) |
| `IMMICH_FLOWS_IMMICH_URL` | upstream | Direct Immich URL for API calls |
| `IMMICH_FLOWS_LISTEN` | `:2283` | Listen address |
| `IMMICH_FLOWS_DATA_DIR` | `/data` | Database and flow files |
| `IMMICH_FLOWS_FLOWS_DIR` | `/data/flows` | Flow YAML files, reloaded on change |
| `IMMICH_FLOWS_WORKERS` | `4` | Concurrent flow jobs |
| `IMMICH_FLOWS_TRASH_INTERVAL` | `10m` | How often the trash is checked for assets Immich deleted itself (`asset.deleted`) |
| `IMMICH_FLOWS_HTTP_ALLOW` | none | Hosts the `http` step may call, comma-separated: `api.example.com`, `*.example.com`, `*` |
| `IMMICH_FLOWS_SECRET` | generated | Encrypts stored Immich API keys. If unset, a key is generated in `/data/secret.key` |
| `TZ` | `UTC` | Time zone for `schedule` triggers, e.g. `Europe/Berlin` |
| `IMMICH_FLOWS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Docs

- [Install](docs/install.md)
- [Architecture](docs/architecture.md), [Performance](docs/performance.md)
- [Flows](docs/flows.md), [Cookbook](docs/cookbook.md)
- [Tag tools](docs/tags.md)
- [Requirements](docs/requirements.md)

## License

[AGPL-3.0-or-later](LICENSE). Running a modified version as a network service requires publishing its source.
