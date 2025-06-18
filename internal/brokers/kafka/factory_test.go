package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
)

func TestGetFactory(t *testing.T) {
	factory := GetFactory()
	require.NotNil(t, factory)

	// Test factory type
	assert.Equal(t, "kafka", factory.GetType())
}

func TestFactoryCreate(t *testing.T) {
	factory := GetFactory()

	tests := []struct {
		name    string
		config  brokers.BrokerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Brokers:  []string{"localhost:9092"},
				ClientID: "test-client",
				GroupID:  "test-group",
			},
			wantErr: false,
		},
		{
			name: "invalid config - no brokers",
			config: &Config{
				ClientID: "test-client",
			},
			wantErr: true,
			errMsg:  "brokers are required",
		},
		{
			name: "config with security",
			config: &Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_PLAINTEXT",
				SASLMechanism:    "PLAIN",
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, err := factory.Create(tt.config)
			
			if tt.wantErr && tt.errMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, broker)
			} else {
				// The Kafka library creates producers lazily, so creation succeeds
				// even without a real broker
				assert.NoError(t, err)
				assert.NotNil(t, broker)
				
				// Clean up
				if broker != nil {
					broker.Close()
				}
			}
		})
	}
}

func TestFactoryWithInvalidConfigType(t *testing.T) {
	factory := GetFactory()
	
	// Test with a different config type that implements BrokerConfig
	// but is not the expected *Config type
	wrongConfig := &mockBrokerConfig{}
	
	_, err := factory.Create(wrongConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config type")
}

// mockBrokerConfig implements BrokerConfig interface but is not a *Config
type mockBrokerConfig struct{}

func (m *mockBrokerConfig) Validate() error {
	return nil
}

func (m *mockBrokerConfig) GetConnectionString() string {
	return "mock://localhost"
}

func (m *mockBrokerConfig) GetType() string {
	return "mock"
}

func TestFactoryInterfaceCompliance(t *testing.T) {
	var _ brokers.BrokerFactory = GetFactory()
}