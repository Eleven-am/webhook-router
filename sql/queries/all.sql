-- Users queries
-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, is_default)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: ListUsersPaginated :many
SELECT * FROM users ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountUsers :one
SELECT COUNT(*) as count FROM users;

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

-- name: ListSettingsPaginated :many
SELECT * FROM settings ORDER BY key
LIMIT ? OFFSET ?;

-- name: CountSettings :one
SELECT COUNT(*) as count FROM settings;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = ?;

-- Triggers queries
-- name: CreateTrigger :one
INSERT INTO triggers (id, name, type, config, status, active, dlq_broker_id, dlq_enabled, dlq_retry_max)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTrigger :one
SELECT * FROM triggers WHERE id = ?;

-- name: GetHTTPTriggerByUserPathMethod :one
SELECT * FROM triggers
WHERE user_id = ?
AND json_extract(config, '$.path') = ?
AND json_extract(config, '$.method') = ?
AND type = 'http'
AND active = 1;

-- name: ListTriggers :many
SELECT * FROM triggers ORDER BY created_at DESC;

-- name: ListTriggersByUser :many
SELECT * FROM triggers WHERE user_id = ? ORDER BY created_at DESC;

-- name: ListTriggersPaginated :many
SELECT * FROM triggers ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountTriggers :one
SELECT COUNT(*) as count FROM triggers;

-- name: UpdateTrigger :one
UPDATE triggers
SET name = ?, type = ?, config = ?, status = ?, active = ?,
    dlq_broker_id = ?, dlq_enabled = ?, dlq_retry_max = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteTrigger :exec
DELETE FROM triggers WHERE id = ?;

-- Pipelines queries
-- name: CreatePipeline :one
INSERT INTO pipelines (id, name, description, stages, active)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPipeline :one
SELECT * FROM pipelines WHERE id = ?;

-- name: ListPipelines :many
SELECT * FROM pipelines ORDER BY created_at DESC;

