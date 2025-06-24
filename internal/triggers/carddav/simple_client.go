package carddav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/models"
)

// SimpleClient provides a basic CardDAV implementation
type SimpleClient struct {
	httpClient *http.Client
	url        string
	username   string
	password   string
	syncToken  string // For sync-collection support
}

// NewSimpleClient creates a basic CardDAV client
func NewSimpleClient(serverURL, username, password string) *SimpleClient {
	return &SimpleClient{
		httpClient: commonhttp.NewHTTPClientWithTimeout(30 * time.Second),
		url:        strings.TrimSuffix(serverURL, "/"),
		username:   username,
		password:   password,
	}
}

// QueryContacts queries all contacts
func (c *SimpleClient) QueryContacts(ctx context.Context) ([]*models.Contact, error) {
	// Build REPORT request for all contacts
	reportBody := `<?xml version="1.0" encoding="utf-8" ?>
<C:addressbook-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:prop>
    <D:getetag/>
    <C:address-data/>
  </D:prop>
</C:addressbook-query>`

	// Create request
	req, err := http.NewRequestWithContext(ctx, "REPORT", c.url, strings.NewReader(reportBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")
	req.SetBasicAuth(c.username, c.password)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return c.parseMultiStatusResponse(string(body))
}

// parseMultiStatusResponse extracts contacts from WebDAV multistatus response
func (c *SimpleClient) parseMultiStatusResponse(xmlResponse string) ([]*models.Contact, error) {
	var contacts []*models.Contact

	// Extract ETags for change detection
	etagMap := make(map[string]string)

	// Simple extraction of ETags
	responseBlocks := strings.Split(xmlResponse, "<D:response>")
	for _, block := range responseBlocks[1:] {
		// Extract href
		hrefStart := strings.Index(block, "<D:href>")
		hrefEnd := strings.Index(block, "</D:href>")
		if hrefStart == -1 || hrefEnd == -1 {
			continue
		}
		href := block[hrefStart+8 : hrefEnd]

		// Extract etag
		etagStart := strings.Index(block, "<D:getetag>")
		etagEnd := strings.Index(block, "</D:getetag>")
		if etagStart != -1 && etagEnd != -1 {
			etag := block[etagStart+11 : etagEnd]
			// Remove quotes
			etag = strings.Trim(etag, "\"")
			etagMap[href] = etag
		}
	}

	// Extract vCard data
	addrDataStart := "<C:address-data>"
	addrDataEnd := "</C:address-data>"

	startIdx := 0
	for {
		start := strings.Index(xmlResponse[startIdx:], addrDataStart)
		if start == -1 {
			break
		}
		start += startIdx + len(addrDataStart)

		end := strings.Index(xmlResponse[start:], addrDataEnd)
		if end == -1 {
			break
		}
		end += start

		vcardData := xmlResponse[start:end]
		// Unescape XML entities
		vcardData = strings.ReplaceAll(vcardData, "&lt;", "<")
		vcardData = strings.ReplaceAll(vcardData, "&gt;", ">")
		vcardData = strings.ReplaceAll(vcardData, "&amp;", "&")
		vcardData = strings.ReplaceAll(vcardData, "&#13;", "\r")

		// Parse vCard
		contact, err := ParseVCard(strings.NewReader(vcardData))
		if err == nil && contact != nil {
			// Find matching etag
			for href, etag := range etagMap {
				if strings.Contains(href, contact.UID) {
					contact.Metadata.ETag = etag
					break
				}
			}
			contacts = append(contacts, contact)
		}

		startIdx = end
	}

	return contacts, nil
}

// SyncCollection performs a sync-collection request for efficient change detection
func (c *SimpleClient) SyncCollection(ctx context.Context) ([]*models.Contact, string, error) {
	// Build sync-collection request
	var syncBody string
	if c.syncToken == "" {
		// Initial sync
		syncBody = `<?xml version="1.0" encoding="utf-8" ?>
<D:sync-collection xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:sync-token/>
  <D:sync-level>1</D:sync-level>
  <D:prop>
    <D:getetag/>
    <C:address-data/>
  </D:prop>
</D:sync-collection>`
	} else {
		// Incremental sync
		syncBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<D:sync-collection xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:sync-token>%s</D:sync-token>
  <D:sync-level>1</D:sync-level>
  <D:prop>
    <D:getetag/>
    <C:address-data/>
  </D:prop>
</D:sync-collection>`, c.syncToken)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "REPORT", c.url, strings.NewReader(syncBody))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create sync request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")
	req.SetBasicAuth(c.username, c.password)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("sync request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check if sync-collection is supported
	if resp.StatusCode == http.StatusNotImplemented || resp.StatusCode == http.StatusBadRequest {
		// Fall back to regular query
		contacts, err := c.QueryContacts(ctx)
		return contacts, "", err
	}

	if resp.StatusCode != http.StatusMultiStatus {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	// Extract new sync token
	newToken := c.extractSyncToken(string(body))

	// Parse contacts
	contacts, err := c.parseMultiStatusResponse(string(body))
	return contacts, newToken, err
}

// extractSyncToken extracts the sync token from the response
func (c *SimpleClient) extractSyncToken(xmlResponse string) string {
	// Simple extraction - in production use proper XML parsing
	tokenStart := strings.Index(xmlResponse, "<D:sync-token>")
	if tokenStart == -1 {
		return ""
	}
	tokenStart += len("<D:sync-token>")

	tokenEnd := strings.Index(xmlResponse[tokenStart:], "</D:sync-token>")
	if tokenEnd == -1 {
		return ""
	}

	return xmlResponse[tokenStart : tokenStart+tokenEnd]
}

// SetSyncToken sets the sync token for incremental syncs
func (c *SimpleClient) SetSyncToken(token string) {
	c.syncToken = token
}
