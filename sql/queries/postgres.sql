-- PostgreSQL-specific queries using $1, $2 syntax

-- Users queries
-- name: CreateUser :one
INSERT INTO users (id, username, password_hash, is_default)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: ListUsersPaginated :many
SELECT * FROM users ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountUsers :one
SELECT COUNT(*) as count FROM users;

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

-- name: ListSettingsPaginated :many
SELECT * FROM settings ORDER BY key
LIMIT $1 OFFSET $2;

-- name: CountSettings :one
SELECT COUNT(*) as count FROM settings;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = $1;

-- Triggers queries
-- name: CreateTrigger :one
INSERT INTO triggers (id, name, type, config, status, active, dlq_broker_id, dlq_enabled, dlq_retry_max)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetTrigger :one
SELECT * FROM triggers WHERE id = $1;

-- name: GetHTTPTriggerByUserPathMethod :one
SELECT * FROM triggers
WHERE user_id = $1
AND config->>'path' = $2
AND config->>'method' = $3
AND type = 'http'
AND active = true;

-- name: ListTriggers :many
SELECT * FROM triggers ORDER BY created_at DESC;

-- name: ListTriggersByUser :many
SELECT * FROM triggers WHERE user_id = $1 ORDER BY created_at DESC;

-- name: ListTriggersPaginated :many
SELECT * FROM triggers ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountTriggers :one
SELECT COUNT(*) as count FROM triggers;

-- name: UpdateTrigger :one
UPDATE triggers
SET name = $2, type = $3, config = $4, status = $5, active = $6,
    dlq_broker_id = $7, dlq_enabled = $8, dlq_retry_max = $9,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteTrigger :exec
DELETE FROM triggers WHERE id = $1;

-- Pipelines queries
-- name: CreatePipeline :one
INSERT INTO pipelines (id, name, description, stages, active)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPipeline :one
SELECT * FROM pipelines WHERE id = $1;

-- name: ListPipelines :many
SELECT * FROM pipelines ORDER BY created_at DESC;

