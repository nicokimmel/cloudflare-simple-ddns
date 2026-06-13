package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	ConfigPath = "/config/domains.json"
	MinTTL     = 60
	MaxTTL     = 86400
)

type Entry struct {
	Domain    string `json:"domain"`
	Proxied   bool   `json:"proxied"`
	IPVersion int    `json:"ip_version"`
	TTL       int    `json:"ttl,omitempty"`
}

type Env struct {
	CloudflareAPIToken string
	RunInterval        time.Duration
}

func LoadEnv() (Env, error) {
	keys := []string{"CLOUDFLARE_API_TOKEN", "RUN_INTERVAL"}
	allowed := map[string]struct{}{
		"CLOUDFLARE_API_TOKEN": {},
		"RUN_INTERVAL":         {},
	}

	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, ok := allowed[key]; ok {
			continue
		}
		if strings.HasPrefix(key, "CLOUDFLARE_") || key == "RUN_INTERVAL" {
			return Env{}, fmt.Errorf("unsupported environment variable %q", key)
		}
	}

	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return Env{}, fmt.Errorf("missing required environment variable %s", key)
		}
	}

	interval, err := time.ParseDuration(strings.TrimSpace(os.Getenv("RUN_INTERVAL")))
	if err != nil {
		return Env{}, fmt.Errorf("invalid RUN_INTERVAL: %w", err)
	}
	if interval <= 0 {
		return Env{}, errors.New("RUN_INTERVAL must be greater than zero")
	}

	return Env{
		CloudflareAPIToken: strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")),
		RunInterval:        interval,
	}, nil
}

func LoadEntries(path string) ([]Entry, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var entries []Entry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("parse config json: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("parse config json: unexpected trailing data")
	}

	if len(entries) == 0 {
		return nil, errors.New("config must contain at least one entry")
	}

	if err := ValidateEntries(entries); err != nil {
		return nil, err
	}

	normalized := make([]Entry, len(entries))
	for i, entry := range entries {
		entry.Domain = NormalizeDomain(entry.Domain)
		if entry.TTL == 0 {
			entry.TTL = 1
		}
		normalized[i] = entry
	}

	return normalized, nil
}

func ValidateEntries(entries []Entry) error {
	seen := make(map[string]struct{}, len(entries))

	for i, entry := range entries {
		domain := NormalizeDomain(entry.Domain)
		if domain == "" {
			return fmt.Errorf("entry %d: domain must not be empty", i)
		}

		if entry.IPVersion != 4 && entry.IPVersion != 6 {
			return fmt.Errorf("entry %d: ip_version must be 4 or 6", i)
		}

		if entry.TTL != 0 && entry.TTL != 1 && (entry.TTL < MinTTL || entry.TTL > MaxTTL) {
			return fmt.Errorf("entry %d: ttl must be 1 or between %d and %d", i, MinTTL, MaxTTL)
		}

		key := fmt.Sprintf("%s|%d", domain, entry.IPVersion)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("entry %d: duplicate domain/ip_version combination for %s", i, domain)
		}
		seen[key] = struct{}{}
	}

	return nil
}

func NormalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}

func NeedsIPv4(entries []Entry) bool {
	return slices.ContainsFunc(entries, func(entry Entry) bool { return entry.IPVersion == 4 })
}

func NeedsIPv6(entries []Entry) bool {
	return slices.ContainsFunc(entries, func(entry Entry) bool { return entry.IPVersion == 6 })
}

func RecordType(ipVersion int) (string, error) {
	switch ipVersion {
	case 4:
		return "A", nil
	case 6:
		return "AAAA", nil
	default:
		return "", fmt.Errorf("unsupported ip_version %d", ipVersion)
	}
}
