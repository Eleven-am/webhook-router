-- Dead Letter Queue table for failed messages (PostgreSQL)

CREATE TABLE IF NOT EXISTS dlq_messages (
    id TEXT PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL UNIQUE,
    -- route_id removed - triggers now handle all routing
    trigger_id TEXT,
    pipeline_id TEXT,
    source_broker_id TEXT NOT NULL,
    dlq_broker_id TEXT NOT NULL,
    broker_name VARCHAR(100) NOT NULL,
    queue VARCHAR(255) NOT NULL,
    exchange VARCHAR(255),
    routing_key VARCHAR(255) NOT NULL,
    headers JSONB DEFAULT '{}',
    body TEXT NOT NULL,
    error_message TEXT NOT NULL,
    failure_count INTEGER DEFAULT 1,
    first_failure TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_failure TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_retry TIMESTAMP,
    status VARCHAR(50) DEFAULT 'pending', -- pending, retrying, abandoned
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (trigger_id) REFERENCES triggers (id) ON DELETE SET NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id) ON DELETE SET NULL,
    FOREIGN KEY (source_broker_id) REFERENCES broker_configs (id),
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs (id)
);

-- Indexes for DLQ queries
CREATE INDEX idx_dlq_messages_status ON dlq_messages(status);
CREATE INDEX idx_dlq_messages_next_retry ON dlq_messages(next_retry);
CREATE INDEX idx_dlq_messages_failure_count ON dlq_messages(failure_count);
CREATE INDEX idx_dlq_messages_first_failure ON dlq_messages(first_failure);
CREATE INDEX idx_dlq_messages_source_broker ON dlq_messages(source_broker_id);
CREATE INDEX idx_dlq_messages_dlq_broker ON dlq_messages(dlq_broker_id);

-- Update trigger for updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_dlq_messages_updated_at BEFORE UPDATE
    ON dlq_messages FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();