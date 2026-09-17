-- sqlstore/schema.sql — PostgreSQL
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,
    device        JSONB NOT NULL DEFAULT '{}',
    client        JSONB NOT NULL DEFAULT '{}',
    network       JSONB NOT NULL DEFAULT '{}',
    data          JSONB NOT NULL DEFAULT '{}',
    locale        TEXT NOT NULL DEFAULT '',
    locale_source TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    renewed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx  ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_idx  ON sessions (expires_at);
