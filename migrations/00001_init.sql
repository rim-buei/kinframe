-- +goose Up
-- +goose StatementBegin
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('google', 'apple')),
    provider_subject TEXT NOT NULL,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (provider, provider_subject)
);

CREATE TABLE spaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_by_account_id TEXT NOT NULL REFERENCES accounts(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);

CREATE TABLE memberships (
    id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL REFERENCES spaces(id),
    account_id TEXT NOT NULL REFERENCES accounts(id),
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'editor', 'viewer')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (space_id, account_id)
);

CREATE TABLE invite_links (
    id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL REFERENCES spaces(id),
    token_hash TEXT NOT NULL UNIQUE,
    default_role TEXT NOT NULL CHECK (default_role IN ('admin', 'editor', 'viewer')),
    created_by TEXT NOT NULL REFERENCES accounts(id),
    expires_at TEXT,
    revoked_at TEXT,
    max_uses INTEGER,
    use_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE albums (
    id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL REFERENCES spaces(id),
    title TEXT NOT NULL,
    description TEXT,
    archive_state TEXT NOT NULL DEFAULT 'hot'
        CHECK (archive_state IN ('hot', 'archiving', 'glacier', 'restoring')),
    archive_job_id TEXT,
    max_bytes INTEGER,
    max_items INTEGER,
    created_by TEXT NOT NULL REFERENCES accounts(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE media (
    id TEXT PRIMARY KEY,
    album_id TEXT NOT NULL REFERENCES albums(id),
    space_id TEXT NOT NULL REFERENCES spaces(id),
    kind TEXT NOT NULL CHECK (kind IN ('photo', 'video')),
    original_key TEXT NOT NULL,
    byte_size INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    width INTEGER,
    height INTEGER,
    duration_ms INTEGER,
    checksum TEXT,
    uploaded_by TEXT NOT NULL REFERENCES accounts(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    deleted_at TEXT
);

CREATE TABLE media_derivatives (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media(id),
    kind TEXT NOT NULL CHECK (kind IN ('thumb', 'preview')),
    object_key TEXT NOT NULL,
    byte_size INTEGER NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    UNIQUE (media_id, kind)
);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('album_archive', 'album_restore')),
    album_id TEXT NOT NULL REFERENCES albums(id),
    space_id TEXT NOT NULL REFERENCES spaces(id),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    progress_pct INTEGER NOT NULL DEFAULT 0,
    eta_seconds INTEGER,
    error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    run_after TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE space_settings (
    space_id TEXT PRIMARY KEY REFERENCES spaces(id),
    theme_id TEXT NOT NULL DEFAULT 'default',
    max_upload_bytes INTEGER
);

CREATE INDEX idx_memberships_account ON memberships(account_id);
CREATE INDEX idx_albums_space ON albums(space_id);
CREATE INDEX idx_media_album ON media(album_id);
CREATE INDEX idx_jobs_status_run_after ON jobs(status, run_after);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS space_settings;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS media_derivatives;
DROP TABLE IF EXISTS media;
DROP TABLE IF EXISTS albums;
DROP TABLE IF EXISTS invite_links;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS spaces;
DROP TABLE IF EXISTS accounts;
-- +goose StatementEnd
