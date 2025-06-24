package sqlc

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/generated/postgres"
)

// Helper methods for GetDashboardStats - PostgreSQL implementation

func (s *PostgreSQLCAdapter) getActiveTriggersMetric(ctx context.Context, userID string, currentStart, previousStart time.Time) (storage.MetricWithChange, error) {
	// Get current active triggers count for user
	triggers, err := s.queries.ListTriggersByUser(ctx, userID)
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	currentActive := 0
	for _, trigger := range triggers {
		if trigger.Active != nil && *trigger.Active && !trigger.DeletedAt.Valid {
			currentActive++
		}
	}

	// Count triggers created/activated in current period
	newTriggers := 0
	deactivated := 0
	for _, trigger := range triggers {
		if trigger.CreatedAt.Time.After(currentStart) && trigger.Active != nil && *trigger.Active {
			newTriggers++
		}
		if trigger.UpdatedAt.Valid && trigger.UpdatedAt.Time.After(currentStart) && (trigger.Active == nil || !*trigger.Active) {
			deactivated++
		}
	}

	netChange := float64(newTriggers - deactivated)
	previousActive := float64(currentActive) - netChange

	changePercent := 0.0
	if previousActive > 0 {
		changePercent = (netChange / previousActive) * 100
	}

	return storage.MetricWithChange{
		Current:       float64(currentActive),
		Previous:      previousActive,
		Change:        netChange,
		ChangePercent: changePercent,
	}, nil
}

