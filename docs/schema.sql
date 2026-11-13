-- 跨表关联使用逻辑外键，由应用服务层在事务中维护。

CREATE TABLE files (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL CHECK (length(name) > 0),
    storage_key TEXT NOT NULL UNIQUE CHECK (length(storage_key) > 0),
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    created_at INTEGER NOT NULL,
    deleted_at INTEGER,
    CHECK (deleted_at IS NULL OR deleted_at >= created_at)
);

CREATE INDEX idx_files_created_at ON files(created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE shares (
    id TEXT PRIMARY KEY NOT NULL,
    file_id TEXT NOT NULL, -- 逻辑关联 files.id
    token TEXT NOT NULL UNIQUE CHECK (length(token) >= 43),
    created_at INTEGER NOT NULL,
    revoked_at INTEGER,
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE UNIQUE INDEX idx_shares_one_active_per_file ON shares(file_id)
    WHERE revoked_at IS NULL;

CREATE INDEX idx_shares_file_history ON shares(file_id, created_at DESC);

CREATE TRIGGER shares_cannot_reactivate
BEFORE UPDATE OF revoked_at ON shares
WHEN OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'revoked share cannot be reactivated');
END;
