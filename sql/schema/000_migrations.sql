-- Migration tracking table
-- This table keeps track of which schema migrations have been applied
-- Must be created first before any other migrations

CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    checksum TEXT -- Optional: for verifying migration file integrity
);

-- Index for performance
CREATE INDEX IF NOT EXISTS idx_schema_migrations_applied_at ON schema_migrations(applied_at);