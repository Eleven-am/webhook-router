-- PostgreSQL schema for webhook router

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table for authentication
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
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

-- Routes table for webhook routing configuration
CREATE TABLE IF NOT EXISTS routes (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(50) NOT NULL DEFAULT 'POST',
    queue VARCHAR(255) NOT NULL,
    exchange VARCHAR(255) DEFAULT '',
    routing_key VARCHAR(255) NOT NULL,
    filters JSONB DEFAULT '{}',
    headers JSONB DEFAULT '{}',
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    pipeline_id INTEGER DEFAULT NULL,
    trigger_id INTEGER DEFAULT NULL,
    destination_broker_id INTEGER DEFAULT NULL,
    priority INTEGER DEFAULT 100,
    condition_expression TEXT DEFAULT ''
);

-- Triggers table for various trigger types
CREATE TABLE IF NOT EXISTS triggers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    config JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'stopped',
    active BOOLEAN DEFAULT true,
    error_message TEXT,
    last_execution TIMESTAMP,
    next_execution TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Pipelines table for data transformation
CREATE TABLE IF NOT EXISTS pipelines (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    stages JSONB NOT NULL,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Broker configs table for message broker configurations
CREATE TABLE IF NOT EXISTS broker_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    config JSONB NOT NULL,
    active BOOLEAN DEFAULT true,
    health_status VARCHAR(50) DEFAULT 'unknown',
    last_health_check TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Webhook logs table for execution tracking
CREATE TABLE IF NOT EXISTS webhook_logs (
    id SERIAL PRIMARY KEY,
    route_id INTEGER,
    method VARCHAR(50) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    headers JSONB,
    body TEXT,
    status_code INTEGER DEFAULT 200,
    error TEXT,
    processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trigger_id INTEGER DEFAULT NULL,
    pipeline_id INTEGER DEFAULT NULL,
    transformation_time_ms INTEGER DEFAULT 0,
    broker_publish_time_ms INTEGER DEFAULT 0,
    FOREIGN KEY (route_id) REFERENCES routes (id) ON DELETE CASCADE,
    FOREIGN KEY (trigger_id) REFERENCES triggers (id) ON DELETE SET NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id) ON DELETE SET NULL
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