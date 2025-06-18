package triggers

import (
	"context"
	"fmt"

	"webhook-router/internal/common/distributed"
)

// DistributedExecutorAdapter adapts the simplified distributed executor to the trigger system
type DistributedExecutorAdapter struct {
	executor distributed.Executor
}

// NewDistributedExecutorAdapter creates a new adapter
func NewDistributedExecutorAdapter(executor distributed.Executor) *DistributedExecutorAdapter {
	return &DistributedExecutorAdapter{
		executor: executor,
	}
}

// ExecuteScheduledTask implements the trigger manager's distributed executor function signature
func (a *DistributedExecutorAdapter) ExecuteScheduledTask(triggerID int, taskID string, handler func() error) error {
	// Create a task that wraps the handler
	task := distributed.NewSimpleTask(
		fmt.Sprintf("trigger-%d-%s", triggerID, taskID),
		func(ctx context.Context) error {
			return handler()
		},
	)

	// Execute using the simplified interface
	return a.executor.Execute(context.Background(), task)
}

// TriggerTask represents a trigger-specific task
type TriggerTask struct {
	triggerID   int
	triggerName string
	taskID      string
	handler     func() error
}

// NewTriggerTask creates a new trigger task
func NewTriggerTask(triggerID int, triggerName, taskID string, handler func() error) distributed.Task {
	return &TriggerTask{
		triggerID:   triggerID,
		triggerName: triggerName,
		taskID:      taskID,
		handler:     handler,
	}
}

func (t *TriggerTask) ID() string {
	return fmt.Sprintf("trigger-%d-%s", t.triggerID, t.taskID)
}

func (t *TriggerTask) Execute(ctx context.Context) error {
	// Run the handler
	return t.handler()
}

// SimplifiedDistributedManager wraps trigger manager with simplified distributed execution
type SimplifiedDistributedManager struct {
	*Manager
	executor distributed.Executor
	adapter  *DistributedExecutorAdapter
}

// NewSimplifiedDistributedManager creates a distributed trigger manager with simplified interface
func NewSimplifiedDistributedManager(manager *Manager, executor distributed.Executor) *SimplifiedDistributedManager {
	adapter := NewDistributedExecutorAdapter(executor)

	// Set the distributed executor on the manager
	manager.SetDistributedExecutor(adapter.ExecuteScheduledTask)

	return &SimplifiedDistributedManager{
		Manager:  manager,
		executor: executor,
		adapter:  adapter,
	}
}

// Start starts the trigger manager if this node is the leader
func (s *SimplifiedDistributedManager) Start() error {
	if !s.executor.IsLeader() {
		// Wait for leadership in a separate goroutine
		go s.waitForLeadership()
		return nil
	}

	return s.Manager.Start()
}

// Stop stops the trigger manager and closes the executor
func (s *SimplifiedDistributedManager) Stop() error {
	if err := s.Manager.Stop(); err != nil {
		return err
	}

	return s.executor.Close()
}

// IsLeader returns whether this node is the leader
func (s *SimplifiedDistributedManager) IsLeader() bool {
	return s.executor.IsLeader()
}

// waitForLeadership waits for this node to become leader and then starts the manager
func (s *SimplifiedDistributedManager) waitForLeadership() {
	// This is a simplified version - in production you'd want more sophisticated handling
	for {
		if s.executor.IsLeader() {
			if err := s.Manager.Start(); err != nil {
				s.Manager.logger.Error("Failed to start trigger management after becoming leader", err)
			}
			return
		}
		// Check every few seconds
		select {
		case <-s.Manager.ctx.Done():
			return
		case <-context.Background().Done():
			return
		}
	}
}
