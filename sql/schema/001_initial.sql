-- Initial schema for webhook router
-- Supports both PostgreSQL and SQLite with engine-specific adaptations

-- Users table for authentication
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Settings table for key-value configuration
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Routes table for webhook routing configuration
CREATE TABLE routes (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    endpoint TEXT NOT NULL,
    method TEXT NOT NULL DEFAULT 'POST',
    queue TEXT NOT NULL,
    exchange TEXT DEFAULT '',
    routing_key TEXT NOT NULL,
    filters TEXT DEFAULT '{}',
    headers TEXT DEFAULT '{}',
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    pipeline_id INTEGER DEFAULT NULL,
    trigger_id INTEGER DEFAULT NULL,
    destination_broker_id INTEGER DEFAULT NULL,
    priority INTEGER DEFAULT 100,
    condition_expression TEXT DEFAULT ''
);

-- Triggers table for various trigger types
CREATE TABLE triggers (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    config TEXT NOT NULL,
    status TEXT DEFAULT 'stopped',
    active BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    last_execution TIMESTAMP,
    next_execution TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Pipelines table for data transformation
CREATE TABLE pipelines (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    stages TEXT NOT NULL,
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Broker configs table for message broker configurations
CREATE TABLE broker_configs (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    config TEXT NOT NULL,
    active BOOLEAN DEFAULT TRUE,
    health_status TEXT DEFAULT 'unknown',
    last_health_check TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Webhook logs table for execution tracking
CREATE TABLE webhook_logs (
    id INTEGER PRIMARY KEY,
    route_id INTEGER,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    headers TEXT,
    body TEXT,
    status_code INTEGER DEFAULT 200,
    error TEXT,
    processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trigger_id INTEGER DEFAULT NULL,
    pipeline_id INTEGER DEFAULT NULL,
    transformation_time_ms INTEGER DEFAULT 0,
    broker_publish_time_ms INTEGER DEFAULT 0,
    FOREIGN KEY (route_id) REFERENCES routes (id),
    FOREIGN KEY (trigger_id) REFERENCES triggers (id),
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id)
);

-- Indexes for performance
CREATE INDEX idx_routes_endpoint ON routes(endpoint);
CREATE INDEX idx_routes_active ON routes(active);
CREATE INDEX idx_routes_priority ON routes(priority);

CREATE INDEX idx_webhook_logs_route_id ON webhook_logs(route_id);
CREATE INDEX idx_webhook_logs_processed_at ON webhook_logs(processed_at);

CREATE INDEX idx_triggers_type ON triggers(type);
CREATE INDEX idx_triggers_status ON triggers(status);
CREATE INDEX idx_triggers_active ON triggers(active);

CREATE INDEX idx_pipelines_active ON pipelines(active);

CREATE INDEX idx_broker_configs_type ON broker_configs(type);
CREATE INDEX idx_broker_configs_active ON broker_configs(active);