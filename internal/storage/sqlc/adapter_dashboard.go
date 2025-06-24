package sqlc

import (
	"context"
	"time"

	"webhook-router/internal/storage"
	"webhook-router/internal/storage/generated/sqlite"
)

// Helper methods for GetDashboardStats

type requestMetricsResult struct {
	totalRequests storage.MetricWithChange
	successRate   storage.MetricWithChange
	avgLatency    storage.MetricWithChange
}

func (s *SQLCAdapter) getActiveTriggersMetric(ctx context.Context, userID string, currentStart, previousStart time.Time) (storage.MetricWithChange, error) {
	// Get current active triggers count for user
	triggers, err := s.queries.ListTriggersByUser(ctx, userID)
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	currentActive := 0
	for _, trigger := range triggers {
		if trigger.Active != nil && *trigger.Active && trigger.DeletedAt == nil {
			currentActive++
		}
	}

	// Count triggers created/activated in current period
	newTriggers := 0
	deactivated := 0
	for _, trigger := range triggers {
		if trigger.CreatedAt != nil && trigger.CreatedAt.After(currentStart) && trigger.Active != nil && *trigger.Active {
			newTriggers++
		}
		if trigger.UpdatedAt != nil && trigger.UpdatedAt.After(currentStart) && (trigger.Active == nil || !*trigger.Active) {
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

func (s *SQLCAdapter) getRequestMetrics(ctx context.Context, userID string, currentStart, previousStart, now time.Time, triggerID string) (*requestMetricsResult, error) {
	var currentTotal, currentSuccess int64
	var previousTotal, previousSuccess int64
	var err error

	// Get current period stats for user
	if triggerID != "" {
		stats, err := s.queries.GetExecutionLogStatsForUserAndTrigger(ctx, sqlite.GetExecutionLogStatsForUserAndTriggerParams{
			StartedAt: &currentStart,
			UserID:    userID,
			TriggerID: &triggerID,
		})
		if err != nil {
			return nil, err
		}
		currentTotal = stats.TotalCount
		currentSuccess = stats.SuccessCount
	} else {
		stats, err := s.queries.GetExecutionLogStatsForUser(ctx, sqlite.GetExecutionLogStatsForUserParams{
			StartedAt: &currentStart,
			UserID:    userID,
		})
		if err != nil {
			return nil, err
		}
		currentTotal = stats.TotalCount
		currentSuccess = stats.SuccessCount
	}

	// Get previous period stats for user
	if triggerID != "" {
		stats, err := s.queries.GetExecutionLogStatsForUserAndTrigger(ctx, sqlite.GetExecutionLogStatsForUserAndTriggerParams{
			StartedAt: &previousStart,
			UserID:    userID,
			TriggerID: &triggerID,
		})
		if err != nil {
			return nil, err
		}
		previousTotal = stats.TotalCount
		previousSuccess = stats.SuccessCount
	} else {
		stats, err := s.queries.GetExecutionLogStatsForUser(ctx, sqlite.GetExecutionLogStatsForUserParams{
			StartedAt: &previousStart,
			UserID:    userID,
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
	var currentLatencyPtr, previousLatencyPtr *float64

	if triggerID != "" {
		currentLatencyPtr, err = s.queries.GetAverageLatencyForUserAndTrigger(ctx, sqlite.GetAverageLatencyForUserAndTriggerParams{
			StartedAt:   &currentStart,
			StartedAt_2: &now,
			UserID:      userID,
			TriggerID:   &triggerID,
		})
	} else {
		currentLatencyPtr, err = s.queries.GetAverageLatencyForUser(ctx, sqlite.GetAverageLatencyForUserParams{
			StartedAt:   &currentStart,
			StartedAt_2: &now,
			UserID:      userID,
		})
	}
	if err != nil {
		return nil, err
	}

	// Get average latency for previous period for user
	if triggerID != "" {
		previousLatencyPtr, err = s.queries.GetAverageLatencyForUserAndTrigger(ctx, sqlite.GetAverageLatencyForUserAndTriggerParams{
			StartedAt:   &previousStart,
			StartedAt_2: &currentStart,
			UserID:      userID,
			TriggerID:   &triggerID,
		})
	} else {
		previousLatencyPtr, err = s.queries.GetAverageLatencyForUser(ctx, sqlite.GetAverageLatencyForUserParams{
			StartedAt:   &previousStart,
			StartedAt_2: &currentStart,
			UserID:      userID,
		})
	}
	if err != nil {
		return nil, err
	}

	currentLatency := 0.0
	if currentLatencyPtr != nil {
		currentLatency = *currentLatencyPtr
	}

	previousLatency := 0.0
	if previousLatencyPtr != nil {
		previousLatency = *previousLatencyPtr
	}

	latencyChange := currentLatency - previousLatency
	latencyChangePercent := 0.0
	if previousLatency > 0 {
		latencyChangePercent = (latencyChange / previousLatency) * 100
	}

	return &requestMetricsResult{
		totalRequests: storage.MetricWithChange{
			Current:       currentTotalFloat,
			Previous:      previousTotalFloat,
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
			Current:       currentLatency,
			Previous:      previousLatency,
			Change:        latencyChange,
			ChangePercent: latencyChangePercent,
		},
	}, nil
}

func (s *SQLCAdapter) getDLQMetric(ctx context.Context, userID string, periodStart time.Time) (storage.MetricWithChange, error) {
	// Get current DLQ count for user
	stats, err := s.GetDLQStatsForUser(userID)
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	currentCount := float64(stats.PendingMessages)

	// Count DLQ messages added in period for user using SQLC
	addedResult, err := s.queries.CountDLQMessagesAddedSinceForUser(ctx, sqlite.CountDLQMessagesAddedSinceForUserParams{
		CreatedAt: &periodStart,
		UserID:    userID,
	})
	if err != nil {
		return storage.MetricWithChange{}, err
	}

	// Count DLQ messages resolved in period for user using SQLC
	resolvedResult, err := s.queries.CountDLQMessagesResolvedSinceForUser(ctx, sqlite.CountDLQMessagesResolvedSinceForUserParams{
		UpdatedAt: &periodStart,
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

func (s *SQLCAdapter) getTimeSeries(ctx context.Context, userID string, start, end time.Time, triggerID string) ([]storage.TimeSeriesPoint, error) {
	// Determine interval based on time range
	duration := end.Sub(start)

	var points []storage.TimeSeriesPoint

	triggerFilter := triggerID
	if triggerFilter == "" {
		triggerFilter = "all"
	}

	if duration <= 24*time.Hour {
		// Hourly aggregation
		params := sqlite.GetExecutionLogTimeSeriesHourlyForUserParams{
			StartedAt:   &start,
			StartedAt_2: &end,
			UserID:      userID,
			Column4:     triggerFilter,
			TriggerID:   &triggerFilter,
		}
		rows, err := s.queries.GetExecutionLogTimeSeriesHourlyForUser(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert rows to points
		for _, row := range rows {
			timestamp := time.Time{}
			if row.TimeBucket != nil {
				// Parse the time bucket string back to time.Time
				if timeStr, ok := row.TimeBucket.(string); ok {
					if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
						timestamp = t
					}
				}
			}
			points = append(points, storage.TimeSeriesPoint{
				Timestamp: timestamp,
				Requests:  int(row.TotalCount),
				Errors:    int(row.ErrorCount),
			})
		}
	} else if duration <= 7*24*time.Hour {
		// Daily aggregation
		params := sqlite.GetExecutionLogTimeSeriesDailyForUserParams{
			StartedAt:   &start,
			StartedAt_2: &end,
			UserID:      userID,
			Column4:     triggerFilter,
			TriggerID:   &triggerFilter,
		}
		dailyRows, err := s.queries.GetExecutionLogTimeSeriesDailyForUser(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert daily rows to points
		for _, row := range dailyRows {
			timestamp := time.Time{}
			if row.TimeBucket != nil {
				// Parse the time bucket string back to time.Time
				if timeStr, ok := row.TimeBucket.(string); ok {
					if t, err := time.Parse("2006-01-02", timeStr); err == nil {
						timestamp = t
					}
				}
			}
			points = append(points, storage.TimeSeriesPoint{
				Timestamp: timestamp,
				Requests:  int(row.TotalCount),
				Errors:    int(row.ErrorCount),
			})
		}
	} else {
		// Weekly aggregation
		params := sqlite.GetExecutionLogTimeSeriesWeeklyForUserParams{
			StartedAt:   &start,
			StartedAt_2: &end,
			UserID:      userID,
			Column4:     triggerFilter,
			TriggerID:   &triggerFilter,
		}
		weeklyRows, err := s.queries.GetExecutionLogTimeSeriesWeeklyForUser(ctx, params)
		if err != nil {
			return nil, err
		}
		// Convert weekly rows to points
		for _, row := range weeklyRows {
			timestamp := time.Time{}
			if row.TimeBucket != nil {
				// Parse the time bucket string back to time.Time
				if timeStr, ok := row.TimeBucket.(string); ok {
					if t, err := time.Parse("2006-01-W", timeStr); err == nil {
						timestamp = t
					}
				}
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

func (s *SQLCAdapter) getTopTriggers(ctx context.Context, userID string, start, end time.Time) ([]storage.TriggerStatsSummary, error) {
	// Get top triggers by request volume for user using SQLC
	params := sqlite.GetTopTriggersByRequestsForUserParams{
		StartedAt: &start,
		UserID:    userID,
		Limit:     10,
	}

	rows, err := s.queries.GetTopTriggersByRequestsForUser(ctx, params)
	if err != nil {
		return nil, err
	}

	triggers := make([]storage.TriggerStatsSummary, len(rows))
	for i, row := range rows {
		successRate := 0.0
		if row.TotalRequests > 0 {
			successRate = (float64(row.SuccessfulRequests) / float64(row.TotalRequests)) * 100
		}

		avgLatency := 0.0
		if row.AvgLatency != nil {
			avgLatency = *row.AvgLatency
		}

		var lastProcessed *time.Time
		if row.LastProcessed != nil {
			if timeStr, ok := row.LastProcessed.(string); ok {
				if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
					lastProcessed = &t
				}
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

func (s *SQLCAdapter) getRecentActivity(ctx context.Context, userID string, triggerID string) ([]storage.ActivityEvent, error) {
	// Get recent execution logs for user using SQLC
	var logs []sqlite.ExecutionLog
	var err error

	if triggerID != "" {
		logs, err = s.queries.GetRecentExecutionLogsForUserAndTrigger(ctx, sqlite.GetRecentExecutionLogsForUserAndTriggerParams{
			UserID:    userID,
			TriggerID: &triggerID,
			Limit:     5,
		})
	} else {
		logs, err = s.queries.GetRecentExecutionLogsForUser(ctx, sqlite.GetRecentExecutionLogsForUserParams{
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
		if log.StartedAt != nil {
			timestamp = *log.StartedAt
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

// GetDashboardStats implements the dashboard stats for SQLite with optional trigger filtering
func (s *SQLCAdapter) GetDashboardStats(userID string, currentStart, previousStart, now time.Time, triggerID string) (*storage.DashboardStats, error) {
	ctx := context.Background()

	// Get active triggers metric
	activeTriggersMetric, err := s.getActiveTriggersMetric(ctx, userID, currentStart, previousStart)
	if err != nil {
		return nil, err
	}

	// Get request metrics with optional trigger filtering
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

	// Get recent activity with optional trigger filtering
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
