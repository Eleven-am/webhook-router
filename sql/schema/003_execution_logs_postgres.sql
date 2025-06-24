-- Execution logs table for comprehensive trigger and pipeline_old tracking (PostgreSQL version)
-- Replaces webhook_logs with full end-to-end execution logging

CREATE TABLE IF NOT EXISTS execution_logs (
    id TEXT PRIMARY KEY,
    
    -- Trigger information
    trigger_id TEXT,
    trigger_type TEXT NOT NULL, -- http, schedule, polling, broker, imap, caldav, carddav
    trigger_config JSONB DEFAULT '{}', -- JSON config specific to trigger type
    -- route_id removed - all routing is now handled by triggers
    
    -- Input data
    input_method TEXT, -- HTTP method for HTTP triggers, or trigger-specific method
    input_endpoint TEXT, -- HTTP endpoint or trigger-specific source
    input_headers JSONB DEFAULT '{}', -- JSON headers or metadata
    input_body TEXT, -- Request body or trigger payload
    
    -- Pipeline processing
    pipeline_id TEXT,
    pipeline_stages JSONB DEFAULT '[]', -- JSON array of stages executed
    transformation_data JSONB DEFAULT '{}', -- JSON of transformation details
    transformation_time_ms INTEGER DEFAULT 0,
    
    -- Broker publishing
    broker_id TEXT,
    broker_type TEXT, -- rabbitmq, kafka, redis, aws, gcp
    broker_queue TEXT,
    broker_exchange TEXT,
    broker_routing_key TEXT,
    broker_publish_time_ms INTEGER DEFAULT 0,
    broker_response TEXT, -- Success/failure info from broker
    
    -- Final results
    status TEXT DEFAULT 'processing', -- processing, success, error, partial
    status_code INTEGER, -- HTTP status for HTTP triggers, or custom status
    error_message TEXT,
    output_data TEXT, -- Final processed data
    total_latency_ms INTEGER DEFAULT 0,
    
    -- Timestamps
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    
    -- User association (for scoping)
    user_id TEXT NOT NULL,
    
    -- Foreign key constraints
    FOREIGN KEY (trigger_id) REFERENCES triggers (id) ON DELETE SET NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id) ON DELETE SET NULL,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_execution_logs_trigger_id ON execution_logs(trigger_id);
CREATE INDEX IF NOT EXISTS idx_execution_logs_trigger_type ON execution_logs(trigger_type);
CREATE INDEX IF NOT EXISTS idx_execution_logs_status ON execution_logs(status);
CREATE INDEX IF NOT EXISTS idx_execution_logs_started_at ON execution_logs(started_at);
CREATE INDEX IF NOT EXISTS idx_execution_logs_user_id ON execution_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_execution_logs_pipeline_id ON execution_logs(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_execution_logs_broker_type ON execution_logs(broker_type);