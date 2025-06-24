package caldav

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	ics "github.com/emersion/go-ical"
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/models"
)

// SimpleClient provides a basic CalDAV implementation
type SimpleClient struct {
	clientWrapper   *commonhttp.HTTPClientWrapper
	url             string
	username        string
	password        string
	oauth2ServiceID string
	syncToken       string // For sync-collection support
}

// NewSimpleClient creates a basic CalDAV client
func NewSimpleClient(serverURL, username, password string) *SimpleClient {
	// Create HTTP client wrapper
	clientWrapper := commonhttp.NewHTTPClientWrapper(
		commonhttp.WithTimeout(30 * time.Second),
	).WithCircuitBreaker(fmt.Sprintf("caldav-client-%s", serverURL))

	return &SimpleClient{
		clientWrapper: clientWrapper,
		url:           strings.TrimSuffix(serverURL, "/"),
		username:      username,
		password:      password,
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

	// Prepare headers
	headers := map[string]string{
		"Content-Type": "application/xml; charset=utf-8",
		"Depth":        "1",
	}

	// Add authentication
	if c.oauth2ServiceID == "" && c.username != "" && c.password != "" {
		// Use basic auth if no OAuth2
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password))
	}

	// Use HTTPClientWrapper
	opts := &commonhttp.RequestOptions{
		Method:    "REPORT",
		URL:       c.url,
		Body:      strings.NewReader(reportBody),
		Headers:   headers,
		ParseJSON: false, // CalDAV returns XML
	}

	// Execute request
	resp, err := c.clientWrapper.Request(ctx, opts)
	if err != nil {
		return nil, errors.ConnectionError("request failed", err)
	}

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, errors.InternalError(fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, string(resp.RawBody)), nil)
	}

	// Parse response
	return c.parseMultiStatusResponse(string(resp.RawBody))
}

// SetOAuth2Manager configures OAuth2 authentication
func (c *SimpleClient) SetOAuth2Manager(manager commonhttp.OAuth2ManagerInterface, serviceID string) {
	c.oauth2ServiceID = serviceID
	if c.clientWrapper != nil && manager != nil && serviceID != "" {
		c.clientWrapper.SetOAuth2Manager(manager, serviceID)
	}
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

	return nil, errors.InternalError("no VEVENT found", nil)
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

	return time.Time{}, errors.ValidationError(fmt.Sprintf("unable to parse datetime: %s", value))
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

	// Prepare headers
	headers := map[string]string{
		"Content-Type": "application/xml; charset=utf-8",
		"Depth":        "1",
	}

	// Add authentication
	if c.oauth2ServiceID == "" && c.username != "" && c.password != "" {
		// Use basic auth if no OAuth2
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password))
	}

	// Use HTTPClientWrapper
	opts := &commonhttp.RequestOptions{
		Method:    "REPORT",
		URL:       c.url,
		Body:      strings.NewReader(syncBody),
		Headers:   headers,
		ParseJSON: false, // CalDAV returns XML
	}

	// Execute request
	resp, err := c.clientWrapper.Request(ctx, opts)
	if err != nil {
		return nil, "", errors.ConnectionError("sync request failed", err)
	}

	// Check if sync-collection is supported
	if resp.StatusCode == http.StatusNotImplemented || resp.StatusCode == http.StatusBadRequest {
		// Fall back to regular query
		events, err := c.QueryEvents(ctx, time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, 1, 0))
		return events, "", err
	}

	if resp.StatusCode != http.StatusMultiStatus {
		return nil, "", errors.InternalError(fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, string(resp.RawBody)), nil)
	}

	// Parse response
	body := resp.RawBody

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
