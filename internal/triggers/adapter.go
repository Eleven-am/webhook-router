package triggers

import (
	"context"
	"fmt"
)

// HandlerAdapter adapts triggers.TriggerHandler to base.TriggerHandler
// This is used by all trigger implementations to bridge the interface gap
type HandlerAdapter struct {
	handler TriggerHandler
}

// NewHandlerAdapter creates a new handler adapter
func NewHandlerAdapter(handler TriggerHandler) *HandlerAdapter {
	return &HandlerAdapter{handler: handler}
}

// HandleTriggerEvent implements base.TriggerHandler
func (a *HandlerAdapter) HandleTriggerEvent(ctx context.Context, event interface{}) error {
	triggerEvent, ok := event.(*TriggerEvent)
	if !ok {
		return fmt.Errorf("invalid event type: expected *TriggerEvent, got %T", event)
	}
	return a.handler(triggerEvent)
}
