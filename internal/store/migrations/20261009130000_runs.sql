CREATE TABLE runs (
  id INTEGER PRIMARY KEY,
  flow_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  user_id TEXT NOT NULL,
  asset_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
  error TEXT,
  started_at INTEGER NOT NULL,  -- unix ms
  finished_at INTEGER
);
CREATE INDEX runs_flow ON runs (flow_id, started_at DESC);

CREATE TABLE run_steps (
  run_id INTEGER NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  step_id TEXT,
  use TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('success', 'failed', 'skipped')),
  output TEXT,                  -- JSON
  error TEXT,
  started_at INTEGER NOT NULL,
  duration_ms INTEGER NOT NULL,
  PRIMARY KEY (run_id, seq)
);
