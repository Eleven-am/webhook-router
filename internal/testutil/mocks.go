package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
)

// MockStorage implements storage.Storage interface for testing
type MockStorage struct {
	mu                     sync.RWMutex
	triggers               map[string]*storage.Trigger
	pipelines              map[string]*storage.Pipeline
	brokerConfigs          map[string]*storage.BrokerConfig
	users                  map[string]*storage.User
	settings               map[string]string
	dlqMessages            map[string]*storage.DLQMessage
	dlqMessagesByMessageID map[string]*storage.DLQMessage
	nextID                 int

	// Control error injection
	ErrorOnMethod map[string]error
}

// NewMockStorage creates a new mock storage instance
func NewMockStorage() *MockStorage {
	return &MockStorage{
		triggers:               make(map[string]*storage.Trigger),
		pipelines:              make(map[string]*storage.Pipeline),
		brokerConfigs:          make(map[string]*storage.BrokerConfig),
		users:                  make(map[string]*storage.User),
		settings:               make(map[string]string),
		dlqMessages:            make(map[string]*storage.DLQMessage),
		dlqMessagesByMessageID: make(map[string]*storage.DLQMessage),
		nextID:                 1,
		ErrorOnMethod:          make(map[string]error),
	}
}

// Connection management
func (m *MockStorage) Connect(config storage.StorageConfig) error {
	if err := m.ErrorOnMethod["Connect"]; err != nil {
		return err
	}
	return nil
}

func (m *MockStorage) Close() error {
	if err := m.ErrorOnMethod["Close"]; err != nil {
		return err
	}
	return nil
}

func (m *MockStorage) Health() error {
	if err := m.ErrorOnMethod["Health"]; err != nil {
		return err
	}
	return nil
}

// TODO: Route methods removed - replaced with trigger-based architecture
// These methods are no longer needed since routes have been replaced with triggers

// Users
func (m *MockStorage) CreateUser(username, password string) (*storage.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["CreateUser"]; err != nil {
		return nil, err
	}

	// Check if user already exists
	for _, user := range m.users {
		if user.Username == username {
			return nil, fmt.Errorf("user %s already exists", username)
		}
	}

	userID := fmt.Sprintf("mock-user-%d", m.nextID)
	m.nextID++

	// First user is not a default user (server owner)
	isDefault := len(m.users) > 0

	user := &storage.User{
		ID:           userID,
		Username:     username,
		PasswordHash: password, // In tests, we store plain password for simplicity
		IsDefault:    isDefault,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	m.users[userID] = user
	return user, nil
}

func (m *MockStorage) GetUser(userID string) (*storage.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetUser"]; err != nil {
		return nil, err
	}

	user, ok := m.users[userID]
	if !ok {
		return nil, fmt.Errorf("user %s not found", userID)
	}
	return user, nil
}

func (m *MockStorage) GetUserByUsername(username string) (*storage.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetUserByUsername"]; err != nil {
		return nil, err
	}

	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, fmt.Errorf("user %s not found", username)
}

func (m *MockStorage) ValidateUser(username, password string) (*storage.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["ValidateUser"]; err != nil {
		return nil, err
	}

	for _, user := range m.users {
		if user.Username == username && user.PasswordHash == password {
			return user, nil
		}
	}
	return nil, fmt.Errorf("invalid credentials")
}

func (m *MockStorage) UpdateUserCredentials(userID string, username, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["UpdateUserCredentials"]; err != nil {
		return err
	}

	user, ok := m.users[userID]
	if !ok {
		return fmt.Errorf("user %s not found", userID)
	}

	user.Username = username
	user.PasswordHash = password
	user.UpdatedAt = time.Now()
	return nil
}

func (m *MockStorage) IsDefaultUser(userID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["IsDefaultUser"]; err != nil {
		return false, err
	}

	user, ok := m.users[userID]
	if !ok {
		return false, fmt.Errorf("user %s not found", userID)
	}
	return user.IsDefault, nil
}

