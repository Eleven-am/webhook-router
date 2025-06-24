-- PostgreSQL schema for webhook router

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table for authentication
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Settings table for key-value configuration
CREATE TABLE IF NOT EXISTS settings (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Removed routes table - all routing is now handled by triggers

-- Triggers table for various trigger types (HTTP, schedule, polling, etc.)
-- HTTP triggers now contain routing information in their config
CREATE TABLE IF NOT EXISTS triggers (
    id TEXT PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    config JSONB NOT NULL, -- JSON config including routing info for HTTP triggers
    status VARCHAR(50) DEFAULT 'stopped',
    active BOOLEAN DEFAULT true,
    error_message TEXT,
    last_execution TIMESTAMP,
    next_execution TIMESTAMP,
    pipeline_id TEXT DEFAULT NULL,
    destination_broker_id TEXT DEFAULT NULL, -- For direct broker reference
    dlq_broker_id TEXT DEFAULT NULL,
    dlq_enabled BOOLEAN DEFAULT false,
    dlq_retry_max INTEGER DEFAULT 3,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id TEXT NOT NULL,
    deleted_at TIMESTAMP DEFAULT NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines(id) ON DELETE SET NULL,
    FOREIGN KEY (destination_broker_id) REFERENCES broker_configs(id) ON DELETE SET NULL,
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs(id) ON DELETE SET NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Pipelines table for data transformation
CREATE TABLE IF NOT EXISTS pipelines (
    id TEXT PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    stages JSONB NOT NULL,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Broker configs table for message broker configurations
CREATE TABLE IF NOT EXISTS broker_configs (
    id TEXT PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    config JSONB NOT NULL,
    active BOOLEAN DEFAULT true,
    health_status VARCHAR(50) DEFAULT 'unknown',
    last_health_check TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    dlq_enabled BOOLEAN DEFAULT false,
    dlq_broker_id TEXT DEFAULT NULL,
    user_id TEXT,
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs(id) ON DELETE SET NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for performance

CREATE INDEX idx_triggers_type ON triggers(type);
CREATE INDEX idx_triggers_status ON triggers(status);
CREATE INDEX idx_triggers_active ON triggers(active);
CREATE INDEX idx_triggers_dlq_enabled ON triggers(dlq_enabled);
CREATE INDEX idx_triggers_dlq_broker ON triggers(dlq_broker_id);

CREATE INDEX idx_pipelines_active ON pipelines(active);

CREATE INDEX idx_broker_configs_type ON broker_configs(type);
CREATE INDEX idx_broker_configs_active ON broker_configs(active);
CREATE INDEX idx_broker_configs_dlq_enabled ON broker_configs(dlq_enabled);
CREATE INDEX idx_broker_configs_dlq_broker ON broker_configs(dlq_broker_id);

-- Unique constraint for HTTP triggers (user_id + path + method)
CREATE UNIQUE INDEX idx_http_trigger_user_path_method
ON triggers(
    user_id,
    (config->>'path'),
    (config->>'method')
)
WHERE type = 'http';