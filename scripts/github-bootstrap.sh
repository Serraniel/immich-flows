#!/usr/bin/env bash
# Creates labels, milestones and issues. Idempotent: existing items (by name/title) are skipped.
# Usage: scripts/github-bootstrap.sh [owner/repo]
set -euo pipefail

REPO="${1:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"

label() { gh label create "$1" --color "$2" --description "$3" -R "$REPO" --force >/dev/null; }

milestone() {
  local title="$1" desc="$2"
  gh api "repos/$REPO/milestones?state=all&per_page=100" -q '.[].title' | grep -qxF "$title" ||
    gh api "repos/$REPO/milestones" -f title="$title" -f description="$desc" >/dev/null
}

EXISTING="$(gh issue list -R "$REPO" --state all --limit 500 --json title -q '.[].title')"
issue() {
  local ms="$1" labels="$2" title="$3" body="$4"
  if grep -qxF "$title" <<<"$EXISTING"; then return; fi
  gh issue create -R "$REPO" --milestone "$ms" --label "$labels" --title "$title" --body "$body" >/dev/null
  echo "created: $title"
}

label "type: feature" 1d76db "New functionality"
label "type: bug" d73a4a "Something is broken"
label "type: chore" cfd3d7 "Maintenance, tooling"
label "type: docs" 0075ca "Documentation"
label "area: proxy" 5319e7 "Reverse proxy and hooks"
label "area: engine" 5319e7 "Flow engine, steps, triggers"
label "area: ui" 5319e7 "Web UI"
label "area: tags" 5319e7 "Tag management"
label "area: immich-api" 5319e7 "Immich API client / compatibility"
label "area: ci" 5319e7 "CI/CD, releases"
label "priority: p1" b60205 "Must have"
label "priority: p2" fbca04 "Should have"
label "priority: p3" c2e0c6 "Later / nice to have"

M1="v0.1 Foundation"
M2="v0.2 Proxy & events"
M3="v0.3 Flow engine"
M4="v0.4 Tag management"
M5="v0.5 Visual editor"
M6="v1.0 Stable"
M7="Backlog: extensibility"
milestone "$M1" "CI/CD, store, Immich client, auth"
milestone "$M2" "Route hooks, upload hashing, all P1 events"
milestone "$M3" "YAML flows, executor, core steps"
milestone "$M4" "Bulk rename/move/merge of tags, UI scaffold"
milestone "$M5" "Flow designer, YAML editor, run history"
milestone "$M6" "Coverage of out-of-band events, docs, performance"
milestone "$M7" "Custom triggers and actions (low priority)"

