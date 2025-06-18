package imap

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/triggers"
)

// Trigger implements the IMAP polling trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config        *Config
	client        *client.Client
	messageCount  int64
	errorCount    int64
	oauthManager  *oauth2.Manager
	mu            sync.RWMutex
	builder       *triggers.TriggerBuilder
}


func NewTrigger(config *Config) (*Trigger, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("invalid IMAP config: %v", err))
	}
	
	builder := triggers.NewTriggerBuilder("imap", config)
	
	trigger := &Trigger{
		config:       config,
		messageCount: 0,
		errorCount:   0,
		builder:      builder,
	}
	
	// Initialize BaseTrigger
	trigger.BaseTrigger = base.NewBaseTrigger("imap", config, nil)
	
	return trigger, nil
}

// Start starts the IMAP polling trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Update BaseTrigger with the adapted handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)
	
	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		return t.run(ctx, handler)
	})
}

// Stop stops the IMAP polling trigger
func (t *Trigger) Stop() error {
	// First stop the base trigger
	if err := t.BaseTrigger.Stop(); err != nil {
		return err
	}
	
	// Then close IMAP connection
	if t.client != nil {
		if err := t.client.Logout(); err != nil {
			t.builder.Logger().Error("Failed to logout from IMAP", err)
		}
		t.client = nil
	}
	
	t.builder.Logger().Info("IMAP trigger stopped",
		logging.Field{"host", t.config.Host},
		logging.Field{"username", t.config.Username},
	)
	return nil
}

func (t *Trigger) run(ctx context.Context, handler triggers.TriggerHandler) error {
	t.builder.Logger().Info("IMAP trigger started",
		logging.Field{"host", t.config.Host},
		logging.Field{"username", t.config.Username},
		logging.Field{"folder", t.config.Folder},
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
	t.poll(handler)
	
	for {
		select {
		case <-ctx.Done():
			t.builder.Logger().Info("IMAP trigger stopped")
			return nil
		case <-ticker.C:
			t.poll(handler)
		}
	}
}

// poll performs a single poll operation
func (t *Trigger) poll(handler triggers.TriggerHandler) {
	
	if err := t.connect(); err != nil {
		t.builder.Logger().Error("IMAP connection error", err)
		t.errorCount++
		return
	}
	
	if err := t.checkEmails(handler); err != nil {
		t.builder.Logger().Error("IMAP check error", err)
		t.errorCount++
		return
	}
}

// connect establishes connection to IMAP server
func (t *Trigger) connect() error {
	if t.client != nil {
		// Check if connection is still alive
		if t.client.State() == imap.ConnectedState {
			return nil // Connection is good
		}
		// Connection is dead, close it
		t.client.Logout()
		t.client = nil
	}
	
	// Connect to server
	var c *client.Client
	var err error
	
	addr := fmt.Sprintf("%s:%d", t.config.Host, t.config.Port)
	if t.config.UseTLS {
		c, err = client.DialTLS(addr, nil)
	} else {
		c, err = client.Dial(addr)
	}
	
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	
	// Login
	if t.config.UseOAuth2 {
		// Use OAuth2 authentication
		if err := t.loginWithOAuth2(c); err != nil {
			c.Logout()
			return fmt.Errorf("OAuth2 login failed: %w", err)
		}
	} else {
		// Use password authentication
		if err := c.Login(t.config.Username, t.config.Password); err != nil {
			c.Logout()
			return fmt.Errorf("login failed: %w", err)
		}
	}
	
	t.client = c
	return nil
}

// checkEmails checks for new emails
func (t *Trigger) checkEmails(handler triggers.TriggerHandler) error {
	// Select folder
	mbox, err := t.client.Select(t.config.Folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder: %w", err)
	}
	
	// Build search criteria
	criteria := imap.NewSearchCriteria()
	if t.config.UnseenOnly {
		criteria.WithoutFlags = []string{imap.SeenFlag}
	}
	
	// Search for messages
	ids, err := t.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	
	if len(ids) == 0 {
		return nil // No new messages
	}
	
	// Create sequence set
	seqset := new(imap.SeqSet)
	for _, id := range ids {
		// Skip if we've already processed this UID
		if t.config.LastUID > 0 && id <= t.config.LastUID {
			continue
		}
		seqset.AddNum(id)
	}
	
	if seqset.Empty() {
		return nil // No new messages to process
	}
	
	// Fetch messages
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	
	go func() {
		done <- t.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchBody, imap.FetchUid, imap.FetchFlags, imap.FetchInternalDate, imap.FetchRFC822Size}, messages)
	}()
	
	// Process messages
	for msg := range messages {
		if err := t.processMessage(msg, handler); err != nil {
			t.builder.Logger().Error("Failed to process message", err,
				logging.Field{"uid", msg.Uid},
			)
		} else {
			// Update last UID
			if msg.Uid > t.config.LastUID {
				t.config.LastUID = msg.Uid
			}
		}
	}
	
	if err := <-done; err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}
	
	// Update last UID in mbox
	if mbox.UidNext > 0 {
		t.config.LastUID = mbox.UidNext - 1
	}
	
	return nil
}

