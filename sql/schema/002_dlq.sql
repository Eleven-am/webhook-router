-- Dead Letter Queue table for failed messages

-- SQLite version
CREATE TABLE IF NOT EXISTS dlq_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id TEXT NOT NULL UNIQUE,
    route_id INTEGER NOT NULL,
    trigger_id INTEGER,
    pipeline_id INTEGER,
    source_broker_id INTEGER NOT NULL,
    dlq_broker_id INTEGER NOT NULL,
    broker_name TEXT NOT NULL,
    queue TEXT NOT NULL,
    exchange TEXT,
    routing_key TEXT NOT NULL,
    headers TEXT DEFAULT '{}',
    body TEXT NOT NULL,
    error_message TEXT NOT NULL,
    failure_count INTEGER DEFAULT 1,
    first_failure DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_failure DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_retry DATETIME,
    status TEXT DEFAULT 'pending', -- pending, retrying, abandoned
    metadata TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (route_id) REFERENCES routes (id) ON DELETE CASCADE,
    FOREIGN KEY (trigger_id) REFERENCES triggers (id) ON DELETE SET NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id) ON DELETE SET NULL,
    FOREIGN KEY (source_broker_id) REFERENCES broker_configs (id),
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs (id)
);

-- Indexes for DLQ queries
CREATE INDEX idx_dlq_messages_status ON dlq_messages(status);
CREATE INDEX idx_dlq_messages_next_retry ON dlq_messages(next_retry);
CREATE INDEX idx_dlq_messages_route_id ON dlq_messages(route_id);
CREATE INDEX idx_dlq_messages_failure_count ON dlq_messages(failure_count);
CREATE INDEX idx_dlq_messages_first_failure ON dlq_messages(first_failure);
CREATE INDEX idx_dlq_messages_source_broker ON dlq_messages(source_broker_id);
CREATE INDEX idx_dlq_messages_dlq_broker ON dlq_messages(dlq_broker_id);