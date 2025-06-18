-- Users queries
-- name: CreateUser :one
INSERT INTO users (username, password_hash, is_default)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: UpdateUser :one
UPDATE users 
SET username = ?, password_hash = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: UpdateUserCredentials :exec
UPDATE users 
SET username = ?, password_hash = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- Settings queries
-- name: SetSetting :exec
INSERT OR REPLACE INTO settings (key, value, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP);

-- name: GetSetting :one
SELECT * FROM settings WHERE key = ?;

-- name: ListSettings :many
SELECT * FROM settings ORDER BY key;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = ?;

-- Routes queries
-- name: CreateRoute :one
INSERT INTO routes (
    name, endpoint, method, queue, exchange, routing_key,
    filters, headers, active, pipeline_id, trigger_id,
    destination_broker_id, priority, condition_expression,
    signature_config, signature_secret
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRoute :one
SELECT * FROM routes WHERE id = ?;

-- name: GetRouteByName :one
SELECT * FROM routes WHERE name = ?;

-- name: GetRouteByEndpoint :one
SELECT * FROM routes WHERE endpoint = ? AND method = ?;

-- name: ListRoutes :many
SELECT * FROM routes ORDER BY priority ASC, created_at DESC;

-- name: ListActiveRoutes :many
SELECT * FROM routes WHERE active = 1 ORDER BY priority ASC, created_at DESC;

-- name: UpdateRoute :one
UPDATE routes
SET name = ?, endpoint = ?, method = ?, queue = ?, exchange = ?,
    routing_key = ?, filters = ?, headers = ?, active = ?,
    pipeline_id = ?, trigger_id = ?, destination_broker_id = ?,
    priority = ?, condition_expression = ?, signature_config = ?,
    signature_secret = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteRoute :exec
DELETE FROM routes WHERE id = ?;

-- Triggers queries
-- name: CreateTrigger :one
INSERT INTO triggers (name, type, config, status, active)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTrigger :one
SELECT * FROM triggers WHERE id = ?;

-- name: ListTriggers :many
SELECT * FROM triggers ORDER BY created_at DESC;

-- name: UpdateTrigger :one
UPDATE triggers
SET name = ?, type = ?, config = ?, status = ?, active = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteTrigger :exec
DELETE FROM triggers WHERE id = ?;

-- Pipelines queries
-- name: CreatePipeline :one
INSERT INTO pipelines (name, description, stages, active)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetPipeline :one
SELECT * FROM pipelines WHERE id = ?;

-- name: ListPipelines :many
SELECT * FROM pipelines ORDER BY created_at DESC;

-- name: UpdatePipeline :one
UPDATE pipelines
SET name = ?, description = ?, stages = ?, active = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeletePipeline :exec
DELETE FROM pipelines WHERE id = ?;

-- Broker configs queries
-- name: CreateBrokerConfig :one
INSERT INTO broker_configs (name, type, config, active, health_status, dlq_enabled, dlq_broker_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBrokerConfig :one
SELECT * FROM broker_configs WHERE id = ?;

-- name: GetBrokerConfigWithDLQ :one
SELECT 
    bc.*,
    dlq.id as dlq_id,
    dlq.name as dlq_name,
    dlq.type as dlq_type,
    dlq.config as dlq_config
FROM broker_configs bc
LEFT JOIN broker_configs dlq ON bc.dlq_broker_id = dlq.id
WHERE bc.id = ?;

-- name: ListBrokerConfigs :many
SELECT * FROM broker_configs ORDER BY created_at DESC;

-- name: UpdateBrokerConfig :one
UPDATE broker_configs
SET name = ?, type = ?, config = ?, active = ?, dlq_enabled = ?, dlq_broker_id = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteBrokerConfig :exec
DELETE FROM broker_configs WHERE id = ?;

-- Webhook logs queries
-- name: CreateWebhookLog :one
INSERT INTO webhook_logs (
    route_id, method, endpoint, headers, body, status_code,
    error, trigger_id, pipeline_id, transformation_time_ms,
    broker_publish_time_ms
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWebhookLog :one
SELECT * FROM webhook_logs WHERE id = ?;

-- name: ListWebhookLogs :many
SELECT * FROM webhook_logs ORDER BY processed_at DESC LIMIT ? OFFSET ?;

-- name: DeleteOldWebhookLogs :exec
DELETE FROM webhook_logs WHERE processed_at < ?;

-- name: GetWebhookLogsByRouteID :many
SELECT * FROM webhook_logs 
WHERE route_id = ?
ORDER BY processed_at DESC 
LIMIT ? OFFSET ?;

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
WHERE processed_at >= ?;

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
WHERE route_id = ? AND processed_at >= ?;

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
WHERE r.id = ?
GROUP BY r.id, r.name;

-- DLQ queries
-- name: CreateDLQMessage :one
INSERT INTO dlq_messages (
    message_id, route_id, trigger_id, pipeline_id, source_broker_id, dlq_broker_id,
    broker_name, queue, exchange, routing_key, headers, body, error_message,
    failure_count, first_failure, last_failure, next_retry, status, metadata
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDLQMessage :one
SELECT * FROM dlq_messages WHERE id = ?;

-- name: GetDLQMessageByMessageID :one
SELECT * FROM dlq_messages WHERE message_id = ?;

-- name: ListPendingDLQMessages :many
SELECT * FROM dlq_messages 
WHERE status = 'pending' AND next_retry <= CURRENT_TIMESTAMP
ORDER BY next_retry ASC
LIMIT ?;

-- name: ListDLQMessages :many
SELECT * FROM dlq_messages
ORDER BY last_failure DESC
LIMIT ? OFFSET ?;

-- name: ListDLQMessagesByRoute :many
SELECT * FROM dlq_messages
WHERE route_id = ?
ORDER BY last_failure DESC
LIMIT ? OFFSET ?;

-- name: ListDLQMessagesBySourceBroker :many
SELECT * FROM dlq_messages
WHERE source_broker_id = ?
ORDER BY last_failure DESC
LIMIT ? OFFSET ?;

-- name: ListDLQMessagesByDLQBroker :many
SELECT * FROM dlq_messages
WHERE dlq_broker_id = ?
ORDER BY last_failure DESC
LIMIT ? OFFSET ?;

-- name: ListDLQMessagesByStatus :many
SELECT * FROM dlq_messages
WHERE status = ?
ORDER BY last_failure DESC
LIMIT ? OFFSET ?;

-- name: UpdateDLQMessage :one
UPDATE dlq_messages
SET failure_count = ?, last_failure = ?, next_retry = ?, 
    error_message = ?, status = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: UpdateDLQMessageStatus :exec
UPDATE dlq_messages
SET status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteDLQMessage :exec
DELETE FROM dlq_messages WHERE id = ?;

-- name: DeleteOldDLQMessages :exec
DELETE FROM dlq_messages 
WHERE status = 'abandoned' AND first_failure < ?;

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