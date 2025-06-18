package models

import (
	"time"
)

// CalendarEvent represents a unified calendar event structure
// regardless of source (CalDAV, iCal, Google Calendar, etc.)
type CalendarEvent struct {
	ID          string              `json:"id"`
	UID         string              `json:"uid"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Location    string              `json:"location"`
	Start       time.Time           `json:"start"`
	End         time.Time           `json:"end"`
	AllDay      bool                `json:"all_day"`
	Status      string              `json:"status"` // confirmed, tentative, cancelled
	Organizer   *Person             `json:"organizer,omitempty"`
	Attendees   []Attendee          `json:"attendees,omitempty"`
	Recurrence  string              `json:"recurrence,omitempty"` // Raw RRULE string
	Reminders   []Reminder          `json:"reminders,omitempty"`
	Categories  []string            `json:"categories,omitempty"`
	Created     time.Time           `json:"created"`
	Updated     time.Time           `json:"updated"`
	Metadata    CalendarMetadata    `json:"metadata"`
}

// Person represents a person with email and name
type Person struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Attendee represents an event attendee
type Attendee struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Status   string `json:"status"`   // accepted, declined, tentative, needs-action
	Required bool   `json:"required"` // required vs optional
	Role     string `json:"role,omitempty"` // chair, participant, etc.
}

// Reminder represents an event reminder/alarm
type Reminder struct {
	Method  string `json:"method"`  // email, popup, sms
	Minutes int    `json:"minutes"` // minutes before event
}

// CalendarMetadata contains source-specific metadata
type CalendarMetadata struct {
	Source         string                 `json:"source"` // caldav, ical, google, outlook
	CalendarName   string                 `json:"calendar_name,omitempty"`
	CalendarID     string                 `json:"calendar_id,omitempty"`
	ETag           string                 `json:"etag,omitempty"`
	Sequence       int                    `json:"sequence,omitempty"`
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// CalendarEventStatus constants
const (
	EventStatusConfirmed  = "confirmed"
	EventStatusTentative  = "tentative"
	EventStatusCancelled  = "cancelled"
)

// AttendeeStatus constants
const (
	AttendeeStatusAccepted   = "accepted"
	AttendeeStatusDeclined   = "declined"
	AttendeeStatusTentative  = "tentative"
	AttendeeStatusNeedsAction = "needs-action"
)

// ReminderMethod constants
const (
	ReminderMethodEmail = "email"
	ReminderMethodPopup = "popup"
	ReminderMethodSMS   = "sms"
)