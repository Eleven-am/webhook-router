-- Add DLQ configuration to broker_configs table
ALTER TABLE broker_configs ADD COLUMN dlq_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE broker_configs ADD COLUMN dlq_broker_id INTEGER DEFAULT NULL;
ALTER TABLE broker_configs ADD CONSTRAINT fk_dlq_broker FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs(id);