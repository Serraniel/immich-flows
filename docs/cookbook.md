# Cookbook

Flows for common tasks. Reference: [flows.md](flows.md).

## Tags from filenames

`SERVICE-A_123_cat_dog.jpg` → `service-a/cat`, `service-a/dog`.

```yaml
id: service-tags
trigger:
  on: asset.uploaded
  if: 'asset.originalFileName matches "^SERVICE-[AB]_"'
steps:
  - id: parse
    use: regex
    with:
      input: '{{ asset.baseName }}'
      pattern: '^SERVICE-(?P<service>[AB])_(?P<id>[^_]+)_(?P<tags>.+)$'
  - use: tag.assign
    if: steps.parse.matched
    with:
      tags: '{{ split(steps.parse.tags, "_") | map("service-" + lower(steps.parse.service) + "/" + #) }}'
```

Per-service logic: use `switch`, see [examples/flows/service-tags.yaml](../examples/flows/service-tags.yaml).

## Hash tags

Hashes of the uploaded bytes and of the file Immich stored (differ if a proxy or Immich converts the file).

```yaml
id: hash-tags
trigger:
  on: asset.uploaded
steps:
  - id: original
    use: hash
    with: { source: original, algorithms: [md5, sha1] }
  - id: stored
    use: hash
    with: { source: stored, algorithms: [md5, sha1] }
  - use: tag.assign
    with:
      tags:
        - 'hashes/MD5/ORIGINAL/{{ steps.original.md5 }}'
        - 'hashes/SHA1/ORIGINAL/{{ steps.original.sha1 }}'
        - 'hashes/MD5/STORED/{{ steps.stored.md5 }}'
        - 'hashes/SHA1/STORED/{{ steps.stored.sha1 }}'
```

`source: original` exists only for uploads through immich-flows. For a flow that also runs manually (backfill) or for [webhook uploads](install.md#assets-added-outside-immich-flows), guard it:

```yaml
  - id: original
    use: hash
    if: 'trigger.data?.original != nil'
    with: { source: original }
  - use: tag.assign
    if: 'trigger.data?.original != nil'
    with: { tags: 'hashes/MD5/ORIGINAL/{{ steps.original.md5 }}' }
```

Hashes without tags (not visible in the Immich UI):

```yaml
  - use: metadata.set
    with:
      key: immich-flows.hashes
      value: { md5: '{{ steps.stored.md5 }}', sha1: '{{ steps.stored.sha1 }}' }
```

## Merge tags when resolving duplicates

Immich merges tags itself when exactly one asset is kept. For other cases (several kept) or to merge only some tags:

```yaml
id: dedup-tags
trigger:
  on: duplicates.resolved
steps:
  - use: tag.merge
    with:
      from: '{{ trigger.data.trashed }}'
      prefix: service-     # optional
```

Runs once per kept asset; `to` defaults to it.

## React to tags set in Immich

Tag an asset `todo/print` in Immich → also tag it `print-queue`:

```yaml
id: print-queue
trigger:
  on: tag.assigned
  if: 'trigger.data.tag.value == "todo/print"'
steps:
  - use: tag.assign
    with: { tags: print-queue }
```

Tags assigned by flows do not trigger flows.

## Mark trashed assets

```yaml
id: trashed
trigger:
  on: asset.trashed
steps:
  - use: tag.assign
    with: { tags: lifecycle/trashed }
  - use: tag.remove
    with: { tags: lifecycle/restored }
```

## Run on existing assets

Any flow can run on existing assets (as the caller), e.g. to backfill hash tags:

```http
POST /_flows/api/flows/hash-tags/run
{"all": true}
```

Progress and errors: Runs page (`/_flows/runs`).

## Rename or reorganize tags

Not a flow: use the Tags page (`/_flows/tags`) or [the tag API](tags.md), e.g. move every `service-a/*` tag to `services/a/*` with one bulk operation.

## Notify another service

`IMMICH_FLOWS_HTTP_ALLOW=ntfy.sh`:

```yaml
id: notify-upload
trigger:
  on: asset.uploaded
  if: 'asset.type == "VIDEO"'
steps:
  - use: http
    with:
      method: POST
      url: https://ntfy.sh/my-immich
      body: 'New video: {{ asset.originalFileName }}'
```
