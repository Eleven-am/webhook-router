package triggers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"webhook-router/internal/common/logging"
	"webhook-router/internal/locks"
	"webhook-router/internal/redis"
)

// DistributedManager wraps the regular Manager with distributed coordination
type DistributedManager struct {
	*Manager
	lockManager         *locks.Manager
	redisClient         *redis.Client
	nodeID              string
	isLeader            bool
	leaderLock          locks.Lock
	mu                  sync.RWMutex
	coordinationEnabled bool
	ctx                 context.Context
	cancel              context.CancelFunc
	logger              logging.Logger
}

// DistributedConfig contains configuration for distributed trigger management
type DistributedConfig struct {
	NodeID              string        `json:"node_id"`
	LeaderElectionTTL   time.Duration `json:"leader_election_ttl"`
	TaskLockTTL         time.Duration `json:"task_lock_ttl"`
	LeaderCheckInterval time.Duration `json:"leader_check_interval"`
}

// NewDistributedManager creates a distributed trigger manager
func NewDistributedManager(manager *Manager, redisClient *redis.Client, lockManager *locks.Manager, config *DistributedConfig) *DistributedManager {
	if config == nil {
		config = &DistributedConfig{
			NodeID:              fmt.Sprintf("node-%d", time.Now().UnixNano()),
			LeaderElectionTTL:   30 * time.Second,
			TaskLockTTL:         5 * time.Minute,
			LeaderCheckInterval: 10 * time.Second,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"component", "distributed_manager"},
		logging.Field{"node_id", config.NodeID},
	)

	dm := &DistributedManager{
		Manager:             manager,
		lockManager:         lockManager,
		redisClient:         redisClient,
		nodeID:              config.NodeID,
		isLeader:            false,
		coordinationEnabled: redisClient != nil && lockManager != nil,
		ctx:                 ctx,
		cancel:              cancel,
		logger:              logger,
	}

	if dm.coordinationEnabled {
		// Set up distributed executor for the underlying manager
		dm.Manager.SetDistributedExecutor(dm.ExecuteScheduledTask)

		// Start leader election process
		go dm.leaderElectionLoop(config)
		dm.logger.Info("Distributed trigger manager initialized")
	} else {
		dm.logger.Info("Distributed coordination disabled - running in standalone mode")
	}

	return dm
}

// Start starts the distributed manager with leader election
func (dm *DistributedManager) Start() error {
	if !dm.coordinationEnabled {
		// Fall back to normal manager behavior
		return dm.Manager.Start()
	}

	// Wait for initial leader election
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to become leader or wait for leader election
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for leader election")
		default:
			dm.mu.RLock()
			isLeader := dm.isLeader
			dm.mu.RUnlock()

			if isLeader {
				dm.logger.Info("Node is now the leader, starting trigger management")
				return dm.Manager.Start()
			}

			dm.logger.Debug("Node is not the leader, waiting")
			time.Sleep(2 * time.Second)
		}
	}
}

// Stop stops the distributed manager and releases leadership
func (dm *DistributedManager) Stop() error {
	if dm.coordinationEnabled {
		dm.releaseLeadership()
		dm.cancel() // Cancel the context to stop background goroutines
	}
	return dm.Manager.Stop()
}

// ExecuteScheduledTask executes a scheduled task with distributed coordination
func (dm *DistributedManager) ExecuteScheduledTask(triggerID int, taskID string, handler func() error) error {
	if !dm.coordinationEnabled {
		// Fall back to direct execution
		return handler()
	}

	// Try to acquire task lock
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	taskLock, err := dm.lockManager.AcquireTaskLock(ctx, taskID)
	if err != nil {
		// Another node is already executing this task
		dm.logger.Debug("Task already being executed by another node",
			logging.Field{"task_id", taskID},
		)
		return nil
	}
	defer taskLock.Release(context.Background())

	// Check if we're still the leader before executing
	dm.mu.RLock()
	isLeader := dm.isLeader
	dm.mu.RUnlock()

	if !isLeader {
		dm.logger.Warn("Node is no longer leader, skipping task",
			logging.Field{"task_id", taskID},
		)
		return nil
	}

	// Execute the task
	dm.logger.Info("Executing scheduled task",
		logging.Field{"task_id", taskID},
		logging.Field{"trigger_id", triggerID},
	)
	return handler()
}

