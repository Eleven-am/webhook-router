package storage

import (
	"fmt"
	"time"
)

// DLQStubs provides default implementations for DLQ methods
// This follows KISS principle - DLQ is handled separately via settings
type DLQStubs struct{}

// CreateDLQMessage - stub implementation
func (s *DLQStubs) CreateDLQMessage(message *DLQMessage) error {
	return fmt.Errorf("DLQ operations are handled via settings API")
}

// GetDLQMessage - stub implementation
func (s *DLQStubs) GetDLQMessage(id int) (*DLQMessage, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// GetDLQMessageByMessageID - stub implementation
func (s *DLQStubs) GetDLQMessageByMessageID(messageID string) (*DLQMessage, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// ListPendingDLQMessages - stub implementation
func (s *DLQStubs) ListPendingDLQMessages(limit int) ([]*DLQMessage, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// ListDLQMessages - stub implementation
func (s *DLQStubs) ListDLQMessages(limit, offset int) ([]*DLQMessage, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// ListDLQMessagesByRoute - stub implementation
func (s *DLQStubs) ListDLQMessagesByRoute(routeID int, limit, offset int) ([]*DLQMessage, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// ListDLQMessagesByStatus - stub implementation
func (s *DLQStubs) ListDLQMessagesByStatus(status string, limit, offset int) ([]*DLQMessage, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// UpdateDLQMessage - stub implementation
func (s *DLQStubs) UpdateDLQMessage(message *DLQMessage) error {
	return fmt.Errorf("DLQ operations are handled via settings API")
}

// UpdateDLQMessageStatus - stub implementation
func (s *DLQStubs) UpdateDLQMessageStatus(id int, status string) error {
	return fmt.Errorf("DLQ operations are handled via settings API")
}

// DeleteDLQMessage - stub implementation
func (s *DLQStubs) DeleteDLQMessage(id int) error {
	return fmt.Errorf("DLQ operations are handled via settings API")
}

// DeleteOldDLQMessages - stub implementation
func (s *DLQStubs) DeleteOldDLQMessages(before time.Time) error {
	return fmt.Errorf("DLQ operations are handled via settings API")
}

// GetDLQStats - stub implementation
func (s *DLQStubs) GetDLQStats() (*DLQStats, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}

// GetDLQStatsByRoute - stub implementation
func (s *DLQStubs) GetDLQStatsByRoute() ([]*DLQRouteStats, error) {
	return nil, fmt.Errorf("DLQ operations are handled via settings API")
}