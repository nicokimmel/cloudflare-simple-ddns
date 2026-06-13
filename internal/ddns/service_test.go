package ddns

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"

	"cloudflare-simple-ddns/internal/cloudflare"
)

func TestMatchZoneChoosesLongestMatch(t *testing.T) {
	t.Parallel()

	zone, ok := MatchZone("*.sub.example.com", []cloudflare.Zone{
		{ID: "1", Name: "example.com"},
		{ID: "2", Name: "sub.example.com"},
	})
	if !ok {
		t.Fatal("MatchZone returned ok=false, want true")
	}
	if zone.Name != "sub.example.com" {
		t.Fatalf("zone.Name = %q, want sub.example.com", zone.Name)
	}
}

func TestRunSyncFailsWithoutMatchingZone(t *testing.T) {
	t.Parallel()

	cf := &fakeCloudflare{
		zones: []cloudflare.Zone{{ID: "1", Name: "other.com"}},
	}
	service := &Service{
		ConfigPath: writeConfig(t, `[{"domain":"example.com","proxied":false,"ip_version":4}]`),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		CF:         cf,
		IP:         fakeIPDetector{ipv4: net.ParseIP("1.2.3.4")},
	}

	summary := service.RunSync(context.Background())
	if got, want := summary.Failed, 1; got != want {
		t.Fatalf("Failed = %d, want %d", got, want)
	}
}

func TestRunSyncUpdatesMultipleMatchingRecords(t *testing.T) {
	t.Parallel()

	cf := &fakeCloudflare{
		zones: []cloudflare.Zone{{ID: "zone-1", Name: "example.com"}},
		records: []cloudflare.DNSRecord{
			{ID: "1", Type: "A", Name: "app.example.com", Content: "1.1.1.1", Proxied: false, TTL: 1},
			{ID: "2", Type: "A", Name: "app.example.com", Content: "2.2.2.2", Proxied: false, TTL: 1},
		},
	}
	service := &Service{
		ConfigPath: writeConfig(t, `[{"domain":"app.example.com","proxied":true,"ip_version":4}]`),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		CF:         cf,
		IP:         fakeIPDetector{ipv4: net.ParseIP("9.9.9.9")},
	}

	summary := service.RunSync(context.Background())
	if got, want := summary.Updated, 1; got != want {
		t.Fatalf("Updated = %d, want %d", got, want)
	}
	if got, want := len(cf.updatedIDs), 2; got != want {
		t.Fatalf("updatedIDs = %d, want %d", got, want)
	}
}

func TestRunSyncCreatesRecord(t *testing.T) {
	t.Parallel()

	cf := &fakeCloudflare{
		zones: []cloudflare.Zone{{ID: "zone-1", Name: "example.com"}},
	}
	service := &Service{
		ConfigPath: writeConfig(t, `[{"domain":"example.com","proxied":false,"ip_version":4}]`),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		CF:         cf,
		IP:         fakeIPDetector{ipv4: net.ParseIP("1.2.3.4")},
	}

	summary := service.RunSync(context.Background())
	if got, want := summary.Created, 1; got != want {
		t.Fatalf("Created = %d, want %d", got, want)
	}
}

type fakeCloudflare struct {
	zones      []cloudflare.Zone
	records    []cloudflare.DNSRecord
	updatedIDs []string
}

func (f *fakeCloudflare) ListZones(context.Context) ([]cloudflare.Zone, error) {
	return f.zones, nil
}

func (f *fakeCloudflare) ListDNSRecords(context.Context, string, string, string) ([]cloudflare.DNSRecord, error) {
	return f.records, nil
}

func (f *fakeCloudflare) CreateDNSRecord(_ context.Context, _ string, request cloudflare.UpsertRecordRequest) (cloudflare.DNSRecord, error) {
	return cloudflare.DNSRecord{ID: "created", Type: request.Type, Name: request.Name, Content: request.Content}, nil
}

func (f *fakeCloudflare) UpdateDNSRecord(_ context.Context, _ string, recordID string, request cloudflare.UpsertRecordRequest) (cloudflare.DNSRecord, error) {
	f.updatedIDs = append(f.updatedIDs, recordID)
	return cloudflare.DNSRecord{ID: recordID, Type: request.Type, Name: request.Name, Content: request.Content}, nil
}

type fakeIPDetector struct {
	ipv4    net.IP
	ipv6    net.IP
	ipv4Err error
	ipv6Err error
}

func (f fakeIPDetector) DetectIPv4(context.Context) (net.IP, error) {
	if f.ipv4Err != nil {
		return nil, f.ipv4Err
	}
	return f.ipv4, nil
}

func (f fakeIPDetector) DetectIPv6(context.Context) (net.IP, error) {
	if f.ipv6Err != nil {
		return nil, f.ipv6Err
	}
	return f.ipv6, nil
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := t.TempDir() + "/domains.json"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	return path
}