func (m *MockStorage) GetUserCount() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetUserCount"]; err != nil {
		return 0, err
	}

	return len(m.users), nil
}

// Settings
func (m *MockStorage) GetSetting(key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetSetting"]; err != nil {
		return "", err
	}

	value, ok := m.settings[key]
	if !ok {
		return "", fmt.Errorf("setting %s not found", key)
	}
	return value, nil
}

func (m *MockStorage) SetSetting(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["SetSetting"]; err != nil {
		return err
	}

	m.settings[key] = value
	return nil
}

func (m *MockStorage) GetAllSettings() (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetAllSettings"]; err != nil {
		return nil, err
	}

	settings := make(map[string]string)
	for k, v := range m.settings {
		settings[k] = v
	}
	return settings, nil
}

// TODO: Implement all the other storage methods as needed for testing
// For now, providing minimal implementations that return errors

func (m *MockStorage) CreateExecutionLog(log *storage.ExecutionLog) error {
	return fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) UpdateExecutionLog(log *storage.ExecutionLog) error {
	return fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetExecutionLog(id string) (*storage.ExecutionLog, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) ListExecutionLogs(userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) ListExecutionLogsWithCount(userID string, limit, offset int) ([]*storage.ExecutionLog, int, error) {
	return nil, 0, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetExecutionLogsByTriggerID(triggerID string, userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetExecutionLogsByTriggerType(triggerType string, userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetExecutionLogsByStatus(status string, userID string, limit, offset int) ([]*storage.ExecutionLog, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetStats() (*storage.Stats, error) {
	return &storage.Stats{}, nil
}

func (m *MockStorage) GetTriggerStats(triggerID string) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

func (m *MockStorage) GetDashboardStats(userID string, currentStart, previousStart, now time.Time, triggerID string) (*storage.DashboardStats, error) {
	return &storage.DashboardStats{}, nil
}

// Trigger operations
func (m *MockStorage) CreateTrigger(trigger *storage.Trigger) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["CreateTrigger"]; err != nil {
		return err
	}

	trigger.ID = fmt.Sprintf("mock-trigger-%d", m.nextID)
	m.nextID++
	trigger.CreatedAt = time.Now()
	trigger.UpdatedAt = time.Now()
	m.triggers[trigger.ID] = trigger
	return nil
}

func (m *MockStorage) GetTrigger(id string) (*storage.Trigger, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetTrigger"]; err != nil {
		return nil, err
	}

	trigger, ok := m.triggers[id]
	if !ok {
		return nil, fmt.Errorf("trigger %s not found", id)
	}
	return trigger, nil
}

func (m *MockStorage) GetHTTPTriggerByUserPathMethod(userID, path, method string) (*storage.Trigger, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, trigger := range m.triggers {
		if trigger.UserID == userID && trigger.Type == "http" {
			if config, ok := trigger.Config["path"]; ok && config == path {
				if methodConfig, ok := trigger.Config["method"]; ok && methodConfig == method {
					return trigger, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("trigger not found")
}

func (m *MockStorage) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetTriggers"]; err != nil {
		return nil, err
	}

	var result []*storage.Trigger
	for _, trigger := range m.triggers {
		result = append(result, trigger)
	}
	return result, nil
}

func (m *MockStorage) GetTriggersPaginated(filters storage.TriggerFilters, limit, offset int) ([]*storage.Trigger, int, error) {
	triggers, err := m.GetTriggers(filters)
	if err != nil {
		return nil, 0, err
	}

	total := len(triggers)
	if offset >= total {
		return []*storage.Trigger{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return triggers[offset:end], total, nil
}

func (m *MockStorage) UpdateTrigger(trigger *storage.Trigger) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["UpdateTrigger"]; err != nil {
		return err
	}

	if _, ok := m.triggers[trigger.ID]; !ok {
		return fmt.Errorf("trigger %s not found", trigger.ID)
	}
	trigger.UpdatedAt = time.Now()
	m.triggers[trigger.ID] = trigger
	return nil
}

func (m *MockStorage) DeleteTrigger(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["DeleteTrigger"]; err != nil {
		return err
	}

	delete(m.triggers, id)
	return nil
}

// Pipeline operations
func (m *MockStorage) CreatePipeline(pipeline *storage.Pipeline) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["CreatePipeline"]; err != nil {
		return err
	}

	pipeline.ID = fmt.Sprintf("mock-pipeline-%d", m.nextID)
	m.nextID++
	pipeline.CreatedAt = time.Now()
	pipeline.UpdatedAt = time.Now()
	m.pipelines[pipeline.ID] = pipeline
	return nil
}

func (m *MockStorage) GetPipeline(id string) (*storage.Pipeline, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetPipeline"]; err != nil {
		return nil, err
	}

	pipeline, ok := m.pipelines[id]
	if !ok {
		return nil, fmt.Errorf("pipeline %s not found", id)
	}
	return pipeline, nil
}

func (m *MockStorage) GetPipelines() ([]*storage.Pipeline, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetPipelines"]; err != nil {
		return nil, err
	}

	var result []*storage.Pipeline
	for _, pipeline := range m.pipelines {
		result = append(result, pipeline)
	}
	return result, nil
}

func (m *MockStorage) GetPipelinesPaginated(limit, offset int) ([]*storage.Pipeline, int, error) {
	pipelines, err := m.GetPipelines()
	if err != nil {
		return nil, 0, err
	}

	total := len(pipelines)
	if offset >= total {
		return []*storage.Pipeline{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return pipelines[offset:end], total, nil
}

func (m *MockStorage) UpdatePipeline(pipeline *storage.Pipeline) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["UpdatePipeline"]; err != nil {
		return err
	}

	if _, ok := m.pipelines[pipeline.ID]; !ok {
		return fmt.Errorf("pipeline %s not found", pipeline.ID)
	}
	pipeline.UpdatedAt = time.Now()
	m.pipelines[pipeline.ID] = pipeline
	return nil
}

func (m *MockStorage) DeletePipeline(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["DeletePipeline"]; err != nil {
		return err
	}

	delete(m.pipelines, id)
	return nil
}

// Broker operations
func (m *MockStorage) CreateBroker(broker *storage.BrokerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["CreateBroker"]; err != nil {
		return err
	}

	broker.ID = fmt.Sprintf("mock-broker-%d", m.nextID)
	m.nextID++
	broker.CreatedAt = time.Now()
	broker.UpdatedAt = time.Now()
	m.brokerConfigs[broker.ID] = broker
	return nil
}

func (m *MockStorage) GetBroker(id string) (*storage.BrokerConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetBroker"]; err != nil {
		return nil, err
	}

	broker, ok := m.brokerConfigs[id]
	if !ok {
		return nil, fmt.Errorf("broker %s not found", id)
	}
	return broker, nil
}

func (m *MockStorage) GetBrokers() ([]*storage.BrokerConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetBrokers"]; err != nil {
		return nil, err
	}

	var result []*storage.BrokerConfig
	for _, broker := range m.brokerConfigs {
		result = append(result, broker)
	}
	return result, nil
}

func (m *MockStorage) GetBrokersPaginated(limit, offset int) ([]*storage.BrokerConfig, int, error) {
	brokers, err := m.GetBrokers()
	if err != nil {
		return nil, 0, err
	}

	total := len(brokers)
	if offset >= total {
		return []*storage.BrokerConfig{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return brokers[offset:end], total, nil
}

func (m *MockStorage) UpdateBroker(broker *storage.BrokerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["UpdateBroker"]; err != nil {
		return err
	}

	if _, ok := m.brokerConfigs[broker.ID]; !ok {
		return fmt.Errorf("broker %s not found", broker.ID)
	}
	broker.UpdatedAt = time.Now()
	m.brokerConfigs[broker.ID] = broker
	return nil
}

func (m *MockStorage) DeleteBroker(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["DeleteBroker"]; err != nil {
		return err
	}

	delete(m.brokerConfigs, id)
	return nil
}

// DLQ operations
func (m *MockStorage) CreateDLQMessage(message *storage.DLQMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["CreateDLQMessage"]; err != nil {
		return err
	}

	message.ID = fmt.Sprintf("mock-dlq-%d", m.nextID)
	m.nextID++
	message.CreatedAt = time.Now()
	message.UpdatedAt = time.Now()
	m.dlqMessages[message.ID] = message
	m.dlqMessagesByMessageID[message.MessageID] = message
	return nil
}

func (m *MockStorage) GetDLQMessage(id string) (*storage.DLQMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetDLQMessage"]; err != nil {
		return nil, err
	}

	message, ok := m.dlqMessages[id]
	if !ok {
		return nil, fmt.Errorf("DLQ message %s not found", id)
	}
	return message, nil
}

func (m *MockStorage) GetDLQMessageByMessageID(messageID string) (*storage.DLQMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetDLQMessageByMessageID"]; err != nil {
		return nil, err
	}

	message, ok := m.dlqMessagesByMessageID[messageID]
	if !ok {
		return nil, fmt.Errorf("DLQ message with message ID %s not found", messageID)
	}
	return message, nil
}

func (m *MockStorage) ListPendingDLQMessages(limit int) ([]*storage.DLQMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["ListPendingDLQMessages"]; err != nil {
		return nil, err
	}

	var pending []*storage.DLQMessage
	count := 0
	for _, message := range m.dlqMessages {
		if message.Status == "pending" && count < limit {
			pending = append(pending, message)
			count++
		}
	}
	return pending, nil
}

func (m *MockStorage) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["ListDLQMessages"]; err != nil {
		return nil, err
	}

	var messages []*storage.DLQMessage
	for _, message := range m.dlqMessages {
		messages = append(messages, message)
	}

	total := len(messages)
	if offset >= total {
		return []*storage.DLQMessage{}, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return messages[offset:end], nil
}

func (m *MockStorage) ListDLQMessagesWithCount(limit, offset int) ([]*storage.DLQMessage, int, error) {
	messages, err := m.ListDLQMessages(limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return messages, len(m.dlqMessages), nil
}

func (m *MockStorage) ListDLQMessagesByTrigger(triggerID string, limit, offset int) ([]*storage.DLQMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var messages []*storage.DLQMessage
	for _, message := range m.dlqMessages {
		if message.TriggerID != nil && *message.TriggerID == triggerID {
			messages = append(messages, message)
		}
	}

	total := len(messages)
	if offset >= total {
		return []*storage.DLQMessage{}, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return messages[offset:end], nil
}

func (m *MockStorage) ListDLQMessagesByTriggerWithCount(triggerID string, limit, offset int) ([]*storage.DLQMessage, int, error) {
	messages, err := m.ListDLQMessagesByTrigger(triggerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Count total messages for this trigger
	total := 0
	for _, message := range m.dlqMessages {
		if message.TriggerID != nil && *message.TriggerID == triggerID {
			total++
		}
	}

	return messages, total, nil
}

func (m *MockStorage) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var messages []*storage.DLQMessage
	for _, message := range m.dlqMessages {
		if message.Status == status {
			messages = append(messages, message)
		}
	}

	total := len(messages)
	if offset >= total {
		return []*storage.DLQMessage{}, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return messages[offset:end], nil
}

func (m *MockStorage) UpdateDLQMessage(message *storage.DLQMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["UpdateDLQMessage"]; err != nil {
		return err
	}

	if _, ok := m.dlqMessages[message.ID]; !ok {
		return fmt.Errorf("DLQ message %s not found", message.ID)
	}
	message.UpdatedAt = time.Now()
	m.dlqMessages[message.ID] = message
	return nil
}

func (m *MockStorage) UpdateDLQMessageStatus(id string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["UpdateDLQMessageStatus"]; err != nil {
		return err
	}

	message, ok := m.dlqMessages[id]
	if !ok {
		return fmt.Errorf("DLQ message %s not found", id)
	}
	message.Status = status
	message.UpdatedAt = time.Now()
	return nil
}

func (m *MockStorage) DeleteDLQMessage(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["DeleteDLQMessage"]; err != nil {
		return err
	}

	message, ok := m.dlqMessages[id]
	if ok {
		delete(m.dlqMessagesByMessageID, message.MessageID)
	}
	delete(m.dlqMessages, id)
	return nil
}

func (m *MockStorage) DeleteOldDLQMessages(before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ErrorOnMethod["DeleteOldDLQMessages"]; err != nil {
		return err
	}

	for id, message := range m.dlqMessages {
		if message.CreatedAt.Before(before) {
			delete(m.dlqMessagesByMessageID, message.MessageID)
			delete(m.dlqMessages, id)
		}
	}
	return nil
}

func (m *MockStorage) GetDLQStats() (*storage.DLQStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetDLQStats"]; err != nil {
		return nil, err
	}

	stats := &storage.DLQStats{}
	var oldest *time.Time

	for _, message := range m.dlqMessages {
		stats.TotalMessages++
		switch message.Status {
		case "pending":
			stats.PendingMessages++
		case "retrying":
			stats.RetryingMessages++
		case "abandoned":
			stats.AbandonedMessages++
		}

		if oldest == nil || message.FirstFailure.Before(*oldest) {
			oldest = &message.FirstFailure
		}
	}

	stats.OldestFailure = oldest
	return stats, nil
}

func (m *MockStorage) GetDLQStatsByTrigger() ([]*storage.DLQTriggerStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetDLQStatsByTrigger"]; err != nil {
		return nil, err
	}

	// TODO: Implement trigger-based DLQ stats
	return []*storage.DLQTriggerStats{}, nil
}

func (m *MockStorage) GetDLQStatsByError() ([]*storage.DLQErrorStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ErrorOnMethod["GetDLQStatsByError"]; err != nil {
		return nil, err
	}

	// TODO: Implement error-based DLQ stats
	return []*storage.DLQErrorStats{}, nil
}

// OAuth2 operations - minimal implementation
func (m *MockStorage) CreateOAuth2Service(service *storage.OAuth2Service) error {
	return fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetOAuth2Service(id string) (*storage.OAuth2Service, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) GetOAuth2ServiceByName(name string, userID string) (*storage.OAuth2Service, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) ListOAuth2Services(userID string) ([]*storage.OAuth2Service, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) UpdateOAuth2Service(service *storage.OAuth2Service) error {
	return fmt.Errorf("not implemented in mock")
}

func (m *MockStorage) DeleteOAuth2Service(id string) error {
	return fmt.Errorf("not implemented in mock")
}

// Transaction support - minimal implementation
func (m *MockStorage) Transaction(fn func(tx storage.Transaction) error) error {
	// For mock purposes, just execute the function
	return fn(&mockTransaction{})
}

type mockTransaction struct{}

func (t *mockTransaction) Commit() error   { return nil }
func (t *mockTransaction) Rollback() error { return nil }

// MockBroker implements brokers.Broker interface for testing
type MockBroker struct {
	published []*brokers.Message
	mu        sync.RWMutex
	name      string
}

func NewMockBroker() *MockBroker {
	return &MockBroker{
		published: make([]*brokers.Message, 0),
		name:      "mock-broker",
	}
}

func (b *MockBroker) Name() string {
	return b.name
}

func (b *MockBroker) Connect(config brokers.BrokerConfig) error {
	return nil
}

func (b *MockBroker) Publish(message *brokers.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, message)
	return nil
}

func (b *MockBroker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	return nil
}

func (b *MockBroker) Health() error {
	return nil
}

func (b *MockBroker) Close() error {
	return nil
}

func (b *MockBroker) GetPublishedMessages() []*brokers.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]*brokers.Message{}, b.published...)
}

func (b *MockBroker) ClearPublishedMessages() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = make([]*brokers.Message, 0)
}
