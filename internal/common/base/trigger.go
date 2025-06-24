package base

import (
	"context"
	"sync"
	"time"
	"webhook-router/internal/oauth2"
)

// TriggerConfig interface for trigger configurations
type TriggerConfig interface {
	GetType() string
	Validate() error
	GetID() string
	GetName() string
}

// TriggerHandler is an adapter interface for trigger event handling
type TriggerHandler interface {
	HandleTriggerEvent(ctx context.Context, event interface{}) error
}

// BaseTrigger provides common functionality for all trigger implementations
type BaseTrigger struct {
	config        TriggerConfig
	handler       TriggerHandler
	triggerType   string
	isRunning     bool
	mu            sync.RWMutex
	lastExecution *time.Time
	ctx           context.Context
	cancel        context.CancelFunc
	oauth2Manager *oauth2.Manager
}

// NewBaseTrigger creates a new base trigger instance
func NewBaseTrigger(triggerType string, config TriggerConfig, handler TriggerHandler) *BaseTrigger {
	return &BaseTrigger{
		config:      config,
		handler:     handler,
		triggerType: triggerType,
		isRunning:   false,
	}
}

// Name returns the trigger name
func (b *BaseTrigger) Name() string {
	return b.config.GetName()
}

// Type returns the trigger type
func (b *BaseTrigger) Type() string {
	return b.triggerType
}

// ID returns the trigger ID
func (b *BaseTrigger) ID() string {
	return b.config.GetID()
}

// Config returns the trigger configuration
func (b *BaseTrigger) Config() TriggerConfig {
	return b.config
}

// IsRunning returns whether the trigger is currently running
func (b *BaseTrigger) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isRunning
}

// LastExecution returns the last execution time
func (b *BaseTrigger) LastExecution() *time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.lastExecution == nil {
		return nil
	}
	t := *b.lastExecution
	return &t
}

// UpdateLastExecution updates the last execution time
func (b *BaseTrigger) UpdateLastExecution(t time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastExecution = &t
}

// Start starts the trigger with the provided run function
func (b *BaseTrigger) Start(ctx context.Context, runFunc func(context.Context) error) error {
	b.mu.Lock()
	if b.isRunning {
		b.mu.Unlock()
		return nil
	}
	b.isRunning = true
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.mu.Unlock()

	// Run the trigger-specific logic
	go func() {
		defer func() {
			b.mu.Lock()
			b.isRunning = false
			b.mu.Unlock()
		}()

		if err := runFunc(b.ctx); err != nil {
			// Error handling could be improved with structured logging
			// For now, we'll just return
			return
		}
	}()

	return nil
}

// Stop stops the trigger
func (b *BaseTrigger) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.isRunning {
		return nil
	}

	if b.cancel != nil {
		b.cancel()
	}

	b.isRunning = false
	return nil
}

// GetContext returns the trigger's context
func (b *BaseTrigger) GetContext() context.Context {
	return b.ctx
}

// GetHandler returns the trigger's handler
func (b *BaseTrigger) GetHandler() TriggerHandler {
	return b.handler
}

// HandleEvent is a helper method to handle events with proper error handling
func (b *BaseTrigger) HandleEvent(event interface{}) error {
	if b.handler == nil {
		return nil
	}

	b.UpdateLastExecution(time.Now())
	return b.handler.HandleTriggerEvent(b.ctx, event)
}

// SetOAuth2Manager sets the OAuth2 manager for authentication
func (b *BaseTrigger) SetOAuth2Manager(manager *oauth2.Manager) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.oauth2Manager = manager
}

// GetOAuth2Manager returns the OAuth2 manager
func (b *BaseTrigger) GetOAuth2Manager() *oauth2.Manager {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.oauth2Manager
}
