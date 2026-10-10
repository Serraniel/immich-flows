-- Original request (JSON engine.RunRequest) so a run can be retried.
ALTER TABLE runs ADD COLUMN request TEXT;
CREATE INDEX runs_user ON runs (user_id, started_at DESC);
CREATE INDEX runs_started ON runs (started_at DESC);
