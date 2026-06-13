package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEntriesNormalizesAndDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "domains.json")
	data := `[
		{"domain":" Example.COM ","proxied":false,"ip_version":4},
		{"domain":"IPv6.Example.com","proxied":true,"ip_version":6,"ttl":120}
	]`

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	entries, err := LoadEntries(path)
	if err != nil {
		t.Fatalf("LoadEntries returned error: %v", err)
	}

	if got, want := entries[0].Domain, "example.com"; got != want {
		t.Fatalf("Domain = %q, want %q", got, want)
	}
	if got, want := entries[0].TTL, 1; got != want {
		t.Fatalf("TTL = %d, want %d", got, want)
	}
	if got, want := entries[1].Domain, "ipv6.example.com"; got != want {
		t.Fatalf("Domain = %q, want %q", got, want)
	}
}

func TestValidateEntriesRejectsDuplicateCombination(t *testing.T) {
	t.Parallel()

	err := ValidateEntries([]Entry{
		{Domain: "Example.com", IPVersion: 4},
		{Domain: " example.com ", IPVersion: 4},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("ValidateEntries error = %v, want duplicate error", err)
	}
}

func TestValidateEntriesRejectsInvalidTTL(t *testing.T) {
	t.Parallel()

	err := ValidateEntries([]Entry{{Domain: "example.com", IPVersion: 4, TTL: 59}})
	if err == nil || !strings.Contains(err.Error(), "ttl") {
		t.Fatalf("ValidateEntries error = %v, want ttl error", err)
	}
}

func TestLoadEntriesRejectsUnknownField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "domains.json")
	data := `[{"domain":"example.com","proxied":false,"ip_version":4,"zone_id":"abc"}]`

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadEntries(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadEntries error = %v, want unknown field error", err)
	}
}

func TestRecordType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version int
		want    string
	}{
		{version: 4, want: "A"},
		{version: 6, want: "AAAA"},
	}

	for _, tt := range tests {
		got, err := RecordType(tt.version)
		if err != nil {
			t.Fatalf("RecordType(%d) error = %v", tt.version, err)
		}
		if got != tt.want {
			t.Fatalf("RecordType(%d) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestLoadEnv(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "token")
	t.Setenv("RUN_INTERVAL", "15m")

	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv error = %v", err)
	}
	if env.RunInterval.String() != "15m0s" {
		t.Fatalf("RunInterval = %s, want 15m0s", env.RunInterval)
	}
}

func TestLoadEnvRejectsInvalidInterval(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "token")
	t.Setenv("RUN_INTERVAL", "banana")

	_, err := LoadEnv()
	if err == nil || !strings.Contains(err.Error(), "invalid RUN_INTERVAL") {
		t.Fatalf("LoadEnv error = %v, want invalid RUN_INTERVAL error", err)
	}
}
