package imap

import (
	"context"
	"testing"
	"time"
	"webhook-router/internal/triggers"
)

func TestIMAPTriggerConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "Test IMAP",
				},
				Host:         "imap.gmail.com",
				Port:         993,
				UseTLS:       true,
				Username:     "user@example.com",
				Password:     "password",
				Folder:       "INBOX",
				PollInterval: 5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "missing host",
			config: &Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "Test IMAP",
				},
				Username:     "user@example.com",
				Password:     "password",
				PollInterval: 5 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "missing username",
			config: &Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "Test IMAP",
				},
				Host:     "imap.gmail.com",
				Password: "password",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			config: &Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "Test IMAP",
				},
				Host:     "imap.gmail.com",
				Username: "user@example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIMAPTriggerFactory(t *testing.T) {
	factory := &Factory{}
	
	// Test factory type
	if got := factory.GetType(); got != "imap" {
		t.Errorf("Factory.GetType() = %v, want %v", got, "imap")
	}
	
	// Test default config
	defaultConfig := factory.DefaultConfig()
	if defaultConfig == nil {
		t.Error("Factory.DefaultConfig() returned nil")
	}
	
	// Test creating trigger with valid config
	config := &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   1,
			Name: "Test IMAP",
		},
		Host:         "imap.gmail.com",
		Port:         993,
		UseTLS:       true,
		Username:     "user@example.com",
		Password:     "password",
		Folder:       "INBOX",
		PollInterval: 5 * time.Minute,
	}
	
	trigger, err := factory.Create(config)
	if err != nil {
		t.Fatalf("Factory.Create() error = %v", err)
	}
	
	if trigger == nil {
		t.Fatal("Factory.Create() returned nil trigger")
	}
	
	// Test trigger interface methods
	if trigger.Name() != "Test IMAP" {
		t.Errorf("Trigger.Name() = %v, want %v", trigger.Name(), "Test IMAP")
	}
	
	if trigger.Type() != "imap" {
		t.Errorf("Trigger.Type() = %v, want %v", trigger.Type(), "imap")
	}
	
	if trigger.ID() != 1 {
		t.Errorf("Trigger.ID() = %v, want %v", trigger.ID(), 1)
	}
	
	// Test IsRunning before start
	if trigger.IsRunning() {
		t.Error("Trigger should not be running before Start()")
	}
}

func TestIMAPTriggerLifecycle(t *testing.T) {
	config := &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   1,
			Name: "Test IMAP",
		},
		Host:         "imap.example.com",
		Port:         993,
		UseTLS:       true,
		Username:     "test@example.com",
		Password:     "password",
		Folder:       "INBOX",
		PollInterval: 1 * time.Minute, // Minimum allowed interval
	}
	
	trigger, err := NewTrigger(config)
	if err != nil {
		t.Fatalf("NewTrigger() error = %v", err)
	}
	
	// Create a mock handler
	handler := func(event *triggers.TriggerEvent) error {
		// Handler called when email is processed
		return nil
	}
	
	// Start trigger (will fail to connect but that's expected)
	ctx := context.Background()
	err = trigger.Start(ctx, handler)
	if err != nil {
		t.Fatalf("Trigger.Start() error = %v", err)
	}
	
	// Check if running
	if !trigger.IsRunning() {
		t.Error("Trigger should be running after Start()")
	}
	
	// Give it a moment to attempt connection
	time.Sleep(200 * time.Millisecond)
	
	// Stop trigger
	err = trigger.Stop()
	if err != nil {
		t.Fatalf("Trigger.Stop() error = %v", err)
	}
	
	// Check if stopped
	if trigger.IsRunning() {
		t.Error("Trigger should not be running after Stop()")
	}
}

func TestIMAPEncryptedPassword(t *testing.T) {
	config := &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   1,
			Name: "Test IMAP",
		},
		Host:         "imap.gmail.com",
		Port:         993,
		UseTLS:       true,
		Username:     "user@example.com",
		Password:     "",
		Folder:       "INBOX",
		PollInterval: 5 * time.Minute,
	}
	
	// Config should be valid even without plaintext password if encrypted is present
	err := config.Validate()
	if err == nil {
		t.Error("Config.Validate() should fail when password is empty and no decryption happens")
	}
}