// processMessage processes a single email message
func (t *Trigger) processMessage(msg *imap.Message, handler triggers.TriggerHandler) error {
	// Parse email
	emailData := map[string]interface{}{
		"id":       fmt.Sprintf("%d", msg.Uid),
		"uid":      msg.Uid,
		"date":     msg.Envelope.Date,
		"subject":  msg.Envelope.Subject,
		"size":     msg.Size,
		"internal_date": msg.InternalDate,
	}
	
	// Process flags
	flags := map[string]bool{
		"seen":      false,
		"answered":  false,
		"flagged":   false,
		"deleted":   false,
		"draft":     false,
		"recent":    false,
	}
	
	// Check standard flags
	for _, flag := range msg.Flags {
		switch flag {
		case imap.SeenFlag:
			flags["seen"] = true
		case imap.AnsweredFlag:
			flags["answered"] = true
		case imap.FlaggedFlag:
			flags["flagged"] = true  // This is "starred" in many clients
		case imap.DeletedFlag:
			flags["deleted"] = true
		case imap.DraftFlag:
			flags["draft"] = true
		case imap.RecentFlag:
			flags["recent"] = true
		}
	}
	
	// Gmail specific flags (labels) and other providers
	var labels []string
	var isImportant bool
	for _, flag := range msg.Flags {
		// Check for Gmail Important flag
		if flag == "\\Important" {
			isImportant = true
		} else if !strings.HasPrefix(flag, "\\") {
			// Custom flags/labels usually don't start with \
			labels = append(labels, flag)
		}
	}
	
	// Add important flag if found
	if isImportant {
		flags["important"] = true
	}
	
	emailData["flags"] = flags
	if len(labels) > 0 {
		emailData["labels"] = labels
	}
	
	// From addresses
	if len(msg.Envelope.From) > 0 {
		from := msg.Envelope.From[0]
		fromEmail := fmt.Sprintf("%s@%s", from.MailboxName, from.HostName)
		emailData["from"] = map[string]string{
			"email": fromEmail,
			"name":  from.PersonalName,
		}
		
		// Check from filter
		if len(t.config.FromFilter) > 0 {
			matched := false
			for _, filter := range t.config.FromFilter {
				if strings.Contains(fromEmail, filter) {
					matched = true
					break
				}
			}
			if !matched {
				return nil // Skip this email
			}
		}
	}
	
	// Subject filter
	if len(t.config.SubjectFilter) > 0 {
		matched := false
		for _, filter := range t.config.SubjectFilter {
			if strings.Contains(msg.Envelope.Subject, filter) {
				matched = true
				break
			}
		}
		if !matched {
			return nil // Skip this email
		}
	}
	
	// To addresses
	to := make([]map[string]string, 0)
	for _, addr := range msg.Envelope.To {
		to = append(to, map[string]string{
			"email": fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName),
			"name":  addr.PersonalName,
		})
	}
	emailData["to"] = to
	
	// CC addresses
	if len(msg.Envelope.Cc) > 0 {
		cc := make([]map[string]string, 0)
		for _, addr := range msg.Envelope.Cc {
			cc = append(cc, map[string]string{
				"email": fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName),
				"name":  addr.PersonalName,
			})
		}
		emailData["cc"] = cc
	}
	
	// BCC addresses (usually not available in received emails)
	if len(msg.Envelope.Bcc) > 0 {
		bcc := make([]map[string]string, 0)
		for _, addr := range msg.Envelope.Bcc {
			bcc = append(bcc, map[string]string{
				"email": fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName),
				"name":  addr.PersonalName,
			})
		}
		emailData["bcc"] = bcc
	}
	
	// Get body if requested
	if t.config.IncludeBody {
		bodySection, _ := imap.ParseBodySectionName("BODY[]")
		r := msg.GetBody(bodySection)
		if r != nil {
			// Parse message
			mr, err := mail.CreateReader(r)
			if err == nil {
				// Read headers if requested
				if t.config.IncludeHeaders {
					headers := make(map[string]string)
					header := mr.Header
					// Get common headers
					if msgID := header.Get("Message-ID"); msgID != "" {
						headers["Message-ID"] = msgID
					}
					if inReplyTo := header.Get("In-Reply-To"); inReplyTo != "" {
						headers["In-Reply-To"] = inReplyTo
					}
					if references := header.Get("References"); references != "" {
						headers["References"] = references
					}
					if returnPath := header.Get("Return-Path"); returnPath != "" {
						headers["Return-Path"] = returnPath
					}
					if contentType := header.Get("Content-Type"); contentType != "" {
						headers["Content-Type"] = contentType
					}
					if replyTo := header.Get("Reply-To"); replyTo != "" {
						headers["Reply-To"] = replyTo
					}
					emailData["headers"] = headers
				}
				
				// Read body parts
				var textBody, htmlBody string
				attachments := make([]map[string]interface{}, 0)
				
				for {
					p, err := mr.NextPart()
					if err == io.EOF {
						break
					}
					if err != nil {
						continue
					}
					
					switch h := p.Header.(type) {
					case *mail.InlineHeader:
						// This is the message body
						b, _ := io.ReadAll(p.Body)
						contentType, _, _ := h.ContentType()
						
						if strings.Contains(contentType, "text/plain") {
							textBody = string(b)
						} else if strings.Contains(contentType, "text/html") {
							htmlBody = string(b)
						}
						
					case *mail.AttachmentHeader:
						// This is an attachment
						if t.config.AttachmentMode != "none" {
							filename, _ := h.Filename()
							contentType, _, _ := h.ContentType()
							
							attachment := map[string]interface{}{
								"filename":     filename,
								"content_type": contentType,
							}
							
							if t.config.AttachmentMode == "base64" {
								b, _ := io.ReadAll(p.Body)
								attachment["content"] = base64.StdEncoding.EncodeToString(b)
								attachment["size"] = len(b)
							}
							
							attachments = append(attachments, attachment)
						}
					}
				}
				
				if textBody != "" {
					emailData["body_text"] = textBody
				}
				if htmlBody != "" {
					emailData["body_html"] = htmlBody
				}
				if len(attachments) > 0 {
					emailData["attachments"] = attachments
				}
			}
		}
	}
	
	// Create trigger event
	event := &triggers.TriggerEvent{
		ID:          utils.GenerateEventID("imap", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "email",
		Timestamp:   time.Now(),
		Data:        emailData,
		Headers:     map[string]string{
			"X-Trigger-Type": "imap",
			"X-Email-UID":    fmt.Sprintf("%d", msg.Uid),
		},
		Source: triggers.TriggerSource{
			Type: "imap",
			Name: t.config.Name,
			URL:  fmt.Sprintf("imap://%s@%s/%s", t.config.Username, t.config.Host, t.config.Folder),
		},
	}
	
	// Call handler
	if err := handler(event); err != nil {
		return errors.InternalError("handler error", err)
	}
	
	t.messageCount++
	
	// Mark as read if configured
	if t.config.MarkAsRead {
		seqSet := new(imap.SeqSet)
		seqSet.AddNum(msg.Uid)
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}
		if err := t.client.UidStore(seqSet, item, flags, nil); err != nil {
			t.builder.Logger().Error("Failed to mark email as read", err,
				logging.Field{"uid", msg.Uid},
			)
		}
	}
	
	// Move to folder if configured
	if t.config.MoveToFolder != "" {
		seqSet := new(imap.SeqSet)
		seqSet.AddNum(msg.Uid)
		if err := t.client.UidMove(seqSet, t.config.MoveToFolder); err != nil {
			t.builder.Logger().Error("Failed to move email", err,
				logging.Field{"uid", msg.Uid},
				logging.Field{"folder", t.config.MoveToFolder},
			)
		}
	}
	
	// Delete if configured
	if t.config.DeleteAfter {
		seqSet := new(imap.SeqSet)
		seqSet.AddNum(msg.Uid)
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.DeletedFlag}
		if err := t.client.UidStore(seqSet, item, flags, nil); err != nil {
			t.builder.Logger().Error("Failed to delete email", err,
				logging.Field{"uid", msg.Uid},
			)
		}
		// Expunge to actually delete
		if err := t.client.Expunge(nil); err != nil {
			t.builder.Logger().Error("Failed to expunge", err)
		}
	}
	
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
	
	if t.client == nil {
		return errors.ConnectionError("not connected to IMAP server", nil)
	}
	
	return nil
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}



