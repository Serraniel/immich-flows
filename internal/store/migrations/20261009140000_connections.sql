CREATE TABLE connections (
  user_id TEXT PRIMARY KEY,
  api_key_id TEXT NOT NULL,
  api_key BLOB NOT NULL,        -- AES-GCM encrypted secret
  created_at INTEGER NOT NULL   -- unix ms
);
