# Architecture

## Integration options (Immich v3)

| Option | Pre-conversion access | Triggers | Verdict |
| --- | --- | --- | --- |
| **Reverse proxy** | yes, streams the upload body | any API call that passes through | **primary** |
| Immich workflows, core step *Trigger Webhook* | no, sees stored file only | `AssetCreate`, `AssetMetadataExtraction`, `AssetTagged` (no tag details) | **secondary**: `asset.uploaded` for assets not uploaded via the proxy |
| Polling (the sync API rejects API keys) | no | trash contents, delayed | server-side deletes (trash purge) |

Plugins only get a few host functions (`bulkTagAssets`, albums, `httpRequest`) and there is no trigger for delete or duplicate resolution. A proxy alone misses external libraries and server jobs. So immich-flows uses all three. The built-in webhook step is enough as a bridge; no custom WASM plugin is needed.

## Components

```
client ──► immich-flows :2283 ──► [optional converter, e.g. upload optimizer] ──► immich-server
              │  hooks: pre/post per route
              ├─ flow engine (queue, workers)
              ├─ store (SQLite, /data)
              ├─ web UI + API under /_flows
              └─ Immich API client ──────────────────────────────────────────► immich-server
immich-server ── workflow step "Trigger Webhook" (core plugin) ── HTTP ──► immich-flows /_flows/api/webhook
```

- **Proxy**: `httputil.ReverseProxy`, streaming, WebSocket passthrough. Only matched routes are hooked; everything else is untouched.
- **Upload hook**: tees the multipart body; the `assetData` part is hashed while bytes are forwarded. No buffering, no re-encoding.
- **Engine**: flows are DAGs of steps. Runs are queued in SQLite, executed asynchronously with retries, so uploads are never delayed. Pre-hooks that must block (e.g. duplicate resolution) have a timeout and fail open.
- **Trash reconciler**: Immich deletes assets on its own (trash auto-purge) and its sync API rejects API keys. Every `IMMICH_FLOWS_TRASH_INTERVAL` the trash of each connected user is listed and stored with a snapshot; an asset that leaves the trash and no longer exists emits `asset.deleted`. Deletes and restores seen by the proxy are dropped from tracking first, so nothing is reported twice. Only active if a flow uses `asset.deleted`.
- **Identity**: Immich tags are per user. Each user connects once via the UI; immich-flows creates a scoped Immich API key on their behalf and stores it encrypted. Runs act as the asset owner.
- **UI/API auth**: reuses the Immich session (cookie, bearer token or `x-api-key`), validated against `/api/users/me` and cached for 1 minute. Admin-only routes return 403 for other users.

## Hooked routes

| Event | Route |
| --- | --- |
| `asset.uploaded` | `POST /api/assets` |
| `asset.trashed`, `asset.deleted` | `DELETE /api/assets`, `POST /api/trash/empty` |
| `asset.restored` | `POST /api/trash/restore`, `POST /api/trash/restore/assets` |
| `duplicates.resolved` | `POST /api/duplicates/resolve` |
| `tag.assigned`, `tag.removed` | `PUT/DELETE /api/tags/:id/assets`, `PUT /api/tags/assets` |

If Immich's trash is disabled, `DELETE /api/assets` deletes permanently but is reported as `asset.trashed`.

Immich ≥ 2.7 already merges tags of trashed duplicates into the kept asset when exactly one is kept. `duplicates.resolved` covers the remaining cases and custom logic.

## Stack

| Part | Choice | Reason |
| --- | --- | --- |
| Service | Go | single static binary, low memory, stdlib proxy |
| Store | SQLite (`modernc.org/sqlite`, no CGO) | no extra container |
| Expressions | [expr](https://github.com/expr-lang/expr) | fast, sandboxed, typed |
| UI | SvelteKit (static) + Svelte Flow + CodeMirror, embedded in binary | same framework as Immich |
| Immich client | generated from Immich's OpenAPI spec | follows API changes |
| Native plugin | Immich plugin SDK (Extism/WASM) | official extension point |
| Image | distroless static, amd64 + arm64 | small, no shell |
