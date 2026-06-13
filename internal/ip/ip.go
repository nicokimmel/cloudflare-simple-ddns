package ip

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	IPv4URL = "https://ipv4.icanhazip.com"
	IPv6URL = "https://ipv6.icanhazip.com"
)

type Detector struct {
	Client  *http.Client
	Timeout time.Duration
}

func NewDetector(timeout time.Duration) *Detector {
	return &Detector{
		Client:  &http.Client{Timeout: timeout},
		Timeout: timeout,
	}
}

func (d *Detector) DetectIPv4(ctx context.Context) (net.IP, error) {
	return d.detect(ctx, IPv4URL, 4)
}

func (d *Detector) DetectIPv6(ctx context.Context) (net.IP, error) {
	return d.detect(ctx, IPv6URL, 6)
}

func (d *Detector) detect(ctx context.Context, url string, version int) (net.IP, error) {
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: d.Timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request public ip from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request public ip from %s: unexpected status %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return nil, fmt.Errorf("read public ip response from %s: %w", url, err)
	}

	raw := strings.TrimSpace(string(body))
	parsed, err := Validate(raw, version)
	if err != nil {
		return nil, fmt.Errorf("validate public ip response from %s: %w", url, err)
	}

	return parsed, nil
}

func Validate(raw string, version int) (net.IP, error) {
	parsed := net.ParseIP(strings.TrimSpace(raw))
	if parsed == nil {
		return nil, fmt.Errorf("invalid ip address %q", raw)
	}

	switch version {
	case 4:
		if ipv4 := parsed.To4(); ipv4 != nil {
			return ipv4, nil
		}
		return nil, fmt.Errorf("expected IPv4 address, got %q", raw)
	case 6:
		if parsed.To4() != nil {
			return nil, fmt.Errorf("expected IPv6 address, got IPv4 %q", raw)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported ip version %d", version)
	}
}