// loginWithOAuth2 performs OAuth2 authentication
func (t *Trigger) loginWithOAuth2(c *client.Client) error {
	if t.oauthManager == nil {
		return fmt.Errorf("OAuth2 manager not initialized")
	}
	
	// Register OAuth2 service if not already registered
	serviceID := fmt.Sprintf("imap_%d_%d", t.config.ID, t.config.ID)
	
	// Configure OAuth2 based on provider
	var oauthConfig *oauth2.Config
	switch t.config.OAuth2Provider {
	case "google":
		oauthConfig = &oauth2.Config{
			ClientID:     t.config.OAuth2Config.ClientID,
			ClientSecret: t.config.OAuth2Config.ClientSecret,
			TokenURL:     "https://oauth2.googleapis.com/token",
			GrantType:    "client_credentials",
			Scopes:       []string{"https://mail.google.com/"},
		}
	case "microsoft":
		oauthConfig = &oauth2.Config{
			ClientID:     t.config.OAuth2Config.ClientID,
			ClientSecret: t.config.OAuth2Config.ClientSecret,
			TokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			GrantType:    "client_credentials",
			Scopes:       []string{"https://outlook.office.com/IMAP.AccessAsUser.All"},
		}
	default:
		// Use custom configuration
		oauthConfig = &oauth2.Config{
			ClientID:     t.config.OAuth2Config.ClientID,
			ClientSecret: t.config.OAuth2Config.ClientSecret,
			TokenURL:     t.config.OAuth2Config.TokenURL,
			GrantType:    "client_credentials",
			Scopes:       t.config.OAuth2Config.Scopes,
		}
	}
	
	// Register the service
	if err := t.oauthManager.RegisterService(serviceID, oauthConfig); err != nil {
		return fmt.Errorf("failed to register OAuth2 service: %w", err)
	}
	
	// If we have a refresh token, save it
	if t.config.OAuth2Config.RefreshToken != "" {
		// Create a token with just the refresh token
		token := &oauth2.Token{
			RefreshToken: t.config.OAuth2Config.RefreshToken,
			Expiry:       time.Now().Add(-1 * time.Hour), // Mark as expired to force refresh
		}
		if t.oauthManager != nil {
			// Save the refresh token
			tokenStorage := oauth2.NewMemoryTokenStorage()
			tokenStorage.SaveToken(serviceID, token)
		}
	}
	
	// Get access token (will use refresh token if available)
	token, err := t.oauthManager.GetToken(context.Background(), serviceID)
	if err != nil {
		return fmt.Errorf("failed to get OAuth2 token: %w", err)
	}
	
	// Create SASL OAuth bearer client
	saslClient := sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
		Username: t.config.Username,
		Token:    token.AccessToken,
	})
	
	// Authenticate using SASL
	if err := c.Authenticate(saslClient); err != nil {
		return fmt.Errorf("SASL authentication failed: %w", err)
	}
	
	return nil
}

// SetOAuthManager sets the OAuth2 manager for the trigger
func (t *Trigger) SetOAuthManager(manager *oauth2.Manager) {
	t.oauthManager = manager
}