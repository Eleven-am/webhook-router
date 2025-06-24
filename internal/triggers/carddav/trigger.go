package carddav

import (
	"context"
	"fmt"
	"sync"
	"time"

	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/models"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/triggers"
)

// Trigger implements the CardDAV polling trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config       *Config
	client       *SimpleClient
	contactCount int64
	errorCount   int64
	mu           sync.RWMutex
	builder      *triggers.TriggerBuilder
}

func NewTrigger(config *Config) (*Trigger, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("invalid CardDAV config: %v", err))
	}

	builder := triggers.NewTriggerBuilder("carddav", config)

	// Create CardDAV client (will configure auth later if OAuth2 is used)
	client := NewSimpleClient(config.URL, config.Username, config.Password)

	trigger := &Trigger{
		config:       config,
		client:       client,
		contactCount: 0,
		errorCount:   0,
		builder:      builder,
	}

	// Initialize BaseTrigger
	trigger.BaseTrigger = base.NewBaseTrigger("carddav", config, nil)

	return trigger, nil
}

// Start starts the CardDAV polling trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Update BaseTrigger with the adapted handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)

	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		return t.run(ctx, handler)
	})
}

// Stop stops the CardDAV polling trigger
func (t *Trigger) Stop() error {
	// First stop the base trigger
	if err := t.BaseTrigger.Stop(); err != nil {
		return err
	}

	t.builder.Logger().Info("CardDAV trigger stopped",
		logging.Field{"url", t.config.URL},
	)
	return nil
}

func (t *Trigger) run(ctx context.Context, handler triggers.TriggerHandler) error {
	t.builder.Logger().Info("CardDAV trigger started",
		logging.Field{"url", t.config.URL},
		logging.Field{"poll_interval", t.config.PollInterval},
	)

	// Start polling loop
	return t.pollLoop(ctx, handler)
}

// pollLoop runs the polling loop
func (t *Trigger) pollLoop(ctx context.Context, handler triggers.TriggerHandler) error {
	ticker := time.NewTicker(t.config.PollInterval)
	defer ticker.Stop()

	// Initial poll
	t.poll(ctx, handler)

	for {
		select {
		case <-ctx.Done():
			t.builder.Logger().Info("CardDAV trigger stopped")
			return nil
		case <-ticker.C:
			t.poll(ctx, handler)
		}
	}
}

// poll performs a single poll operation
func (t *Trigger) poll(ctx context.Context, handler triggers.TriggerHandler) {
	if err := t.checkContacts(ctx, handler); err != nil {
		t.builder.Logger().Error("CardDAV check error", err)
		t.errorCount++
		return
	}
}

// checkContacts checks for contact changes
func (t *Trigger) checkContacts(ctx context.Context, handler triggers.TriggerHandler) error {
	// Try sync-collection first for efficient change detection
	syncToken := t.config.SyncTokens[t.config.URL]
	t.client.SetSyncToken(syncToken)

	contacts, newToken, err := t.client.SyncCollection(ctx)
	if err == nil && newToken != "" {
		// Sync successful, update token
		if t.config.SyncTokens == nil {
			t.config.SyncTokens = make(map[string]string)
		}
		t.config.SyncTokens[t.config.URL] = newToken
	} else if err != nil || newToken == "" {
		// Fallback to regular query
		contacts, err = t.client.QueryContacts(ctx)
		if err != nil {
			return fmt.Errorf("failed to query contacts: %w", err)
		}
	}

	// Process contacts
	for _, contact := range contacts {
		// Check if we've already processed this contact
		if contact.Metadata.ETag != "" {
			if oldEtag, exists := t.config.ETags[contact.UID]; exists && oldEtag == contact.Metadata.ETag {
				continue // No changes
			}
			t.config.ETags[contact.UID] = contact.Metadata.ETag
		}

		if err := t.processContact(contact, handler); err != nil {
			t.builder.Logger().Error("Failed to process contact", err,
				logging.Field{"uid", contact.UID},
			)
		} else {
			// Mark as processed
			t.config.ProcessedUIDs[contact.UID] = time.Now()
		}
	}

	// Update last sync time
	t.config.LastSync = time.Now()

	return nil
}

// processContact processes a single contact
func (t *Trigger) processContact(contact *models.Contact, handler triggers.TriggerHandler) error {
	// Convert to generic map for webhook data
	contactData := map[string]interface{}{
		"id":           contact.ID,
		"uid":          contact.UID,
		"full_name":    contact.FullName,
		"organization": contact.Organization,
		"title":        contact.Title,
		"department":   contact.Department,
		"notes":        contact.Notes,
		"created":      contact.Created,
		"updated":      contact.Updated,
		"metadata":     contact.Metadata,
	}

	// Add optional fields
	if contact.Name != nil {
		contactData["name"] = contact.Name
	}
	if len(contact.Emails) > 0 {
		contactData["emails"] = contact.Emails
	}
	if len(contact.Phones) > 0 {
		contactData["phones"] = contact.Phones
	}
	if len(contact.Addresses) > 0 {
		contactData["addresses"] = contact.Addresses
	}
	if contact.Birthday != nil {
		contactData["birthday"] = contact.Birthday
	}
	if contact.Anniversary != nil {
		contactData["anniversary"] = contact.Anniversary
	}
	if contact.Photo != nil {
		contactData["photo"] = contact.Photo
	}
	if len(contact.URLs) > 0 {
		contactData["urls"] = contact.URLs
	}
	if len(contact.Categories) > 0 {
		contactData["categories"] = contact.Categories
	}

	// Create trigger event
	event := &triggers.TriggerEvent{
		ID:          utils.GenerateEventID("carddav", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "contact",
		Timestamp:   time.Now(),
		Data:        contactData,
		Headers: map[string]string{
			"X-Trigger-Type": "carddav",
			"X-Contact-UID":  contact.UID,
		},
		Source: triggers.TriggerSource{
			Type: "carddav",
			Name: t.config.Name,
			URL:  t.config.URL,
		},
	}

	// Call handler
	if err := handler(event); err != nil {
		return errors.InternalError("handler error", err)
	}

	t.contactCount++

	return nil
}

// NextExecution returns the next scheduled execution time
func (t *Trigger) NextExecution() *time.Time {
	if !t.IsRunning() || t.LastExecution() == nil {
		return nil
	}

	next := t.LastExecution().Add(t.config.PollInterval)
	return &next
}

// Health returns the trigger health status
func (t *Trigger) Health() error {
	if !t.IsRunning() {
		return errors.InternalError("trigger is not running", nil)
	}

	return nil
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}

// SetOAuthManager sets the OAuth2 manager for authentication
func (t *Trigger) SetOAuthManager(manager *oauth2.Manager) {
	// Set in BaseTrigger
	t.BaseTrigger.SetOAuth2Manager(manager)

	// Update the client if we're using OAuth2
	if t.config.UseOAuth2 && t.config.OAuth2ServiceID != "" {
		// Replace with OAuth2-enabled client
		t.client = NewOAuth2Client(t.config.URL, t.config.OAuth2ServiceID, manager)
	}
}

// GetOAuth2ID returns the OAuth2 service ID if configured
func (t *Trigger) GetOAuth2ID() string {
	if t.config.UseOAuth2 {
		return t.config.OAuth2ServiceID
	}
	return ""
}

// SetOAuth2ID sets the OAuth2 service ID
func (t *Trigger) SetOAuth2ID(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.config.OAuth2ServiceID = id
	t.config.UseOAuth2 = id != ""
}
