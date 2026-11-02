package shortlink

import (
	"strings"
	"testing"
	"time"
)

var lagos = time.FixedZone("WAT", 3600)

func TestValidateAlias(t *testing.T) {
	tests := []struct {
		alias string
		ok    bool
	}{
		{"", true},
		{"summer-sale", true},
		{"Menu", true},
		{"ab", false},
		{"has space", false},
		{"emoji-😀", false},
		{"links", false},
		{"STATIC", false},
		{strings.Repeat("a", 41), false},
	}
	for _, tt := range tests {
		if got := ValidateAlias(tt.alias) == ""; got != tt.ok {
			t.Errorf("ValidateAlias(%q) ok = %v, want %v", tt.alias, got, tt.ok)
		}
	}
}

func TestValidateTarget(t *testing.T) {
	if msg := ValidateTarget("https://shop.example.ng/collections/summer?ref=ig"); msg != "" {
		t.Errorf("valid URL rejected: %s", msg)
	}
	if msg := ValidateTarget(""); msg == "" {
		t.Error("empty URL accepted")
	}
	if msg := ValidateTarget("https://example.com/" + strings.Repeat("x", 2100)); msg == "" {
		t.Error("overlong URL accepted")
	}
}

func TestValidateExpiry(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		value string
		ok    bool
	}{
		{"", true},
		{"2026-06-20", true},
		{"2027-01-01", true},
		{"2026-06-01", false},
		{"2026-02-30", false},
		{"next week", false},
	}
	for _, tt := range tests {
		if got := ValidateExpiry(tt.value, now, lagos) == ""; got != tt.ok {
			t.Errorf("ValidateExpiry(%q) ok = %v, want %v", tt.value, got, tt.ok)
		}
	}
}

func TestValidateSkipsAliasWhenEditing(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	errs := Validate(Form{Target: "https://example.com", Alias: "x"}, false, now, lagos)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	errs = Validate(Form{Target: "", Alias: "x"}, true, now, lagos)
	if _, ok := errs["target"]; !ok {
		t.Error("missing target error")
	}
	if _, ok := errs["alias"]; !ok {
		t.Error("missing alias error")
	}
}

func TestRandomAlias(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		a := RandomAlias(6)
		if len(a) != 6 || strings.Trim(a, alphabet) != "" {
			t.Fatalf("bad alias %q", a)
		}
		seen[a] = true
	}
	if len(seen) < 190 {
		t.Errorf("too many collisions: %d unique of 200", len(seen))
	}
}

func TestNormalize(t *testing.T) {
	f := Form{Target: "  https://a.example ", Alias: " promo ", ExpiresOn: " 2026-01-01"}.Normalize()
	if f.Target != "https://a.example" || f.Alias != "promo" || f.ExpiresOn != "2026-01-01" {
		t.Errorf("Normalize() = %+v", f)
	}
}
