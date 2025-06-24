-- Execution logs queries for dashboard and analytics

-- name: GetExecutionLogStatsForUser :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? AND started_at >= ?;

-- name: GetExecutionLogStatsForUserAndRoute :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = ? AND started_at >= ? AND route_id = ?;

-- name: GetAverageLatencyForUser :one
SELECT AVG(total_latency_ms) as avg_latency
FROM execution_logs
WHERE user_id = ? AND started_at >= ? AND started_at <= ? AND status = 'success';

-- name: GetAverageLatencyForUserAndRoute :one
SELECT AVG(total_latency_ms) as avg_latency
FROM execution_logs
WHERE user_id = ? AND started_at >= ? AND started_at <= ? AND status = 'success' AND route_id = ?;

-- name: GetRecentExecutionLogsForUser :many
SELECT *
FROM execution_logs
WHERE user_id = ?
ORDER BY started_at DESC
LIMIT ?;

-- name: GetRecentExecutionLogsForUserAndRoute :many
SELECT *
FROM execution_logs
WHERE user_id = ? AND route_id = ?
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
    AND (? = 'all' OR route_id = ?)
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
    AND (? = 'all' OR route_id = ?)
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
    AND (? = 'all' OR route_id = ?)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetTopRoutesByRequestsForUser :many
SELECT 
    r.id,
    r.name,
    r.endpoint,
    r.active,
    COUNT(el.id) as total_requests,
    COUNT(CASE WHEN el.status = 'success' THEN 1 END) as successful_requests,
    AVG(CASE WHEN el.status = 'success' THEN el.total_latency_ms END) as avg_latency,
    MAX(el.started_at) as last_processed
FROM routes r
LEFT JOIN execution_logs el ON r.id = el.route_id AND el.started_at >= ?
WHERE r.user_id = ? AND r.deleted_at IS NULL
GROUP BY r.id, r.name, r.endpoint, r.active
ORDER BY total_requests DESC
LIMIT ?;

-- PostgreSQL versions
-- name: GetExecutionLogStatsForUserPG :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2;

-- name: GetExecutionLogStatsForUserAndRoutePG :one
SELECT 
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'success' THEN 1 END) as success_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2 AND route_id = $3;

-- name: GetAverageLatencyForUserPG :one
SELECT AVG(total_latency_ms)::FLOAT as avg_latency
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2 AND started_at <= $3 AND status = 'success';

-- name: GetAverageLatencyForUserAndRoutePG :one
SELECT AVG(total_latency_ms)::FLOAT as avg_latency
FROM execution_logs
WHERE user_id = $1 AND started_at >= $2 AND started_at <= $3 AND status = 'success' AND route_id = $4;

-- name: GetRecentExecutionLogsForUserPG :many
SELECT *
FROM execution_logs
WHERE user_id = $1
ORDER BY started_at DESC
LIMIT $2;

-- name: GetRecentExecutionLogsForUserAndRoutePG :many
SELECT *
FROM execution_logs
WHERE user_id = $1 AND route_id = $2
ORDER BY started_at DESC
LIMIT $3;

-- name: GetExecutionLogTimeSeriesHourlyForUserPG :many
SELECT 
    date_trunc('hour', started_at) as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 
    AND started_at >= $2 
    AND started_at <= $3
    AND ($4 = 'all' OR route_id = $5)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetExecutionLogTimeSeriesDailyForUserPG :many
SELECT 
    date_trunc('day', started_at) as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 
    AND started_at >= $2 
    AND started_at <= $3
    AND ($4 = 'all' OR route_id = $5)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetExecutionLogTimeSeriesWeeklyForUserPG :many
SELECT 
    date_trunc('week', started_at) as time_bucket,
    COUNT(*) as total_count,
    COUNT(CASE WHEN status = 'error' THEN 1 END) as error_count
FROM execution_logs
WHERE user_id = $1 
    AND started_at >= $2 
    AND started_at <= $3
    AND ($4 = 'all' OR route_id = $5)
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: GetTopRoutesByRequestsForUserPG :many
SELECT 
    r.id,
    r.name,
    r.endpoint,
    r.active,
    COUNT(el.id) as total_requests,
    COUNT(CASE WHEN el.status = 'success' THEN 1 END) as successful_requests,
    AVG(CASE WHEN el.status = 'success' THEN el.total_latency_ms END)::FLOAT as avg_latency,
    MAX(el.started_at) as last_processed
FROM routes r
LEFT JOIN execution_logs el ON r.id = el.route_id AND el.started_at >= $1
WHERE r.user_id = $2 AND r.deleted_at IS NULL
GROUP BY r.id, r.name, r.endpoint, r.active
ORDER BY total_requests DESC
LIMIT $3;