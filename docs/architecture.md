# Architecture

## Integration options (Immich v3)

| Option | Pre-conversion access | Triggers | Verdict |
| --- | --- | --- | --- |
| **Reverse proxy** | yes, streams the upload body | any API call that passes through | **primary** |
| Immich native workflows + WASM plugin | no, sees stored file only | `AssetCreate`, `AssetMetadataExtraction`, `AssetTagged` | **secondary**: event bridge for assets not uploaded via the proxy |
| Polling / sync API | no | everything, delayed | fallback for server-side deletes (trash purge) |

Native workflows are still in preview, plugins only get a few host functions (`bulkTagAssets`, albums, `httpRequest`), and they have no trigger for delete or duplicate resolution. A proxy alone misses external libraries and server jobs. So immich-flows uses both.

## Components

```
client ──► immich-flows :2283 ──► [optional converter, e.g. upload optimizer] ──► immich-server
              │  hooks: pre/post per route
              ├─ flow engine (queue, workers)
              ├─ store (SQLite, /data)
              ├─ web UI + API under /_flows
              └─ Immich API client ──────────────────────────────────────────► immich-server
immich-server ── native workflow step "immich-flows#emit" (WASM) ── HTTP ──► immich-flows
```

- **Proxy**: `httputil.ReverseProxy`, streaming, WebSocket passthrough. Only matched routes are hooked; everything else is untouched.
- **Upload hook**: tees the multipart body; the `assetData` part is hashed while bytes are forwarded. No buffering, no re-encoding.
- **Engine**: flows are DAGs of steps. Runs are queued in SQLite, executed asynchronously with retries, so uploads are never delayed. Pre-hooks that must block (e.g. duplicate resolution) have a timeout and fail open.
- **Identity**: Immich tags are per user. Each user connects once via the UI; immich-flows creates a scoped Immich API key on their behalf and stores it encrypted. Runs act as the asset owner.
- **UI auth**: reuses the Immich session (cookie or API key), validated against `/api/users/me`.

## Hooked routes

| Event | Route |
| --- | --- |
| `asset.uploaded` | `POST /api/assets` |
| `asset.trashed`, `asset.deleted` | `DELETE /api/assets`, `POST /api/trash/empty` |
| `asset.restored` | `POST /api/trash/restore`, `POST /api/trash/restore/assets` |
| `duplicates.resolved` | `POST /api/duplicates/resolve` |
| `tag.assigned`, `tag.removed` | `PUT/DELETE /api/tags/:id/assets`, `PUT /api/tags/assets` |

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
