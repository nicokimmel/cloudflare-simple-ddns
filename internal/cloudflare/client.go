package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "https://api.cloudflare.com/client/v4"

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

type ListResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
	TotalPages int `json:"total_pages"`
}

type listResponse[T any] struct {
	Success    bool           `json:"success"`
	Errors     []apiError     `json:"errors"`
	Messages   []any          `json:"messages"`
	Result     []T            `json:"result"`
	ResultInfo ListResultInfo `json:"result_info"`
}

type objectResponse[T any] struct {
	Success  bool       `json:"success"`
	Errors   []apiError `json:"errors"`
	Messages []any      `json:"messages"`
	Result   T          `json:"result"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type UpsertRecordRequest struct {
	Type    string `json:"type,omitempty"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

var hexTokenPattern = regexp.MustCompile(`^[0-9a-fA-F]+$`)

func NewClient(token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: BaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) ListZones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	page := 1

	for {
		var response listResponse[Zone]
		endpoint := fmt.Sprintf("%s/zones?page=%d&per_page=50", c.baseURL, page)
		if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
			return nil, err
		}

		zones = append(zones, response.Result...)
		if response.ResultInfo.TotalPages <= page || response.ResultInfo.TotalPages == 0 {
			break
		}
		page++
	}

	return zones, nil
}

func (c *Client) VerifyToken(ctx context.Context) error {
	var response objectResponse[map[string]any]
	endpoint := fmt.Sprintf("%s/user/tokens/verify", c.baseURL)
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		if LooksLikeGlobalAPIKey(c.token) {
			return fmt.Errorf("%w; CLOUDFLARE_API_TOKEN looks like a Global API Key, but this application requires a Cloudflare API Token", err)
		}
		return err
	}
	return nil
}

func (c *Client) ListDNSRecords(ctx context.Context, zoneID, recordType, name string) ([]DNSRecord, error) {
	var records []DNSRecord
	page := 1

	for {
		query := url.Values{}
		query.Set("type", recordType)
		query.Set("name", name)
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", "50")

		endpoint := fmt.Sprintf("%s/zones/%s/dns_records?%s", c.baseURL, zoneID, query.Encode())
		var response listResponse[DNSRecord]
		if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
			return nil, err
		}

		records = append(records, response.Result...)
		if response.ResultInfo.TotalPages <= page || response.ResultInfo.TotalPages == 0 {
			break
		}
		page++
	}

	return records, nil
}

func (c *Client) CreateDNSRecord(ctx context.Context, zoneID string, request UpsertRecordRequest) (DNSRecord, error) {
	var response objectResponse[DNSRecord]
	endpoint := fmt.Sprintf("%s/zones/%s/dns_records", c.baseURL, zoneID)
	if err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response); err != nil {
		return DNSRecord{}, err
	}
	return response.Result, nil
}

func (c *Client) UpdateDNSRecord(ctx context.Context, zoneID, recordID string, request UpsertRecordRequest) (DNSRecord, error) {
	var response objectResponse[DNSRecord]
	endpoint := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, recordID)
	if err := c.doJSON(ctx, http.MethodPatch, endpoint, request, &response); err != nil {
		return DNSRecord{}, err
	}
	return response.Result, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build cloudflare request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare request %s %s failed: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read cloudflare response: %w", err)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode cloudflare response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare request %s %s failed: %s: %s", method, endpoint, resp.Status, collectErrors(data))
	}

	switch v := out.(type) {
	case *listResponse[Zone]:
		if !v.Success {
			return fmt.Errorf("cloudflare request %s %s failed: %s", method, endpoint, errorsText(v.Errors))
		}
	case *listResponse[DNSRecord]:
		if !v.Success {
			return fmt.Errorf("cloudflare request %s %s failed: %s", method, endpoint, errorsText(v.Errors))
		}
	case *objectResponse[DNSRecord]:
		if !v.Success {
			return fmt.Errorf("cloudflare request %s %s failed: %s", method, endpoint, errorsText(v.Errors))
		}
	}

	return nil
}

func collectErrors(data []byte) string {
	var envelope struct {
		Errors []apiError `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return strings.TrimSpace(string(data))
	}
	return errorsText(envelope.Errors)
}

func errorsText(errors []apiError) string {
	if len(errors) == 0 {
		return "unknown error"
	}

	parts := make([]string, 0, len(errors))
	for _, item := range errors {
		if item.Code == 0 {
			parts = append(parts, item.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", item.Code, item.Message))
	}
	return strings.Join(parts, "; ")
}

func NormalizeRecordName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func LooksLikeGlobalAPIKey(value string) bool {
	trimmed := strings.TrimSpace(value)
	return (len(trimmed) == 37 || len(trimmed) == 40) && hexTokenPattern.MatchString(trimmed)
}