-- name: ListPipelinesPaginated :many
SELECT * FROM pipelines ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountPipelines :one
SELECT COUNT(*) as count FROM pipelines;

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
INSERT INTO broker_configs (id, name, type, config, active, health_status, dlq_enabled, dlq_broker_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
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

-- name: ListBrokerConfigsPaginated :many
SELECT * FROM broker_configs ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountBrokerConfigs :one
SELECT COUNT(*) as count FROM broker_configs;

-- name: UpdateBrokerConfig :one
UPDATE broker_configs
SET name = ?, type = ?, config = ?, active = ?, dlq_enabled = ?, dlq_broker_id = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteBrokerConfig :exec
DELETE FROM broker_configs WHERE id = ?;


-- DLQ queries
-- name: CreateDLQMessage :one
INSERT INTO dlq_messages (
    id, message_id, trigger_id, pipeline_id, source_broker_id, dlq_broker_id,
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

-- name: CountDLQMessages :one
SELECT COUNT(*) as count FROM dlq_messages;


-- name: CountDLQMessagesBySourceBroker :one
SELECT COUNT(*) as count FROM dlq_messages WHERE source_broker_id = ?;

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

-- Dashboard time series queries

-- name: CountDLQMessagesAddedSince :one
SELECT COUNT(*) as count
FROM dlq_messages
WHERE created_at >= ?;

-- name: CountDLQMessagesResolvedSince :one
SELECT COUNT(*) as count
FROM dlq_messages
WHERE updated_at >= ? AND status != 'pending';

-- name: CountDLQMessagesAddedSinceForUser :one
SELECT COUNT(*) as count
FROM dlq_messages dm
INNER JOIN triggers t ON dm.trigger_id = t.id
WHERE dm.created_at >= ? AND t.user_id = ?;

-- name: CountDLQMessagesResolvedSinceForUser :one
SELECT COUNT(*) as count
FROM dlq_messages dm
INNER JOIN triggers t ON dm.trigger_id = t.id
WHERE dm.updated_at >= ? AND dm.status != 'pending' AND t.user_id = ?;

-- Execution logs queries
-- Execution logs queries for dashboard and analytics

-- name: GetExecutionLogStatsForUser :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? AND started_at >= ?;

-- name: GetAverageLatencyForUser :one
SELECT AVG(total_latency_ms) as avg_latency
FROM execution_logs
WHERE user_id = ? AND started_at >= ? AND started_at <= ? AND status = 'success';

-- name: GetRecentExecutionLogsForUser :many
SELECT *
FROM execution_logs
WHERE user_id = ?
ORDER BY started_at DESC
LIMIT ?;

-- name: GetExecutionLogStatsForUserAndTrigger :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? AND started_at >= ? AND trigger_id = ?;

-- name: GetAverageLatencyForUserAndTrigger :one
SELECT AVG(total_latency_ms) as avg_latency
FROM execution_logs
WHERE user_id = ? AND started_at >= ? AND started_at <= ? AND status = 'success' AND trigger_id = ?;

-- name: GetRecentExecutionLogsForUserAndTrigger :many
SELECT *
FROM execution_logs
WHERE user_id = ? AND trigger_id = ?
ORDER BY started_at DESC
LIMIT ?;

-- name: GetExecutionLogTimeSeriesHourlyForUser :many
SELECT 
    strftime('%Y-%m-%d %H:00:00', started_at) as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? 
    AND started_at >= ? 
    AND started_at <= ?
    AND (? = 'all' OR trigger_id = ?)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetExecutionLogTimeSeriesDailyForUser :many
SELECT 
    strftime('%Y-%m-%d', started_at) as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? 
    AND started_at >= ? 
    AND started_at <= ?
    AND (? = 'all' OR trigger_id = ?)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetExecutionLogTimeSeriesWeeklyForUser :many
SELECT 
    strftime('%Y-W%W', started_at) as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? 
    AND started_at >= ? 
    AND started_at <= ?
    AND (? = 'all' OR trigger_id = ?)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetTopTriggersByRequestsForUser :many
SELECT 
    t.id,
    t.name,
    t.type,
    t.active,
    COUNT(el.id) as total_requests,
    COUNT(CASE WHEN el.status = 'success' THEN 1 END) as successful_requests,
    AVG(CASE WHEN el.status = 'success' THEN el.total_latency_ms END) as avg_latency,
    MAX(el.started_at) as last_processed
FROM triggers t
LEFT JOIN execution_logs el ON t.id = el.trigger_id AND el.started_at >= ?
WHERE t.user_id = ? AND t.deleted_at IS NULL
GROUP BY t.id, t.name, t.type, t.active
ORDER BY total_requests DESC
LIMIT ?;

-- Stats queries for backward compatibility
-- name: GetWebhookLogStats :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE started_at >= ?;

-- name: GetTriggerStatistics :one
SELECT 
    COUNT(*) as total_requests,
    COUNT(CASE WHEN el.status = 'success' THEN 1 END) as successful_requests,
    COUNT(CASE WHEN el.status = 'error' THEN 1 END) as failed_requests,
    AVG(CASE WHEN el.status = 'success' THEN el.total_latency_ms END) as avg_transformation_time,
    AVG(el.total_latency_ms) as avg_total_time,
    MAX(el.started_at) as last_processed,
    t.name as name
FROM execution_logs el
JOIN triggers t ON el.trigger_id = t.id
WHERE el.trigger_id = ?
GROUP BY t.name;

-- name: ListDLQMessagesByTrigger :many
SELECT * FROM dlq_messages
WHERE trigger_id = ?
ORDER BY last_failure DESC
LIMIT ? OFFSET ?;

-- name: CountDLQMessagesByTrigger :one
SELECT COUNT(*) as count FROM dlq_messages WHERE trigger_id = ?;

-- name: GetDLQStatsByTrigger :many
SELECT 
    trigger_id,
    COUNT(*) as message_count,
    COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_count,
    COUNT(CASE WHEN status = 'abandoned' THEN 1 END) as abandoned_count
FROM dlq_messages
WHERE trigger_id IS NOT NULL
GROUP BY trigger_id
ORDER BY message_count DESC;

-- OAuth2 Services queries
-- name: CreateOAuth2Service :one
INSERT INTO oauth2_services (
    id, name, client_id, client_secret, token_url, 
    auth_url, redirect_url, scopes, grant_type, user_id
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOAuth2Service :one
SELECT * FROM oauth2_services WHERE id = ?;

-- name: GetOAuth2ServiceByName :one
SELECT * FROM oauth2_services WHERE name = ? AND user_id = ?;

-- name: ListOAuth2Services :many
SELECT * FROM oauth2_services WHERE user_id = ? ORDER BY created_at DESC;

-- name: UpdateOAuth2Service :one
UPDATE oauth2_services
SET name = ?, client_id = ?, client_secret = ?, token_url = ?,
    auth_url = ?, redirect_url = ?, scopes = ?, grant_type = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: DeleteOAuth2Service :exec
DELETE FROM oauth2_services WHERE id = ?;


