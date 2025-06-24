package carddav

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"webhook-router/internal/models"
)

// ParseVCard converts vCard data to our unified contact model
func ParseVCard(reader io.Reader) (*models.Contact, error) {
	dec := vcard.NewDecoder(reader)
	card, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to decode vCard: %w", err)
	}

	contact := &models.Contact{
		Metadata: models.ContactMetadata{
			Source: "carddav",
		},
		Created: time.Now(),
		Updated: time.Now(),
	}

	// UID
	if uid := card.Get(vcard.FieldUID); uid != nil {
		contact.UID = uid.Value
		contact.ID = uid.Value
	}

	// Full name
	if fn := card.Get(vcard.FieldFormattedName); fn != nil {
		contact.FullName = fn.Value
	}

	// Structured name - use the library's Name helper
	if name := card.Name(); name != nil {
		contact.Name = &models.StructuredName{
			First:  name.GivenName,
			Last:   name.FamilyName,
			Middle: name.AdditionalName,
			Prefix: name.HonorificPrefix,
			Suffix: name.HonorificSuffix,
		}
	}

	// Emails
	for _, email := range card[vcard.FieldEmail] {
		emailAddr := models.EmailAddress{
			Address: email.Value,
		}

		// Check type parameter
		if types := email.Params.Types(); len(types) > 0 {
			emailAddr.Type = strings.ToLower(types[0])
		}

		// Check if preferred
		if email.Params.Get("PREF") == "1" {
			emailAddr.Primary = true
		}

		contact.Emails = append(contact.Emails, emailAddr)
	}

	// Phone numbers
	for _, tel := range card[vcard.FieldTelephone] {
		phone := models.PhoneNumber{
			Number: tel.Value,
		}

		// Check type parameter
		if types := tel.Params.Types(); len(types) > 0 {
			// Map vCard types to our types
			vCardType := strings.ToLower(types[0])
			switch vCardType {
			case "work":
				phone.Type = models.PhoneTypeWork
			case "home":
				phone.Type = models.PhoneTypeHome
			case "cell":
				phone.Type = models.PhoneTypeMobile
			case "fax":
				phone.Type = models.PhoneTypeFax
			default:
				phone.Type = models.PhoneTypeOther
			}
		}

		// Check if preferred
		if tel.Params.Get("PREF") == "1" {
			phone.Primary = true
		}

		contact.Phones = append(contact.Phones, phone)
	}

	// Addresses - use the library's Addresses helper
	for _, address := range card.Addresses() {
		addr := models.PostalAddress{
			Street:     address.StreetAddress,
			City:       address.Locality,
			State:      address.Region,
			PostalCode: address.PostalCode,
			Country:    address.Country,
		}

		// Check type
		if types := address.Params.Types(); len(types) > 0 {
			addr.Type = strings.ToLower(types[0])
		}

		// Check if preferred
		if address.Params.Get("PREF") == "1" {
			addr.Primary = true
		}

		contact.Addresses = append(contact.Addresses, addr)
	}

	// Organization
	if org := card.Get(vcard.FieldOrganization); org != nil {
		// Organization can have multiple components separated by semicolons
		// We'll take the first component as the main organization name
		orgParts := strings.Split(org.Value, ";")
		if len(orgParts) > 0 && orgParts[0] != "" {
			contact.Organization = orgParts[0]
		}
	}

	// Title
	if title := card.Get(vcard.FieldTitle); title != nil {
		contact.Title = title.Value
	}

	// Birthday
	if bday := card.Get(vcard.FieldBirthday); bday != nil {
		if t, err := parseVCardDate(bday.Value); err == nil {
			contact.Birthday = &t
		}
	}

	// Anniversary
	if anniversary := card.Get(vcard.FieldAnniversary); anniversary != nil {
		if t, err := parseVCardDate(anniversary.Value); err == nil {
			contact.Anniversary = &t
		}
	}

	// Photo
	if photo := card.Get(vcard.FieldPhoto); photo != nil {
		contact.Photo = &models.Photo{}

		// Check if it's a URI or embedded data
		if strings.HasPrefix(photo.Value, "http") {
			contact.Photo.URL = photo.Value
		} else {
			// Assume base64 encoded
			contact.Photo.Data = photo.Value
			if mediaType := photo.Params.Get("MEDIATYPE"); mediaType != "" {
				contact.Photo.MimeType = mediaType
			}
		}
	}

	// URLs
	for _, url := range card[vcard.FieldURL] {
		u := models.URL{
			Address: url.Value,
		}

		if types := url.Params.Types(); len(types) > 0 {
			u.Type = strings.ToLower(types[0])
		}

		contact.URLs = append(contact.URLs, u)
	}

	// Notes
	if note := card.Get(vcard.FieldNote); note != nil {
		contact.Notes = note.Value
	}

	// Categories - use the library's Categories helper
	contact.Categories = card.Categories()

	// Timestamps - use the library's Revision helper
	if t, err := card.Revision(); err == nil && !t.IsZero() {
		contact.Updated = t
	}

	return contact, nil
}

// parseVCardDate parses various vCard date formats
func parseVCardDate(value string) (time.Time, error) {
	// Try different date formats
	formats := []string{
		"20060102",   // Basic format
		"2006-01-02", // Extended format
		time.RFC3339, // Full datetime
		"2006-01-02T15:04:05Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", value)
}
