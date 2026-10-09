CREATE TABLE jobs (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL,
  payload BLOB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'dead')),
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL,
  run_at INTEGER NOT NULL,      -- unix ms
  last_error TEXT,
  created_at INTEGER NOT NULL,  -- unix ms
  updated_at INTEGER NOT NULL   -- unix ms
);
CREATE INDEX jobs_ready ON jobs (status, run_at);