func (s *PostgreSQLCAdapter) getRequestMetrics(ctx context.Context, userID string, currentStart, previousStart, now time.Time, triggerID string) (*requestMetricsResult, error) {
	var currentTotal, currentSuccess int64
	var previousTotal, previousSuccess int64
	var err error

	// Get current period stats for user
	if triggerID != "" {
		stats, err := s.queries.GetExecutionLogStatsForUserAndTriggerPG(ctx, postgres.GetExecutionLogStatsForUserAndTriggerPGParams{
			UserID:    userID,
			StartedAt: pgtype.Timestamp{Time: currentStart, Valid: true},
			TriggerID: &triggerID,
		})
		if err != nil {
			return nil, err
		}
		currentTotal = stats.TotalCount
		currentSuccess = stats.SuccessCount
	} else {
		stats, err := s.queries.GetExecutionLogStatsForUserPG(ctx, postgres.GetExecutionLogStatsForUserPGParams{
			UserID:    userID,
			StartedAt: pgtype.Timestamp{Time: currentStart, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		currentTotal = stats.TotalCount
		currentSuccess = stats.SuccessCount
	}

	// Get previous period stats for user
	if triggerID != "" {
		stats, err := s.queries.GetExecutionLogStatsForUserAndTriggerPG(ctx, postgres.GetExecutionLogStatsForUserAndTriggerPGParams{
			UserID:    userID,
			StartedAt: pgtype.Timestamp{Time: previousStart, Valid: true},
			TriggerID: &triggerID,
		})
		if err != nil {
			return nil, err
		}
		previousTotal = stats.TotalCount
		previousSuccess = stats.SuccessCount
	} else {
		stats, err := s.queries.GetExecutionLogStatsForUserPG(ctx, postgres.GetExecutionLogStatsForUserPGParams{
			UserID:    userID,
			StartedAt: pgtype.Timestamp{Time: previousStart, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		previousTotal = stats.TotalCount
		previousSuccess = stats.SuccessCount
	}

	// Calculate total requests metric
	currentTotalFloat := float64(currentTotal)
	previousTotalFloat := float64(previousTotal) - currentTotalFloat // Previous period includes current
	if previousTotalFloat < 0 {
		previousTotalFloat = 0
	}

	totalChange := currentTotalFloat - previousTotalFloat
	totalChangePercent := 0.0
	if previousTotalFloat > 0 {
		totalChangePercent = (totalChange / previousTotalFloat) * 100
	}

	// Calculate success rate metric
	currentSuccessRate := 0.0
	if currentTotal > 0 {
		currentSuccessRate = (float64(currentSuccess) / float64(currentTotal)) * 100
	}

	previousSuccessRate := 0.0
	if previousTotal > 0 {
		previousSuccessRate = (float64(previousSuccess) / float64(previousTotal)) * 100
	}

	successRateChange := currentSuccessRate - previousSuccessRate

	// Get average latency for current period for user
	var currentLatency, previousLatency pgtype.Float8

	if triggerID != "" {
		tempLatency, err := s.queries.GetAverageLatencyForUserAndTriggerPG(ctx, postgres.GetAverageLatencyForUserAndTriggerPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: currentStart, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: now, Valid: true},
			TriggerID:   &triggerID,
		})
		if err == nil {
			currentLatency = pgtype.Float8{Float64: tempLatency, Valid: true}
		}
	} else {
		tempLatency, err := s.queries.GetAverageLatencyForUserPG(ctx, postgres.GetAverageLatencyForUserPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: currentStart, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: now, Valid: true},
		})
		if err == nil {
			currentLatency = pgtype.Float8{Float64: tempLatency, Valid: true}
		}
	}
	// Ignore not found errors, just use 0
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	// Get average latency for previous period for user
	if triggerID != "" {
		tempLatency2, err := s.queries.GetAverageLatencyForUserAndTriggerPG(ctx, postgres.GetAverageLatencyForUserAndTriggerPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: previousStart, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: currentStart, Valid: true},
			TriggerID:   &triggerID,
		})
		if err == nil {
			previousLatency = pgtype.Float8{Float64: tempLatency2, Valid: true}
		}
	} else {
		tempLatency2, err := s.queries.GetAverageLatencyForUserPG(ctx, postgres.GetAverageLatencyForUserPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: previousStart, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: currentStart, Valid: true},
		})
		if err == nil {
			previousLatency = pgtype.Float8{Float64: tempLatency2, Valid: true}
		}
	}
	// Ignore not found errors, just use 0
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	currentLatencyVal := 0.0
	if currentLatency.Valid {
		currentLatencyVal = currentLatency.Float64
	}

	previousLatencyVal := 0.0
	if previousLatency.Valid {
		previousLatencyVal = previousLatency.Float64
	}

	latencyChange := currentLatencyVal - previousLatencyVal
	latencyChangePercent := 0.0
	if previousLatencyVal > 0 {
		latencyChangePercent = (latencyChange / previousLatencyVal) * 100
	}

	return &requestMetricsResult{
		totalRequests: storage.MetricWithChange{
			Current:       float64(currentTotal),
			Previous:      float64(previousTotal),
			Change:        totalChange,
			ChangePercent: totalChangePercent,
		},
		successRate: storage.MetricWithChange{
			Current:       currentSuccessRate,
			Previous:      previousSuccessRate,
			Change:        successRateChange,
			ChangePercent: successRateChange, // For percentages, absolute change is the same
		},
		avgLatency: storage.MetricWithChange{
			Current:       currentLatencyVal,
			Previous:      previousLatencyVal,
			Change:        latencyChange,
			ChangePercent: latencyChangePercent,
		},
	}, nil
}

func (s *PostgreSQLCAdapter) getDLQMetric(ctx context.Context, userID string, periodStart time.Time) (storage.MetricWithChange, error) {
	// Get current DLQ count for user
	// Call the method directly on PostgreSQLCAdapter
	stats, err := s.GetDLQStatsForUser(userID)
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	currentCount := float64(stats.PendingMessages)

	// Count DLQ messages added in period for user using SQLC
	addedResult, err := s.queries.CountDLQMessagesAddedSinceForUser(ctx, postgres.CountDLQMessagesAddedSinceForUserParams{
		CreatedAt: pgtype.Timestamp{Time: periodStart, Valid: true},
		UserID:    userID,
	})
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	// Count DLQ messages resolved in period for user using SQLC
	resolvedResult, err := s.queries.CountDLQMessagesResolvedSinceForUser(ctx, postgres.CountDLQMessagesResolvedSinceForUserParams{
		UpdatedAt: pgtype.Timestamp{Time: periodStart, Valid: true},
		UserID:    userID,
	})
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	netChange := float64(addedResult - resolvedResult)
	previousCount := currentCount - netChange
	if previousCount < 0 {
		previousCount = 0
	}

	changePercent := 0.0
	if previousCount > 0 {
		changePercent = (netChange / previousCount) * 100
	}

	return storage.MetricWithChange{
		Current:       currentCount,
		Previous:      previousCount,
		Change:        netChange,
		ChangePercent: changePercent,
	}, nil
}

