-- Uploads already reported, to drop duplicates arriving from Immich workflow webhooks.
CREATE TABLE seen_uploads (
  asset_id TEXT PRIMARY KEY,
  seen_at INTEGER NOT NULL       -- unix ms
);
CREATE INDEX seen_uploads_seen_at ON seen_uploads (seen_at);
