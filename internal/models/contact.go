package models

import (
	"time"
)

// Contact represents a unified contact structure
// regardless of source (CardDAV, vCard, Google Contacts, etc.)
type Contact struct {
	ID           string            `json:"id"`
	UID          string            `json:"uid"`
	FullName     string            `json:"full_name"`
	Name         *StructuredName   `json:"name,omitempty"`
	Emails       []EmailAddress    `json:"emails,omitempty"`
	Phones       []PhoneNumber     `json:"phones,omitempty"`
	Addresses    []PostalAddress   `json:"addresses,omitempty"`
	Organization string            `json:"organization,omitempty"`
	Title        string            `json:"title,omitempty"`
	Department   string            `json:"department,omitempty"`
	Birthday     *time.Time        `json:"birthday,omitempty"`
	Anniversary  *time.Time        `json:"anniversary,omitempty"`
	Photo        *Photo            `json:"photo,omitempty"`
	URLs         []URL             `json:"urls,omitempty"`
	Notes        string            `json:"notes,omitempty"`
	Categories   []string          `json:"categories,omitempty"`
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
	Metadata     ContactMetadata   `json:"metadata"`
}

// StructuredName represents a parsed name
type StructuredName struct {
	First  string `json:"first,omitempty"`
	Last   string `json:"last,omitempty"`
	Middle string `json:"middle,omitempty"`
	Prefix string `json:"prefix,omitempty"` // Mr., Dr., etc.
	Suffix string `json:"suffix,omitempty"` // Jr., III, etc.
}

// EmailAddress represents an email with type
type EmailAddress struct {
	Address string `json:"address"`
	Type    string `json:"type,omitempty"` // work, home, other
	Primary bool   `json:"primary,omitempty"`
}

// PhoneNumber represents a phone number with type
type PhoneNumber struct {
	Number  string `json:"number"`
	Type    string `json:"type,omitempty"` // work, home, mobile, fax
	Primary bool   `json:"primary,omitempty"`
}

// PostalAddress represents a physical address
type PostalAddress struct {
	Street     string `json:"street,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
	Type       string `json:"type,omitempty"` // work, home, other
	Primary    bool   `json:"primary,omitempty"`
}

// Photo represents a contact photo
type Photo struct {
	URL      string `json:"url,omitempty"`      // External URL
	Data     string `json:"data,omitempty"`     // Base64 encoded data
	MimeType string `json:"mime_type,omitempty"` // image/jpeg, image/png
}

// URL represents a web URL
type URL struct {
	Address string `json:"address"`
	Type    string `json:"type,omitempty"` // work, home, profile, other
}

// ContactMetadata contains source-specific metadata
type ContactMetadata struct {
	Source           string                 `json:"source"` // carddav, vcard, google, outlook
	AddressbookName  string                 `json:"addressbook_name,omitempty"`
	AddressbookID    string                 `json:"addressbook_id,omitempty"`
	ETag             string                 `json:"etag,omitempty"`
	ProviderSpecific map[string]interface{} `json:"provider_specific,omitempty"`
}

// Contact type constants
const (
	EmailTypeWork  = "work"
	EmailTypeHome  = "home"
	EmailTypeOther = "other"
	
	PhoneTypeWork   = "work"
	PhoneTypeHome   = "home"
	PhoneTypeMobile = "mobile"
	PhoneTypeFax    = "fax"
	PhoneTypeOther  = "other"
	
	AddressTypeWork  = "work"
	AddressTypeHome  = "home"
	AddressTypeOther = "other"
	
	URLTypeWork    = "work"
	URLTypeHome    = "home"
	URLTypeProfile = "profile"
	URLTypeOther   = "other"
)