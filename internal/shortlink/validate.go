// Package shortlink holds the rules for aliases, destinations and expiry dates.
package shortlink

import (
	"crypto/rand"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var aliasPattern = regexp.MustCompile(`^[a-zA-Z-]{3,40}$`)

// reserved aliases clash with the app's own routes.
var reserved = map[string]bool{
	"links": true, "static": true, "healthz": true, "favicon.ico": true, "robots.txt": true,
}

// Errors maps form field names to messages.
type Errors map[string]string

// Form is the submitted create or edit form.
type Form struct {
	Target    string
	Alias     string
	ExpiresOn string
}

// Normalize trims whitespace from every field.
func (f Form) Normalize() Form {
	return Form{
		Target:    strings.TrimSpace(f.Target),
		Alias:     strings.TrimSpace(f.Alias),
		ExpiresOn: strings.TrimSpace(f.ExpiresOn),
	}
}

// ValidateTarget checks the destination URL.
func ValidateTarget(target string) string {
	if target == "" {
		return "Enter the address the short link should open"
	}
	if len(target) > 2048 {
		return "That address is too long"
	}
	if _, err := url.Parse(target); err != nil {
		return "That does not look like a web address"
	}
	return ""
}

// ValidateAlias checks a custom alias. An empty alias is allowed: one is generated.
func ValidateAlias(alias string) string {
	if alias == "" {
		return ""
	}
	if reserved[strings.ToLower(alias)] {
		return "That alias is reserved, please pick another"
	}
	if !aliasPattern.MatchString(alias) {
		return "Use 3 to 40 letters, numbers or dashes"
	}
	return ""
}

// ValidateExpiry checks an optional YYYY-MM-DD expiry date. It may not be in the past.
func ValidateExpiry(value string, now time.Time, loc *time.Location) string {
	if value == "" {
		return ""
	}
	day, err := time.ParseInLocation(time.DateOnly, value, loc)
	if err != nil {
		return "Pick a valid date"
	}
	today := now.In(loc).Format(time.DateOnly)
	if day.Format(time.DateOnly) < today {
		return "The expiry date cannot be in the past"
	}
	return ""
}

// Validate checks a whole form. When creating, the alias is validated too.
func Validate(f Form, creating bool, now time.Time, loc *time.Location) Errors {
	errs := Errors{}
	if msg := ValidateTarget(f.Target); msg != "" {
		errs["target"] = msg
	}
	if creating {
		if msg := ValidateAlias(f.Alias); msg != "" {
			errs["alias"] = msg
		}
	}
	if msg := ValidateExpiry(f.ExpiresOn, now, loc); msg != "" {
		errs["expires_on"] = msg
	}
	return errs
}

const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// RandomAlias returns a random alias of n characters without look-alike letters.
func RandomAlias(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}
