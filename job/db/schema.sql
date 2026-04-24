CREATE TABLE IF NOT EXISTS jobs (
    id         TEXT     NOT NULL PRIMARY KEY,
    type       TEXT     NOT NULL,
    args_data  BLOB,
    status     TEXT     NOT NULL DEFAULT 'PENDING',
    result     TEXT     NOT NULL DEFAULT '',
    priority   INTEGER  NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_type   ON jobs (type);
