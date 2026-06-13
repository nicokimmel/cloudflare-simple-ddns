package cloudflare

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListZonesParsesPaginatedResponse(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones" {
			t.Fatalf("path = %s, want /zones", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []map[string]any{
				{"id": "zone-1", "name": "example.com"},
			},
			"result_info": map[string]any{
				"page":        1,
				"per_page":    50,
				"count":       1,
				"total_count": 1,
				"total_pages": 1,
			},
		})
	}))

	client := &Client{
		baseURL:    server.URL,
		token:      "token",
		httpClient: &http.Client{Timeout: time.Second},
	}

	zones, err := client.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones error = %v", err)
	}
	if len(zones) != 1 || zones[0].Name != "example.com" {
		t.Fatalf("zones = %#v, want single example.com zone", zones)
	}
}

func TestListDNSRecordsParsesResponse(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "type=A") {
			t.Fatalf("query = %s, want type=A", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []map[string]any{
				{"id": "record-1", "type": "A", "name": "app.example.com", "content": "1.2.3.4", "proxied": false, "ttl": 1},
			},
			"result_info": map[string]any{
				"page":        1,
				"per_page":    50,
				"count":       1,
				"total_count": 1,
				"total_pages": 1,
			},
		})
	}))

	client := &Client{
		baseURL:    server.URL,
		token:      "token",
		httpClient: &http.Client{Timeout: time.Second},
	}

	records, err := client.ListDNSRecords(context.Background(), "zone-1", "A", "app.example.com")
	if err != nil {
		t.Fatalf("ListDNSRecords error = %v", err)
	}
	if len(records) != 1 || records[0].ID != "record-1" {
		t.Fatalf("records = %#v, want single parsed record", records)
	}
}

func TestCreateReturnsCloudflareError(t *testing.T) {
	t.Parallel()

	server := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors": []map[string]any{
				{"code": 9005, "message": "CNAME conflict"},
			},
		})
	}))

	client := &Client{
		baseURL:    server.URL,
		token:      "token",
		httpClient: &http.Client{Timeout: time.Second},
	}

	_, err := client.CreateDNSRecord(context.Background(), "zone-1", UpsertRecordRequest{
		Type: "A", Name: "example.com", Content: "1.2.3.4", TTL: 1, Proxied: false,
	})
	if err == nil || !strings.Contains(err.Error(), "CNAME conflict") {
		t.Fatalf("CreateDNSRecord error = %v, want CNAME conflict", err)
	}
}

func TestLooksLikeGlobalAPIKey(t *testing.T) {
	t.Parallel()

	if !LooksLikeGlobalAPIKey("c894ccffb53b2b6bbd5d9644ccb0c7d78cfe0") {
		t.Fatal("LooksLikeGlobalAPIKey returned false, want true")
	}
	if LooksLikeGlobalAPIKey("this-is-an-api-token-not-a-global-key") {
		t.Fatal("LooksLikeGlobalAPIKey returned true, want false")
	}
}

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping httptest server setup in restricted environment: %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	t.Cleanup(server.Close)
	return server
}
