# Requirements

Priority: **P1** must, **P2** should, **P3** later.

## Deployment
- P1 Single Docker image, amd64 + arm64, configured by env vars.
- P1 Drop-in in front of Immich; chainable with other proxies (e.g. upload optimizer).
- P1 Supports only the latest Immich major.
- P1 Transparent for everything not hooked, including WebSockets and large uploads.

## Events
- P1 Asset uploaded, with hashes of the original bytes before any downstream conversion.
- P1 Asset trashed / deleted / restored.
- P1 Duplicates resolved (with tag state before resolution).
- P1 Tag assigned / removed.
- P2 Events from assets that bypass the proxy (native Immich plugin bridge).
- P2 Manual trigger on selected assets (backfill existing library).
- P3 Schedule trigger; custom triggers via webhook.

## Actions
- P1 Assign, remove, read tags; hierarchical tags auto-created.
- P1 Merge tags of several assets.
- P2 Write asset metadata (key/value).
- P1 File hashes: MD5, SHA-1, SHA-256, SHA-512; original and stored file.
- P1 Filename parsing: regex with named groups, split, conditions.
- P3 HTTP/webhook action; custom actions as plugins (deferred, #38).

## Flows
- P1 Defined in YAML (files and UI).
- P2 Visual editor (Tdarr-like), round-trips with YAML.
- P1 Async execution with retries; run history with per-step logs.
- P1 Runs as asset owner; multi-user.

## Tag management (outside flows)
- P1 Rename tag, move tag (`XX` → `YY/XX`), merge tags, with preview and dry run.
- P2 Bulk operations by pattern.

## Project
- P1 Tests on every PR, edge image from `main`, semver releases with changelog, Dependabot.
- P1 Short docs and compose example.
- P1 AGPL-3.0-or-later.
