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

JSON Schema for editor completion: [flow.schema.json](flow.schema.json). Add to the top of a flow file:

```yaml
# yaml-language-server: $schema=https://github.com/serraniel/immich-flows/raw/main/docs/flow.schema.json
```

## Structure

| Field | Description |
| --- | --- |
| `id` | `[a-z0-9][a-z0-9_-]*`, unique |
| `enabled` | default `true` |
| `trigger.on`, `trigger.if` | event and optional filter expression |
| `steps[].id` | needed to reference outputs as `steps.<id>`; unique within the flow |
| `steps[].use` | step type |
| `steps[].if` | skip the step if false |
| `steps[].with` | step input |
| `steps[].continueOnError` | default `false`: a failing step stops the run |
| `steps[].cases` | only for `use: switch`; first case whose `if` is true runs, a case without `if` is the default and must be last |
| `ui` | editor state, ignored by the engine |

Unknown fields are errors.

## Runs

- An event with several assets starts one run per flow and asset. Without assets, one run per flow.
- `trigger.if` is evaluated per run; if false, nothing is recorded.
- `asset.uploaded` runs start after Immich finished metadata extraction for the asset; tags set earlier would be overwritten by it.
- Steps run in order. A failing step fails the run unless `continueOnError: true`.
- Failed runs are retried with backoff (up to 8 attempts), always from the first step: steps must be idempotent. Expression errors are not retried.
- Every run and step (status, output, error, duration) is stored in the run log.

## Files

Flows are loaded from `IMMICH_FLOWS_FLOWS_DIR` (default `/data/flows`): `*.yaml` and `*.yml`, including subdirectories. Hidden files and directories are ignored. Changes are picked up within ~2 s.

Invalid files are logged and listed with their error at `GET /_flows/api/flows` (admins); other flows keep running. Flow IDs must be unique across files.

Flows run for all users who connected immich-flows (see README).

Hooks only query Immich for extra data (snapshots, trash listings) if an enabled flow uses that trigger.

## Expressions

`if:` fields and `{{ }}` templates use [expr](https://expr-lang.org/docs/language-definition). Available: `trigger`, `asset`, `steps.<id>`, `vars`, `env` (allowlisted).

- `if:` must return a bool. Empty means true.
- A value that is exactly one `{{ }}` keeps its type (list, number, …). Mixed text is formatted as a string.
- A list template inside a list is flattened: `tags: ['a', '{{ ["b", "c"] }}']` → `[a, b, c]`.
- Missing fields: use `?.` (`asset?.exif?.make`).

```yaml
if: 'asset.originalFileName startsWith "SERVICE-A_"'
tags: '{{ split(steps.parse.tags, "_") | map("service-a/" + #) }}'
```

## Triggers

| Trigger | Payload |
| --- | --- |
| `asset.uploaded` | `trigger.data.original` (`filename`, `size`, `md5`, `sha1`, `sha256`, `sha512` of the bytes the client sent), `trigger.data.status` (`created`/`duplicate`) |
| `asset.trashed` | trashed via `DELETE /api/assets` |
| `asset.deleted` | permanently deleted (`force`, or emptied trash). `asset` comes from a snapshot taken before deletion (max 500 assets), otherwise only `asset.id` |
| `asset.restored` | restored from trash |
| `duplicates.resolved` | one run per kept asset. `trigger.data`: `duplicateId`, `kept`, `trashed` (asset IDs), `assets` (snapshots before resolution), `tags` (asset ID → tag values before resolution). Trashed assets also emit `asset.trashed` |
| `tag.assigned` / `tag.removed` | user changed tags through Immich. One event per tag, one run per asset. `trigger.data.tag`: `id`, `value`, `name`. Tag changes made by flows do not trigger these (no loops) |
| `manual` | `assets` selected in UI |
| `schedule` (low priority) | cron |

## Steps

| Step | Purpose |
| --- | --- |
| `hash` | `source`: `original` (bytes the client uploaded, `asset.uploaded` only) or `stored` (file in Immich, default). `algorithms`: `md5`, `sha1`, `sha256`, `sha512` (default `[md5, sha1]`). Outputs: one hex string per algorithm, `size` |
| `regex` | `input`, `pattern` (RE2), `caseInsensitive`. Outputs: `matched`, `match`, `groups`, and each named group |
| `split` | `input`, `separator` (default `_`), `trim`, `skipEmpty` (both default true). Outputs: `items`, `count` |
| `set` | any keys; available as `vars.<key>` |
| `switch` | branch, see `cases` |
| `tag.assign` | `tags` (string or list; `a/b/c` creates parents), `asset`/`assets` (default: run's asset). Outputs: `tags`, `count` |
| `tag.remove` | `tags`, `asset`/`assets`. Removes the tags from the assets, keeps the tags |
| `tag.read` | `asset`, `prefix`. Outputs: `tags` (full values), `names` (last segment), `count` |
| `tag.merge` | `from` (asset IDs), `to` (default: run's asset), `prefix`. Applies the union of the `from` assets' tags to `to`. Outputs: `tags`, `count` |
| `metadata.set` | `key`, `value` (object, replaces the previous value), `asset`. Writes Immich asset metadata (`PUT /api/assets/:id/metadata`) |
| `http` (low priority) | call a webhook |
| `log` | write to run log |

## Hash notes

- `stored` SHA-1 comes from Immich's checksum, no download needed. Other algorithms stream the original from Immich.
- MD5 and SHA-1 are fine for identifying files, not for security. Add SHA-256 if other tools you use expect it.
- One tag per hash value means many tags. Use a dedicated prefix (`hashes/...`) and pick only the algorithms you need.
- Alternative without tag growth: `metadata.set` with key `immich-flows.hashes`. Not visible or searchable in the Immich UI.