# v0.1
issue "$M1" "type: chore,area: ci,priority: p1" "ci: verify PR, edge and release pipelines" \
"- [ ] CI green on PR (test, lint, docker build, PR title)
- [ ] \`edge\` image pushed on merge to main (amd64, arm64)
- [ ] release-please PR opens; merging it publishes \`X.Y.Z\`, \`X.Y\`, \`latest\`
- [ ] Dependabot PRs pass CI"

issue "$M1" "type: feature,area: immich-api,priority: p1" "feat: Immich API client from OpenAPI spec" \
"- Generate Go client from the spec of the pinned Immich release (\`immich-app/immich\` \`open-api/immich-openapi-specs.json\`).
- Only needed endpoints if the generator allows filtering.
- Scheduled workflow: open an issue/PR when a new Immich release changes used endpoints."

issue "$M1" "type: feature,area: engine,priority: p1" "feat: SQLite store with migrations" \
"- \`modernc.org/sqlite\`, WAL, file in \`IMMICH_FLOWS_DATA_DIR\`.
- Embedded, forward-only migrations.
- Tables: users/connections, flows, runs, run_steps, queue."

issue "$M1" "type: feature,area: proxy,priority: p1" "feat: authentication for /_flows routes" \
"- Accept Immich session cookie, bearer token or \`x-api-key\`.
- Validate via \`GET /api/users/me\`, short-lived cache.
- Roles: admin (global flows, tag tools), user (own flows)."

issue "$M1" "type: feature,area: immich-api,priority: p1" "feat: per-user Immich connection with scoped API key" \
"Tags are owned per user, so actions must run as the asset owner.
- User clicks *connect*: create API key via \`POST /api/api-keys\` with minimal permissions.
- Store encrypted (\`IMMICH_FLOWS_SECRET\`), revoke on disconnect.
- Flows for unconnected users are skipped with a clear log entry."

issue "$M1" "type: chore,area: ci,priority: p1" "test: e2e harness with real Immich" \
"- Compose file with Immich (latest major) + immich-flows for tests.
- CI job on PR (path-filtered or label-triggered if slow).
- Helpers: create user, upload asset, read tags."

# v0.2
issue "$M2" "type: feature,area: proxy,priority: p1" "feat(proxy): route hook framework" \
"- Match method + path, pre and post hooks.
- Pre hooks may block with timeout; fail open by default.
- Bodies are streamed, never fully buffered.
- Post hooks get status and parsed JSON response (bounded size)."

issue "$M2" "type: feature,area: proxy,priority: p1" "feat(proxy): hash original upload while streaming" \
"\`POST /api/assets\`
- Tee request body; parse multipart in parallel; hash \`assetData\` (md5, sha1, sha256, sha512).
- Forward original bytes unchanged.
- Correlate with response \`id\` and \`status\` (\`created\`/\`duplicate\`), emit \`asset.uploaded\`.
- Test: 4 GB upload with constant memory."

issue "$M2" "type: feature,area: engine,priority: p1" "feat(events): persistent event queue with retries" \
"- Events persisted before acknowledging hook.
- Worker pool, configurable concurrency, exponential backoff, dead-letter state.
- Survives restart."

issue "$M2" "type: feature,area: proxy,priority: p1" "feat(events): asset trashed, deleted, restored" \
"Routes: \`DELETE /api/assets\` (\`force\`), \`POST /api/trash/empty\`, \`POST /api/trash/restore\`, \`POST /api/trash/restore/assets\`.
Capture asset data needed by flows before deletion (pre hook)."

issue "$M2" "type: feature,area: proxy,priority: p1" "feat(events): duplicates resolved" \
"Route: \`POST /api/duplicates/resolve\`.
- Pre hook snapshots tags of all assets in affected groups.
- Payload: kept, trashed, tag snapshot.
- Note: Immich merges tags itself when exactly one asset is kept."

issue "$M2" "type: feature,area: proxy,priority: p1" "feat(events): tag assigned and removed" \
"Routes: \`PUT/DELETE /api/tags/:id/assets\`, \`PUT /api/tags/assets\`.
Ignore changes made by immich-flows itself to avoid loops (marker header + loop guard)."

# v0.3
issue "$M3" "type: feature,area: engine,priority: p1" "feat(engine): flow YAML schema" \
"- Go types + generated JSON Schema (used by validation and editor).
- Fields: id, name, enabled, trigger (on, if), steps (id, use, with, if, cases), ui.
- Clear validation errors with line numbers."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(engine): expressions and templates" \
"- expr-lang/expr for \`if:\` and \`{{ }}\`.
- Env: trigger, asset, steps, vars, allowlisted env.
- Compile once per flow version, sandboxed, execution timeout."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(engine): executor and run log" \
"- Sequential steps, if/switch branches, step outputs.
- Per-step status, duration, output, error stored in run.
- Retry policy per step; idempotent tag actions."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(steps): hash" \
"- \`source: original\` uses upload hook hashes.
- \`source: stored\`: SHA-1 from Immich checksum; others stream \`GET /api/assets/:id/original\`.
- Algorithms: md5, sha1, sha256, sha512. Hex lowercase."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(steps): regex, split, set" \
"- regex: named groups as outputs, no match = step output \`matched: false\`.
- split via expression helper.
- set: variables for later steps."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(steps): tag.read, tag.assign, tag.remove" \
"- Hierarchical names \`a/b/c\` upserted via \`PUT /api/tags\`.
- Bulk assignment via \`PUT /api/tags/assets\`.
- tag.read with optional prefix filter."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(steps): tag.merge" \
"Union of tags from source assets applied to target assets. Used with \`duplicates.resolved\`."

issue "$M3" "type: feature,area: engine,priority: p2" "feat(steps): metadata.set" \
"Write JSON object to \`PUT /api/assets/:id/metadata\`. Alternative storage for hashes without tag growth."

issue "$M3" "type: feature,area: engine,priority: p1" "feat(flows): load flows from /data/flows" \
"- Load \`*.yaml\` at start, hot reload on change.
- Invalid flows are disabled and reported, others keep running."

issue "$M3" "type: feature,area: engine,priority: p2" "feat(trigger): manual trigger and backfill" \
"Run a flow on selected assets or a search (e.g. all assets of a user) with rate limit. Needed to apply flows to an existing library."

issue "$M3" "type: docs,area: engine,priority: p1" "docs: flow reference and cookbook" \
"Reference for triggers, steps, expressions. Cookbook: SERVICE-A/B filename tags, hash tags, merge tags on dedup."

# v0.4
issue "$M4" "type: feature,area: ui,priority: p1" "feat(ui): SvelteKit scaffold embedded in binary" \
"- \`web/\`, static adapter, served under \`/_flows/\` via go:embed.
- CI job (lint, test, build), Dockerfile stage, Dependabot npm entry."

issue "$M4" "type: feature,area: tags,priority: p1" "feat(tags): rename, move and merge API" \
"- Rename: \`PUT /api/tags/:id\` (name only).
- Move \`XX\` to \`YY/XX\` and merge: create target, reassign assets, delete source (children included).
- Dry run returns affected tags and asset counts. Operations are resumable."

issue "$M4" "type: feature,area: tags,priority: p2" "feat(tags): bulk operations by pattern" \
"Apply rename/move/merge to tags matching a glob or regex, with preview."

issue "$M4" "type: feature,area: ui,priority: p1" "feat(ui): tag management page" \
"Tree view, rename/move/merge with preview and confirmation."

# v0.5
issue "$M5" "type: feature,area: ui,priority: p1" "feat(ui): flow list and YAML editor" \
"CodeMirror with JSON-Schema validation and completion, enable/disable, import/export."

issue "$M5" "type: feature,area: ui,priority: p2" "feat(ui): visual flow designer" \
"- Svelte Flow canvas, node palette from step registry, config forms from JSON Schema.
- Lossless round-trip with YAML (positions in \`ui:\`)."

issue "$M5" "type: feature,area: ui,priority: p1" "feat(ui): run history and logs" \
"List runs per flow, per-step output and errors, re-run failed."

# v1.0
issue "$M6" "type: feature,area: immich-api,priority: p2" "feat(bridge): Immich native plugin for events that bypass the proxy" \
"Immich v3 WASM plugin (\`IMMICH_ALLOW_EXTERNAL_PLUGINS\`) with a step that forwards \`AssetCreate\`, \`AssetMetadataExtraction\`, \`AssetTagged\` to immich-flows via \`httpRequest\`. Covers external libraries and server-side imports. Deduplicate with proxy events."

issue "$M6" "type: feature,area: engine,priority: p2" "feat(events): detect server-side deletes" \
"Trash auto-purge happens inside Immich. Detect via sync API or periodic reconciliation, emit \`asset.deleted\`."

issue "$M6" "type: chore,area: proxy,priority: p1" "perf: proxy load and memory test" \
"Benchmark concurrent large uploads and normal API traffic vs direct Immich. Document overhead."

issue "$M6" "type: docs,priority: p1" "docs: install, configuration, upgrade" \
"Compose examples (plain, with upload optimizer, behind reverse proxy), env reference, Immich major upgrade policy."

# Backlog
issue "$M7" "type: feature,area: engine,priority: p3" "feat(trigger): schedule" "Cron trigger, e.g. nightly reconciliation flows."
issue "$M7" "type: feature,area: engine,priority: p3" "feat(trigger): incoming webhook" "Authenticated endpoint that starts a flow with a JSON payload."
issue "$M7" "type: feature,area: engine,priority: p3" "feat(steps): http request" "Call external services; response available to later steps. Host allowlist."
issue "$M7" "type: feature,area: engine,priority: p3" "feat: custom steps as WASM plugins" \
"Load user steps from \`/data/plugins\` via wazero. Manifest with JSON Schema for config so the UI can render them. Consider compatibility with Immich plugin manifest."
