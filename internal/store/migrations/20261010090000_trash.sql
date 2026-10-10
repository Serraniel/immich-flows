-- Trashed assets, tracked to detect deletions made by Immich itself (trash auto-purge).
CREATE TABLE trash (
  user_id TEXT NOT NULL,
  asset_id TEXT NOT NULL,
  snapshot BLOB NOT NULL,       -- asset JSON when first seen in the trash
  seen_at INTEGER NOT NULL,     -- unix ms
  PRIMARY KEY (user_id, asset_id)
);