// IsLeader returns whether this node is currently the leader
func (dm *DistributedManager) IsLeader() bool {
	if !dm.coordinationEnabled {
		return true // Always leader in standalone mode
	}

	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.isLeader
}

// GetNodeID returns the current node ID
func (dm *DistributedManager) GetNodeID() string {
	return dm.nodeID
}

// leaderElectionLoop continuously tries to maintain leadership
func (dm *DistributedManager) leaderElectionLoop(config *DistributedConfig) {
	ticker := time.NewTicker(config.LeaderCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.tryBecomeLeader(config)
		}
	}
}

// tryBecomeLeader attempts to acquire or maintain leadership
func (dm *DistributedManager) tryBecomeLeader(config *DistributedConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dm.mu.Lock()
	defer dm.mu.Unlock()

	// If we're already the leader, try to extend the lock
	if dm.isLeader && dm.leaderLock != nil {
		if err := dm.leaderLock.Extend(ctx, config.LeaderElectionTTL); err != nil {
			dm.logger.Error("Failed to extend leadership lock", err)
			dm.isLeader = false
			dm.leaderLock = nil
			dm.stopTriggerManagement()
		} else {
			dm.logger.Debug("Extended leadership lock")
		}
		return
	}

	// Try to acquire leadership
	lock, err := dm.lockManager.AcquireSchedulerLock(ctx, "trigger-leader")
	if err != nil {
		// Another node is the leader
		if dm.isLeader {
			dm.logger.Info("Node lost leadership")
			dm.isLeader = false
			dm.leaderLock = nil
			dm.stopTriggerManagement()
		}
		return
	}

	// We became the leader
	if !dm.isLeader {
		dm.logger.Info("Node acquired leadership")
		dm.isLeader = true
		dm.leaderLock = lock
		dm.startTriggerManagement()
	} else {
		dm.leaderLock = lock
	}
}

// startTriggerManagement starts trigger management when becoming leader
func (dm *DistributedManager) startTriggerManagement() {
	go func() {
		if err := dm.Manager.Start(); err != nil {
			dm.logger.Error("Failed to start trigger management", err)
		}
	}()
}

// stopTriggerManagement stops trigger management when losing leadership
func (dm *DistributedManager) stopTriggerManagement() {
	go func() {
		if err := dm.Manager.Stop(); err != nil {
			dm.logger.Error("Failed to stop trigger management", err)
		}
	}()
}

// releaseLeadership releases the leadership lock
func (dm *DistributedManager) releaseLeadership() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.leaderLock != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dm.leaderLock.Release(ctx)
		dm.leaderLock = nil
	}
	dm.isLeader = false
}

// BroadcastTriggerEvent broadcasts a trigger event to all nodes
func (dm *DistributedManager) BroadcastTriggerEvent(event *TriggerEvent) error {
	if !dm.coordinationEnabled {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Publish event to Redis for other nodes to see
	channel := fmt.Sprintf("trigger-events:%s", event.Type)
	return dm.redisClient.Publish(ctx, channel, event)
}

// GetClusterStatus returns the status of the distributed cluster
func (dm *DistributedManager) GetClusterStatus() map[string]interface{} {
	status := map[string]interface{}{
		"node_id":              dm.nodeID,
		"coordination_enabled": dm.coordinationEnabled,
		"is_leader":            dm.IsLeader(),
		"trigger_count":        len(dm.GetAllTriggers()),
	}

	if dm.coordinationEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check Redis connectivity
		if err := dm.redisClient.Health(); err != nil {
			status["redis_health"] = "unhealthy"
			status["redis_error"] = err.Error()
		} else {
			status["redis_health"] = "healthy"
		}

		// Get leader info from Redis
		if leaderInfo, err := dm.redisClient.Get(ctx, "trigger-leader"); err == nil {
			status["current_leader"] = leaderInfo
		}
	}

	return status
}
