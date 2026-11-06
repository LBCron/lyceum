package unit

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewHashFormat(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{6}$`)
	for range 100 {
		if h := NewHash(); !re.MatchString(h) {
			t.Fatalf("NewHash() = %q, want 6 lowercase hex characters", h)
		}
	}
}

func TestSlug(t *testing.T) {
	forty := strings.Repeat("a", 40)
	tests := []struct {
		name, title, want string
	}{
		{"simple", "CSV export", "csv-export"},
		{"punctuation", "Fix: login (OAuth) bug!", "fix-login-oauth-bug"},
		{"runs collapse", "a -- b __ c", "a-b-c"},
		{"digits kept", "Upgrade to Go 1.27", "upgrade-to-go-1-27"},
		{"leading and trailing junk", "  ...Hello, World!!!  ", "hello-world"},
		{"unicode dropped", "Café — naïve façade", "caf-na-ve-fa-ade"},
		{"unicode uppercase", "ÀBC", "bc"},
		{"emoji", "🚀 launch 🚀", "launch"},
		{"exactly 40", forty, forty},
		{"41 cut to 40", forty + "b", forty},
		{"cut lands on dash", strings.Repeat("a", 39) + " bcd", strings.Repeat("a", 39)},
		{"cut mid-word after collapsed run", strings.Repeat("a", 38) + " - bcd", strings.Repeat("a", 38) + "-b"},
		{"long title", "Add a very long feature title that goes well past the limit", "add-a-very-long-feature-title-that-goes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Slug(tt.title)
			if err != nil {
				t.Fatalf("Slug(%q): %v", tt.title, err)
			}
			if got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.title, got, tt.want)
			}
			if len(got) > MaxSlug {
				t.Errorf("Slug(%q) is %d characters, max %d", tt.title, len(got), MaxSlug)
			}
		})
	}
}

func TestSlugEmpty(t *testing.T) {
	for _, title := range []string{"", "   ", "!!!---???", "—…", "日本語"} {
		if got, err := Slug(title); err == nil {
			t.Errorf("Slug(%q) = %q, want an error", title, got)
		}
	}
}

func TestIDRoundTrip(t *testing.T) {
	id := ID("a3f9c2", "csv-export")
	if id != "a3f9c2-csv-export" {
		t.Fatalf("ID = %q", id)
	}
	hash, slug, err := ParseID(id)
	if err != nil || hash != "a3f9c2" || slug != "csv-export" {
		t.Errorf("ParseID(%q) = %q, %q, %v", id, hash, slug, err)
	}
}

func TestParseIDInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"a3f9c2",
		"a3f9c2-",
		"a3f9c-csv",
		"a3f9c23-csv",
		"A3F9C2-csv",
		"g3f9c2-csv",
		"a3f9c2-CSV",
		"a3f9c2--csv",
		"a3f9c2-csv-",
		"a3f9c2-csv--export",
		"a3f9c2-csv_export",
		"a3f9c2-" + strings.Repeat("a", 41),
	} {
		if h, sl, err := ParseID(s); err == nil {
			t.Errorf("ParseID(%q) = %q, %q; want an error", s, h, sl)
		}
	}
	if _, _, err := ParseID("a3f9c2-" + strings.Repeat("a", 40)); err != nil {
		t.Errorf("40-character slug rejected: %v", err)
	}
}

func TestBranchPrefix(t *testing.T) {
	want := map[string]string{
		"feat":     "feat/",
		"bug":      "fix/",
		"refactor": "refactor/",
		"chore":    "chore/",
		"doc":      "docs/",
		"test":     "test/",
	}
	for typ, prefix := range want {
		got, err := BranchPrefix(typ)
		if err != nil || got != prefix {
			t.Errorf("BranchPrefix(%q) = %q, %v; want %q", typ, got, err, prefix)
		}
	}
	for _, typ := range []string{"", "fix", "docs", "feature", "Feat", "spike"} {
		if got, err := BranchPrefix(typ); err == nil {
			t.Errorf("BranchPrefix(%q) = %q; want an error", typ, got)
		}
	}
}
