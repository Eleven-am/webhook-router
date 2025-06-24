-- Initial schema for webhook router
-- Supports both PostgreSQL and SQLite with engine-specific adaptations

-- Users table for authentication
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Settings table for key-value configuration
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Removed routes table - all routing is now handled by triggers

-- Triggers table for various trigger types (HTTP, schedule, polling, etc.)
-- HTTP triggers now contain routing information in their config
CREATE TABLE IF NOT EXISTS triggers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    config TEXT NOT NULL, -- JSON config including routing info for HTTP triggers
    status TEXT DEFAULT 'stopped',
    active BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    last_execution TIMESTAMP,
    next_execution TIMESTAMP,
    pipeline_id TEXT DEFAULT NULL,
    destination_broker_id TEXT DEFAULT NULL, -- For direct broker reference
    dlq_broker_id TEXT DEFAULT NULL,
    dlq_enabled BOOLEAN DEFAULT FALSE,
    dlq_retry_max INTEGER DEFAULT 3,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id TEXT NOT NULL,
    deleted_at TIMESTAMP DEFAULT NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines(id),
    FOREIGN KEY (destination_broker_id) REFERENCES broker_configs(id),
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Pipelines table for data transformation
CREATE TABLE IF NOT EXISTS pipelines (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    stages TEXT NOT NULL,
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Broker configs table for message broker configurations
CREATE TABLE IF NOT EXISTS broker_configs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    config TEXT NOT NULL,
    active BOOLEAN DEFAULT TRUE,
    health_status TEXT DEFAULT 'unknown',
    last_health_check TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    dlq_enabled BOOLEAN DEFAULT FALSE,
    dlq_broker_id TEXT DEFAULT NULL,
    user_id TEXT,
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Indexes for performance

CREATE INDEX IF NOT EXISTS idx_triggers_type ON triggers(type);
CREATE INDEX IF NOT EXISTS idx_triggers_status ON triggers(status);
CREATE INDEX IF NOT EXISTS idx_triggers_active ON triggers(active);
CREATE INDEX IF NOT EXISTS idx_triggers_dlq_enabled ON triggers(dlq_enabled);
CREATE INDEX IF NOT EXISTS idx_triggers_dlq_broker ON triggers(dlq_broker_id);

CREATE INDEX IF NOT EXISTS idx_pipelines_active ON pipelines(active);

CREATE INDEX IF NOT EXISTS idx_broker_configs_type ON broker_configs(type);
CREATE INDEX IF NOT EXISTS idx_broker_configs_active ON broker_configs(active);
CREATE INDEX IF NOT EXISTS idx_broker_configs_dlq_enabled ON broker_configs(dlq_enabled);
CREATE INDEX IF NOT EXISTS idx_broker_configs_dlq_broker ON broker_configs(dlq_broker_id);

-- Unique constraint for HTTP triggers (user_id + path + method)
CREATE UNIQUE INDEX IF NOT EXISTS idx_http_trigger_user_path_method
ON triggers(
    user_id,
    json_extract(config, '$.path'),
    json_extract(config, '$.method')
)
WHERE type = 'http';

