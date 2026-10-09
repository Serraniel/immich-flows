# Flows

> Draft. Syntax may change until v1.0.

A flow is one YAML document: a trigger and a list of steps. The visual editor reads and writes the same format; node positions go into `ui:`.

```yaml
id: example
name: Example
enabled: true
trigger:
  on: asset.uploaded
  if: 'asset.type == "IMAGE"'     # optional filter
steps:
  - id: h
    use: hash
    with: { source: original, algorithms: [md5, sha1] }
  - use: tag.assign
    with:
      tags: ['hashes/MD5/{{ steps.h.md5 }}']
```

Full example: [examples/flows/service-tags.yaml](../examples/flows/service-tags.yaml).

## Sources

Flows load from `/data/flows/*.yaml` (read-only in UI, GitOps-friendly) and from the UI (stored in the database, exportable as YAML).

## Expressions

`if:` fields and `{{ }}` templates use [expr](https://expr-lang.org/docs/language-definition). Available: `trigger`, `asset`, `steps.<id>`, `vars`, `env` (allowlisted).

## Triggers

| Trigger | Payload |
| --- | --- |
| `asset.uploaded` | `asset`, `upload.original` (hashes, size), `upload.status` (`created`/`duplicate`) |
| `asset.trashed` / `asset.deleted` / `asset.restored` | `assets` |
| `duplicates.resolved` | `kept`, `trashed`, tags of all assets before resolution |
| `tag.assigned` / `tag.removed` | `tag`, `assets` |
| `manual` | `assets` selected in UI |
| `schedule` (low priority) | cron |

## Steps

| Step | Purpose |
| --- | --- |
| `hash` | `source: original\|stored`, `algorithms: [md5, sha1, sha256, sha512]` |
| `regex` | named groups become outputs |
| `set` | assign variables |
| `switch` / `if` | branch |
| `tag.read` | tags of an asset, optional prefix filter |
| `tag.assign` / `tag.remove` | create missing tags (hierarchical `a/b/c`), assign |
| `tag.merge` | union tags of several assets onto targets |
| `metadata.set` | write a JSON object to Immich asset metadata (`PUT /api/assets/:id/metadata`) |
| `http` (low priority) | call a webhook |
| `log` | write to run log |

## Hash notes

- `stored` SHA-1 comes from Immich's checksum, no download needed. Other algorithms stream the original from Immich.
- MD5 and SHA-1 are fine for identifying files, not for security. Add SHA-256 if other tools you use expect it.
- One tag per hash value means many tags. Use a dedicated prefix (`hashes/...`) and pick only the algorithms you need.
- Alternative without tag growth: `metadata.set` with key `immich-flows.hashes`. Not visible or searchable in the Immich UI.