func (s *PostgreSQLCAdapter) getTimeSeries(ctx context.Context, userID string, start, end time.Time, triggerID string) ([]storage.TimeSeriesPoint, error) {
	// Determine interval based on time range
	duration := end.Sub(start)

	var points []storage.TimeSeriesPoint

	routeFilter := triggerID
	if routeFilter == "" {
		routeFilter = "all"
	}

	if duration <= 24*time.Hour {
		// Hourly aggregation
		params := postgres.GetExecutionLogTimeSeriesHourlyForUserPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: start, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: end, Valid: true},
			Column4:     routeFilter,
			TriggerID: func() *string {
				if routeFilter == "all" {
					return nil
				}
				return &routeFilter
			}(),
		}
		rows, err := s.queries.GetExecutionLogTimeSeriesHourlyForUserPG(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert rows to points
		for _, row := range rows {
			timestamp := time.Time{}
			if row.TimeBucket.Valid {
				timestamp = row.TimeBucket.Time
			}
			points = append(points, storage.TimeSeriesPoint{
				Timestamp: timestamp,
				Requests:  int(row.TotalCount),
				Errors:    int(row.ErrorCount),
			})
		}
	} else if duration <= 7*24*time.Hour {
		// Daily aggregation
		params := postgres.GetExecutionLogTimeSeriesDailyForUserPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: start, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: end, Valid: true},
			Column4:     routeFilter,
			TriggerID: func() *string {
				if routeFilter == "all" {
					return nil
				}
				return &routeFilter
			}(),
		}
		dailyRows, err := s.queries.GetExecutionLogTimeSeriesDailyForUserPG(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert daily rows to points
		for _, row := range dailyRows {
			timestamp := time.Time{}
			if row.TimeBucket.Valid {
				timestamp = row.TimeBucket.Time
			}
			points = append(points, storage.TimeSeriesPoint{
				Timestamp: timestamp,
				Requests:  int(row.TotalCount),
				Errors:    int(row.ErrorCount),
			})
		}
	} else {
		// Weekly aggregation
		params := postgres.GetExecutionLogTimeSeriesWeeklyForUserPGParams{
			UserID:      userID,
			StartedAt:   pgtype.Timestamp{Time: start, Valid: true},
			StartedAt_2: pgtype.Timestamp{Time: end, Valid: true},
			Column4:     routeFilter,
			TriggerID: func() *string {
				if routeFilter == "all" {
					return nil
				}
				return &routeFilter
			}(),
		}
		weeklyRows, err := s.queries.GetExecutionLogTimeSeriesWeeklyForUserPG(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert weekly rows to points
		for _, row := range weeklyRows {
			timestamp := time.Time{}
			if row.TimeBucket.Valid {
				timestamp = row.TimeBucket.Time
			}
			points = append(points, storage.TimeSeriesPoint{
				Timestamp: timestamp,
				Requests:  int(row.TotalCount),
				Errors:    int(row.ErrorCount),
			})
		}
	}

	return points, nil
}

func (s *PostgreSQLCAdapter) getTopTriggers(ctx context.Context, userID string, start, end time.Time) ([]storage.TriggerStatsSummary, error) {
	// Get top triggers by request volume for user using SQLC
	params := postgres.GetTopTriggersByRequestsForUserPGParams{
		StartedAt: pgtype.Timestamp{Time: start, Valid: true},
		UserID:    userID,
		Limit:     10,
	}

	rows, err := s.queries.GetTopTriggersByRequestsForUserPG(ctx, params)
	if err != nil {
		return nil, err
	}

	triggers := make([]storage.TriggerStatsSummary, len(rows))
	for i, row := range rows {
		successRate := 0.0
		if row.TotalRequests > 0 {
			successRate = (float64(row.SuccessfulRequests) / float64(row.TotalRequests)) * 100
		}

		avgLatency := row.AvgLatency

		var lastProcessed *time.Time
		if row.LastProcessed != nil {
			switch v := row.LastProcessed.(type) {
			case pgtype.Timestamp:
				if v.Valid {
					lastProcessed = &v.Time
				}
			case time.Time:
				lastProcessed = &v
			}
		}

		triggers[i] = storage.TriggerStatsSummary{
			ID:            row.ID,
			Name:          row.Name,
			Active:        row.Active != nil && *row.Active,
			TotalRequests: int(row.TotalRequests),
			SuccessRate:   successRate,
			AvgLatencyMs:  avgLatency,
			LastProcessed: lastProcessed,
		}
	}

	return triggers, nil
}

