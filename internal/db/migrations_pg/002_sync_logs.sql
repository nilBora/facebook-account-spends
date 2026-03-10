CREATE TABLE sync_runs (
    id            TEXT PRIMARY KEY,
    trigger       TEXT NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    success_count INTEGER NOT NULL DEFAULT 0,
    error_count   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE sync_errors (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES sync_runs(id),
    account_id TEXT NOT NULL,
    message    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sync_errors_run ON sync_errors(run_id);
