-- PostgreSQL-specific queries using $1, $2 syntax

-- Users queries
-- name: CreateUser :one
INSERT INTO users (username, password_hash, is_default)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: UpdateUser :one
UPDATE users 
SET username = $2, password_hash = $3, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateUserCredentials :exec
UPDATE users
SET username = $2, password_hash = $3, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- Settings queries
-- name: SetSetting :exec
INSERT INTO settings (key, value, updated_at)
VALUES ($1, $2, CURRENT_TIMESTAMP)
ON CONFLICT (key) DO UPDATE 
SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP;

-- name: GetSetting :one
SELECT * FROM settings WHERE key = $1;

-- name: ListSettings :many
SELECT * FROM settings ORDER BY key;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = $1;

-- Routes queries
-- name: CreateRoute :one
INSERT INTO routes (
    name, endpoint, method, queue, exchange, routing_key,
    filters, headers, active, pipeline_id, trigger_id,
    destination_broker_id, priority, condition_expression,
    signature_config, signature_secret
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetRoute :one
SELECT * FROM routes WHERE id = $1;

-- name: GetRouteByName :one
SELECT * FROM routes WHERE name = $1;

-- name: GetRouteByEndpoint :one
SELECT * FROM routes WHERE endpoint = $1 AND method = $2;

-- name: ListRoutes :many
SELECT * FROM routes ORDER BY priority ASC, created_at DESC;

-- name: ListActiveRoutes :many
SELECT * FROM routes WHERE active = true ORDER BY priority ASC, created_at DESC;

-- name: UpdateRoute :one
UPDATE routes
SET name = $2, endpoint = $3, method = $4, queue = $5, exchange = $6,
    routing_key = $7, filters = $8, headers = $9, active = $10,
    pipeline_id = $11, trigger_id = $12, destination_broker_id = $13,
    priority = $14, condition_expression = $15, signature_config = $16,
    signature_secret = $17, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteRoute :exec
DELETE FROM routes WHERE id = $1;

-- Triggers queries
-- name: CreateTrigger :one
INSERT INTO triggers (name, type, config, status, active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTrigger :one
SELECT * FROM triggers WHERE id = $1;

-- name: ListTriggers :many
SELECT * FROM triggers ORDER BY created_at DESC;

-- name: UpdateTrigger :one
UPDATE triggers
SET name = $2, type = $3, config = $4, status = $5, active = $6,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteTrigger :exec
DELETE FROM triggers WHERE id = $1;

-- Pipelines queries
-- name: CreatePipeline :one
INSERT INTO pipelines (name, description, stages, active)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPipeline :one
SELECT * FROM pipelines WHERE id = $1;

-- name: ListPipelines :many
SELECT * FROM pipelines ORDER BY created_at DESC;

-- name: UpdatePipeline :one
UPDATE pipelines
SET name = $2, description = $3, stages = $4, active = $5,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeletePipeline :exec
DELETE FROM pipelines WHERE id = $1;

-- Broker configs queries
-- name: CreateBrokerConfig :one
INSERT INTO broker_configs (name, type, config, active, health_status, dlq_enabled, dlq_broker_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetBrokerConfig :one
SELECT * FROM broker_configs WHERE id = $1;

-- name: GetBrokerConfigWithDLQ :one
SELECT 
    bc.*,
    dlq.id as dlq_id,
    dlq.name as dlq_name,
    dlq.type as dlq_type,
    dlq.config as dlq_config
FROM broker_configs bc
LEFT JOIN broker_configs dlq ON bc.dlq_broker_id = dlq.id
WHERE bc.id = $1;

-- name: ListBrokerConfigs :many
SELECT * FROM broker_configs ORDER BY created_at DESC;

-- name: UpdateBrokerConfig :one
UPDATE broker_configs
SET name = $2, type = $3, config = $4, active = $5, dlq_enabled = $6, dlq_broker_id = $7,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteBrokerConfig :exec
DELETE FROM broker_configs WHERE id = $1;

-- Webhook logs queries
-- name: CreateWebhookLog :one
INSERT INTO webhook_logs (
    route_id, method, endpoint, headers, body, status_code,
    error, trigger_id, pipeline_id, transformation_time_ms,
    broker_publish_time_ms
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetWebhookLog :one
SELECT * FROM webhook_logs WHERE id = $1;

-- name: ListWebhookLogs :many
SELECT * FROM webhook_logs ORDER BY processed_at DESC LIMIT $1 OFFSET $2;

-- name: DeleteOldWebhookLogs :exec
DELETE FROM webhook_logs WHERE processed_at < $1;

-- name: GetWebhookLogsByRouteID :many
SELECT * FROM webhook_logs 
WHERE route_id = $1
ORDER BY processed_at DESC 
LIMIT $2 OFFSET $3;

-- name: GetWebhookLogStats :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 END) as success_count,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_count,
    AVG(transformation_time_ms) as avg_transformation_time,
    AVG(broker_publish_time_ms) as avg_publish_time,
    MAX(transformation_time_ms) as max_transformation_time,
    MAX(broker_publish_time_ms) as max_publish_time
FROM webhook_logs
WHERE processed_at >= $1;

-- name: GetWebhookLogStatsByRoute :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 END) as success_count,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_count,
    AVG(transformation_time_ms) as avg_transformation_time,
    AVG(broker_publish_time_ms) as avg_publish_time,
    MAX(transformation_time_ms) as max_transformation_time,
    MAX(broker_publish_time_ms) as max_publish_time
FROM webhook_logs
WHERE route_id = $1 AND processed_at >= $2;

-- name: GetRouteStatistics :one
SELECT 
    r.id,
    r.name,
    COUNT(w.id) as total_requests,
    COUNT(CASE WHEN w.status_code >= 200 AND w.status_code < 300 THEN 1 END) as successful_requests,
    COUNT(CASE WHEN w.status_code >= 400 THEN 1 END) as failed_requests,
    AVG(w.transformation_time_ms) as avg_transformation_time,
    AVG(w.broker_publish_time_ms) as avg_publish_time,
    MAX(w.processed_at) as last_processed
FROM routes r
LEFT JOIN webhook_logs w ON r.id = w.route_id
WHERE r.id = $1
GROUP BY r.id, r.name;

-- DLQ queries
-- name: CreateDLQMessage :one
INSERT INTO dlq_messages (
    message_id, route_id, trigger_id, pipeline_id, source_broker_id, dlq_broker_id,
    broker_name, queue, exchange, routing_key, headers, body, error_message,
    failure_count, first_failure, last_failure, next_retry, status, metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
RETURNING *;

-- name: GetDLQMessage :one
SELECT * FROM dlq_messages WHERE id = $1;

-- name: GetDLQMessageByMessageID :one
SELECT * FROM dlq_messages WHERE message_id = $1;

-- name: ListPendingDLQMessages :many
SELECT * FROM dlq_messages 
WHERE status = 'pending' AND next_retry <= CURRENT_TIMESTAMP
ORDER BY next_retry ASC
LIMIT $1;

-- name: ListDLQMessages :many
SELECT * FROM dlq_messages
ORDER BY last_failure DESC
LIMIT $1 OFFSET $2;

-- name: ListDLQMessagesByRoute :many
SELECT * FROM dlq_messages
WHERE route_id = $1
ORDER BY last_failure DESC
LIMIT $2 OFFSET $3;

-- name: ListDLQMessagesBySourceBroker :many
SELECT * FROM dlq_messages
WHERE source_broker_id = $1
ORDER BY last_failure DESC
LIMIT $2 OFFSET $3;

-- name: ListDLQMessagesByDLQBroker :many
SELECT * FROM dlq_messages
WHERE dlq_broker_id = $1
ORDER BY last_failure DESC
LIMIT $2 OFFSET $3;

-- name: ListDLQMessagesByStatus :many
SELECT * FROM dlq_messages
WHERE status = $1
ORDER BY last_failure DESC
LIMIT $2 OFFSET $3;

-- name: UpdateDLQMessage :one
UPDATE dlq_messages
SET failure_count = $2, last_failure = $3, next_retry = $4, 
    error_message = $5, status = $6, metadata = $7, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: UpdateDLQMessageStatus :exec
UPDATE dlq_messages
SET status = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: DeleteDLQMessage :exec
DELETE FROM dlq_messages WHERE id = $1;

-- name: DeleteOldDLQMessages :exec
DELETE FROM dlq_messages 
WHERE status = 'abandoned' AND first_failure < $1;

-- name: GetDLQStatsBySourceBroker :many
SELECT 
    source_broker_id,
    COUNT(*) as message_count,
    COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_count,
    COUNT(CASE WHEN status = 'abandoned' THEN 1 END) as abandoned_count
FROM dlq_messages
GROUP BY source_broker_id
ORDER BY message_count DESC;

-- name: GetDLQStatsByDLQBroker :many
SELECT 
    dlq_broker_id,
    COUNT(*) as message_count,
    COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_count,
    COUNT(CASE WHEN status = 'abandoned' THEN 1 END) as abandoned_count
FROM dlq_messages
GROUP BY dlq_broker_id
ORDER BY message_count DESC;