# Tag tools

Immich can rename a tag but not move it, and renaming leaves children on the old path. immich-flows does both, including children, and merges into existing tags.

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
