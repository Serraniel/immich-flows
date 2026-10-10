# Tag tools

Immich can rename a tag but not move it, and renaming leaves children on the old path. immich-flows does both, including children, and merges into existing tags.

UI: `/_flows/tags` shows the tag tree; select a tag or enter a pattern, preview, then apply. The API below does the same.

```http
POST /_flows/api/tags/move
{"from": "XX", "to": "YY/XX", "dryRun": true}
```

| Case | from → to |
| --- | --- |
| Rename | `a/b` → `a/c` |
| Move | `XX` → `YY/XX` |
| Merge | `cat` → `cats` (existing) |

- `dryRun: true` returns the plan (tags, target, asset counts) without changes.
- Applies to the caller's own tags, with the caller's credentials.
- Steps: create target tags, tag the assets (trashed included), delete the source subtree. Safe to re-run after a failure.
- Moving a tag into its own subtree is rejected.
- Immich re-imports tags from sidecar files after its background sidecar jobs, which can bring source tags back. The move waits for Immich's queues to go idle (fixed 3 s for non-admins) and repeats until the source stays gone, up to 5 times. `settled: false` in the response means: re-run later.

## Bulk

```http
POST /_flows/api/tags/bulk
{"match": "^service-a/", "replace": "services/a/", "dryRun": true}
```

- `match`: RE2 regex on full tag values. `replace`: replacement with `$1`, `${name}`.
- Each matching tag is moved like above; children of a matching tag move with it.
- Rejected if two tags map to the same target or a target lies inside another source (run in two steps).
- Stops at the first failed move; the response lists completed moves.
