package caldav

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	
	ics "github.com/emersion/go-ical"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/models"
)

// SimpleClient provides a basic CalDAV implementation
type SimpleClient struct {
	httpClient *http.Client
	url        string
	username   string
	password   string
	syncToken  string // For sync-collection support
}

// NewSimpleClient creates a basic CalDAV client
func NewSimpleClient(serverURL, username, password string) *SimpleClient {
	return &SimpleClient{
		httpClient: commonhttp.NewHTTPClientWithTimeout(30 * time.Second),
		url:      strings.TrimSuffix(serverURL, "/"),
		username: username,
		password: password,
	}
}

// QueryEvents queries events using a REPORT request
func (c *SimpleClient) QueryEvents(ctx context.Context, start, end time.Time) ([]*models.CalendarEvent, error) {
	// Build REPORT request body
	reportBody := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
  </D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="%s" end="%s"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>`,
		start.UTC().Format("20060102T150405Z"),
		end.UTC().Format("20060102T150405Z"))
	
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

// parseMultiStatusResponse extracts calendar events from WebDAV multistatus response
func (c *SimpleClient) parseMultiStatusResponse(xmlResponse string) ([]*models.CalendarEvent, error) {
	var events []*models.CalendarEvent
	
	// Simple extraction of calendar data
	// In production, use proper XML parsing
	calDataStart := "<C:calendar-data>"
	calDataEnd := "</C:calendar-data>"
	
	startIdx := 0
	for {
		start := strings.Index(xmlResponse[startIdx:], calDataStart)
		if start == -1 {
			break
		}
		start += startIdx + len(calDataStart)
		
		end := strings.Index(xmlResponse[start:], calDataEnd)
		if end == -1 {
			break
		}
		end += start
		
		calData := xmlResponse[start:end]
		// Unescape XML entities
		calData = strings.ReplaceAll(calData, "&lt;", "<")
		calData = strings.ReplaceAll(calData, "&gt;", ">")
		calData = strings.ReplaceAll(calData, "&amp;", "&")
		calData = strings.ReplaceAll(calData, "&#13;", "\r")
		
		// Parse iCal data
		event, err := c.parseICalData(calData)
		if err == nil && event != nil {
			events = append(events, event)
		}
		
		startIdx = end
	}
	
	return events, nil
}

// parseICalData parses iCal data into our model
func (c *SimpleClient) parseICalData(icalData string) (*models.CalendarEvent, error) {
	dec := ics.NewDecoder(strings.NewReader(icalData))
	cal, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	
	// Find first VEVENT
	for _, component := range cal.Children {
		if component.Name == ics.CompEvent {
			return parseSimpleVEvent(component)
		}
	}
	
	return nil, fmt.Errorf("no VEVENT found")
}

// parseSimpleVEvent converts VEVENT to our model
func parseSimpleVEvent(event *ics.Component) (*models.CalendarEvent, error) {
	calEvent := &models.CalendarEvent{
		Metadata: models.CalendarMetadata{
			Source: "caldav",
		},
	}
	
	// Get properties
	if uid := event.Props.Get(ics.PropUID); uid != nil {
		calEvent.UID = uid.Value
		calEvent.ID = uid.Value
	}
	
	if summary := event.Props.Get(ics.PropSummary); summary != nil {
		calEvent.Title = summary.Value
	}
	
	if desc := event.Props.Get(ics.PropDescription); desc != nil {
		calEvent.Description = desc.Value
	}
	
	if loc := event.Props.Get(ics.PropLocation); loc != nil {
		calEvent.Location = loc.Value
	}
	
	if dtstart := event.Props.Get(ics.PropDateTimeStart); dtstart != nil {
		if t, err := parseSimpleDateTime(dtstart.Value); err == nil {
			calEvent.Start = t
			// Check for all-day
			if len(dtstart.Value) == 8 {
				calEvent.AllDay = true
			}
		}
	}
	
	if dtend := event.Props.Get(ics.PropDateTimeEnd); dtend != nil {
		if t, err := parseSimpleDateTime(dtend.Value); err == nil {
			calEvent.End = t
		}
	}
	
	if status := event.Props.Get(ics.PropStatus); status != nil {
		calEvent.Status = strings.ToLower(status.Value)
	} else {
		calEvent.Status = models.EventStatusConfirmed
	}
	
	if rrule := event.Props.Get("RRULE"); rrule != nil {
		calEvent.Recurrence = rrule.Value
	}
	
	if created := event.Props.Get(ics.PropCreated); created != nil {
		if t, err := parseSimpleDateTime(created.Value); err == nil {
			calEvent.Created = t
		}
	}
	
	if modified := event.Props.Get(ics.PropLastModified); modified != nil {
		if t, err := parseSimpleDateTime(modified.Value); err == nil {
			calEvent.Updated = t
		}
	}
	
	// Parse organizer
	if org := event.Props.Get(ics.PropOrganizer); org != nil {
		calEvent.Organizer = &models.Person{}
		if strings.HasPrefix(strings.ToUpper(org.Value), "MAILTO:") {
			calEvent.Organizer.Email = org.Value[7:]
		}
		if cn := org.Params.Get("CN"); cn != "" {
			calEvent.Organizer.Name = cn
		}
	}
	
	// Parse attendees
	for _, prop := range event.Props[ics.PropAttendee] {
		attendee := models.Attendee{
			Status: models.AttendeeStatusNeedsAction,
		}
		
		if strings.HasPrefix(strings.ToUpper(prop.Value), "MAILTO:") {
			attendee.Email = prop.Value[7:]
		}
		
		if cn := prop.Params.Get("CN"); cn != "" {
			attendee.Name = cn
		}
		
		if partstat := prop.Params.Get("PARTSTAT"); partstat != "" {
			switch strings.ToUpper(partstat) {
			case "ACCEPTED":
				attendee.Status = models.AttendeeStatusAccepted
			case "DECLINED":
				attendee.Status = models.AttendeeStatusDeclined
			case "TENTATIVE":
				attendee.Status = models.AttendeeStatusTentative
			}
		}
		
		if role := prop.Params.Get("ROLE"); role != "" {
			attendee.Role = strings.ToLower(role)
			attendee.Required = strings.ToUpper(role) == "REQ-PARTICIPANT"
		}
		
		calEvent.Attendees = append(calEvent.Attendees, attendee)
	}
	
	return calEvent, nil
}

// parseSimpleDateTime parses common iCal datetime formats
func parseSimpleDateTime(value string) (time.Time, error) {
	// Remove any timezone suffix for now
	value = strings.TrimSuffix(value, "Z")
	
	formats := []string{
		"20060102T150405", // DateTime
		"20060102",        // Date only
	}
	
	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t, nil
		}
	}
	
	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", value)
}

// SyncCollection performs a sync-collection request for efficient change detection
func (c *SimpleClient) SyncCollection(ctx context.Context) ([]*models.CalendarEvent, string, error) {
	// Build sync-collection request
	var syncBody string
	if c.syncToken == "" {
		// Initial sync
		syncBody = `<?xml version="1.0" encoding="utf-8" ?>
<D:sync-collection xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:sync-token/>
  <D:sync-level>1</D:sync-level>
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
  </D:prop>
</D:sync-collection>`
	} else {
		// Incremental sync
		syncBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<D:sync-collection xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:sync-token>%s</D:sync-token>
  <D:sync-level>1</D:sync-level>
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
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
		events, err := c.QueryEvents(ctx, time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, 1, 0))
		return events, "", err
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
	
	// Parse events
	events, err := c.parseMultiStatusResponse(string(body))
	return events, newToken, err
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