func (s *PostgreSQLCAdapter) getRecentActivity(ctx context.Context, userID string, triggerID string) ([]storage.ActivityEvent, error) {
	// Get recent execution logs for user using SQLC
	var logs []postgres.ExecutionLog
	var err error

	if triggerID != "" {
		logs, err = s.queries.GetRecentExecutionLogsForUserAndTriggerPG(ctx, postgres.GetRecentExecutionLogsForUserAndTriggerPGParams{
			UserID:    userID,
			TriggerID: &triggerID,
			Limit:     5,
		})
	} else {
		logs, err = s.queries.GetRecentExecutionLogsForUserPG(ctx, postgres.GetRecentExecutionLogsForUserPGParams{
			UserID: userID,
			Limit:  5,
		})
	}
	if err != nil {
		return nil, err
	}

	activities := make([]storage.ActivityEvent, len(logs))
	for i, log := range logs {
		eventType := "success"
		message := "Webhook processed successfully"

		if log.Status != nil && *log.Status == "error" {
			eventType = "error"
			if log.ErrorMessage != nil && *log.ErrorMessage != "" {
				message = *log.ErrorMessage
			} else {
				message = "Webhook processing failed"
			}
		}

		triggerName := ""
		if log.TriggerID != nil {
			triggerName = *log.TriggerID
			if trigger, err := s.GetTrigger(*log.TriggerID); err == nil && trigger != nil {
				triggerName = trigger.Name
			}
		}

		timestamp := time.Now()
		if log.StartedAt.Valid {
			timestamp = log.StartedAt.Time
		}

		activities[i] = storage.ActivityEvent{
			Type:      eventType,
			Trigger:   triggerName,
			Message:   message,
			Timestamp: timestamp,
		}
	}

	return activities, nil
}

// GetDashboardStats implements the dashboard stats for PostgreSQL with optional route filtering
func (s *PostgreSQLCAdapter) GetDashboardStats(userID string, currentStart, previousStart, now time.Time, triggerID string) (*storage.DashboardStats, error) {
	ctx := context.Background()

	// Get active triggers metric
	activeTriggersMetric, err := s.getActiveTriggersMetric(ctx, userID, currentStart, previousStart)
	if err != nil {
		return nil, err
	}

	// Get request metrics with optional route filtering
	requestMetrics, err := s.getRequestMetrics(ctx, userID, currentStart, previousStart, now, triggerID)
	if err != nil {
		return nil, err
	}

	// Get DLQ metric
	dlqMetric, err := s.getDLQMetric(ctx, userID, currentStart)
	if err != nil {
		return nil, err
	}

	// Get time series data - pass the triggerID for filtering
	timeSeries, err := s.getTimeSeries(ctx, userID, currentStart, now, triggerID)
	if err != nil {
		return nil, err
	}

	// Get top triggers
	topTriggers, err := s.getTopTriggers(ctx, userID, currentStart, now)
	if err != nil {
		return nil, err
	}

	// Get recent activity with optional route filtering
	recentActivity, err := s.getRecentActivity(ctx, userID, triggerID)
	if err != nil {
		return nil, err
	}

	return &storage.DashboardStats{
		Metrics: storage.DashboardMetrics{
			ActiveTriggers: activeTriggersMetric,
			TotalRequests:  requestMetrics.totalRequests,
			SuccessRate:    requestMetrics.successRate,
			AverageLatency: requestMetrics.avgLatency,
			DLQMessages:    dlqMetric,
		},
		TimeSeries:     timeSeries,
		TopTriggers:    topTriggers,
		RecentActivity: recentActivity,
	}, nil
}