-- name: ListPipelinesPaginated :many
SELECT * FROM pipelines ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPipelines :one
SELECT COUNT(*) as count FROM pipelines;

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
INSERT INTO broker_configs (id, name, type, config, active, health_status, dlq_enabled, dlq_broker_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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

-- name: ListBrokerConfigsPaginated :many
SELECT * FROM broker_configs ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountBrokerConfigs :one
SELECT COUNT(*) as count FROM broker_configs;

-- name: UpdateBrokerConfig :one
UPDATE broker_configs
SET name = $2, type = $3, config = $4, active = $5, dlq_enabled = $6, dlq_broker_id = $7,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteBrokerConfig :exec
DELETE FROM broker_configs WHERE id = $1;


-- DLQ queries
-- name: CreateDLQMessage :one
INSERT INTO dlq_messages (
    id, message_id, trigger_id, pipeline_id, source_broker_id, dlq_broker_id,
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

-- name: CountDLQMessages :one
SELECT COUNT(*) as count FROM dlq_messages;


-- name: CountDLQMessagesBySourceBroker :one
SELECT COUNT(*) as count FROM dlq_messages WHERE source_broker_id = $1;

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

-- Dashboard time series and stats queries

-- name: CountDLQMessagesAddedSince :one
SELECT COUNT(*) as count
FROM dlq_messages
WHERE created_at >= $1;

-- name: CountDLQMessagesAddedSinceForUser :one
SELECT COUNT(*) as count
FROM dlq_messages dm
INNER JOIN triggers t ON dm.trigger_id = t.id
WHERE dm.created_at >= $1 AND t.user_id = $2;

-- name: CountDLQMessagesResolvedSince :one
SELECT COUNT(*) as count
FROM dlq_messages
WHERE updated_at >= $1 AND status != 'pending';

-- name: CountDLQMessagesResolvedSinceForUser :one
SELECT COUNT(*) as count
FROM dlq_messages dm
INNER JOIN triggers t ON dm.trigger_id = t.id
WHERE dm.updated_at >= $1 AND dm.status != 'pending' AND t.user_id = $2;





-- PostgreSQL versions
-- name: GetExecutionLogStatsForUserPG :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2;

-- name: GetAverageLatencyForUserPG :one
SELECT AVG(total_latency_ms)::FLOAT as avg_latency
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2 AND started_at <= $3 AND status = 'success';

-- name: GetRecentExecutionLogsForUserPG :many
SELECT *
FROM execution_logs
WHERE user_id = $1
ORDER BY started_at DESC
LIMIT $2;

-- name: GetExecutionLogStatsForUserAndTriggerPG :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2 AND trigger_id = $3;

-- name: GetAverageLatencyForUserAndTriggerPG :one
SELECT AVG(total_latency_ms)::FLOAT as avg_latency
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2 AND started_at <= $3 AND status = 'success' AND trigger_id = $4;

-- name: GetRecentExecutionLogsForUserAndTriggerPG :many
SELECT *
FROM execution_logs
WHERE user_id = $1 AND trigger_id = $2
ORDER BY started_at DESC
LIMIT $3;

-- name: GetExecutionLogTimeSeriesHourlyForUserPG :many
SELECT 
    date_trunc('hour', started_at)::TIMESTAMP as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 
    AND started_at >= $2 
    AND started_at <= $3
    AND ($4 = 'all' OR trigger_id = $5)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetExecutionLogTimeSeriesDailyForUserPG :many
SELECT 
    date_trunc('day', started_at)::TIMESTAMP as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 
    AND started_at >= $2 
    AND started_at <= $3
    AND ($4 = 'all' OR trigger_id = $5)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetExecutionLogTimeSeriesWeeklyForUserPG :many
SELECT 
    date_trunc('week', started_at)::TIMESTAMP as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 
    AND started_at >= $2 
    AND started_at <= $3
    AND ($4 = 'all' OR trigger_id = $5)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetTopTriggersByRequestsForUserPG :many
SELECT 
    t.id,
    t.name,
    t.type,
    t.active,
    COUNT(el.id) as total_requests,
    COUNT(CASE WHEN el.status = 'success' THEN 1 END) as successful_requests,
    AVG(CASE WHEN el.status = 'success' THEN el.total_latency_ms END)::FLOAT as avg_latency,
    MAX(el.started_at) as last_processed
FROM triggers t
LEFT JOIN execution_logs el ON t.id = el.trigger_id AND el.started_at >= $1
WHERE t.user_id = $2 AND t.deleted_at IS NULL
GROUP BY t.id, t.name, t.type, t.active
ORDER BY total_requests DESC
LIMIT $3;

-- Stats queries for backward compatibility
-- name: GetWebhookLogStats :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE started_at >= $1;

-- name: GetTriggerStatistics :one
SELECT 
    COUNT(*) as total_requests,
    COUNT(CASE WHEN el.status = 'success' THEN 1 END) as successful_requests,
    COUNT(CASE WHEN el.status = 'error' THEN 1 END) as failed_requests,
    AVG(CASE WHEN el.status = 'success' THEN el.total_latency_ms END)::FLOAT as avg_transformation_time,
    AVG(el.total_latency_ms)::FLOAT as avg_total_time,
    MAX(el.started_at) as last_processed,
    t.name as name
FROM execution_logs el
JOIN triggers t ON el.trigger_id = t.id
WHERE el.trigger_id = $1
GROUP BY t.name;

-- name: ListDLQMessagesByTrigger :many
SELECT * FROM dlq_messages
WHERE trigger_id = $1
ORDER BY last_failure DESC
LIMIT $2 OFFSET $3;

-- name: CountDLQMessagesByTrigger :one
SELECT COUNT(*) as count FROM dlq_messages WHERE trigger_id = $1;

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
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetOAuth2Service :one
SELECT * FROM oauth2_services WHERE id = $1;

-- name: GetOAuth2ServiceByName :one
SELECT * FROM oauth2_services WHERE name = $1 AND user_id = $2;

-- name: ListOAuth2Services :many
SELECT * FROM oauth2_services WHERE user_id = $1 ORDER BY created_at DESC;

-- name: UpdateOAuth2Service :one
UPDATE oauth2_services
SET name = $2, client_id = $3, client_secret = $4, token_url = $5,
    auth_url = $6, redirect_url = $7, scopes = $8, grant_type = $9, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING *;

-- name: DeleteOAuth2Service :exec
DELETE FROM oauth2_services WHERE id = $1;
