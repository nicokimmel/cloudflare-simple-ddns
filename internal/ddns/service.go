package ddns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"cloudflare-simple-ddns/internal/cloudflare"
	"cloudflare-simple-ddns/internal/config"
	"cloudflare-simple-ddns/internal/ip"
)

type CloudflareAPI interface {
	ListZones(ctx context.Context) ([]cloudflare.Zone, error)
	ListDNSRecords(ctx context.Context, zoneID, recordType, name string) ([]cloudflare.DNSRecord, error)
	CreateDNSRecord(ctx context.Context, zoneID string, request cloudflare.UpsertRecordRequest) (cloudflare.DNSRecord, error)
	UpdateDNSRecord(ctx context.Context, zoneID, recordID string, request cloudflare.UpsertRecordRequest) (cloudflare.DNSRecord, error)
}

type IPDetector interface {
	DetectIPv4(ctx context.Context) (net.IP, error)
	DetectIPv6(ctx context.Context) (net.IP, error)
}

type Service struct {
	ConfigPath string
	Logger     *slog.Logger
	CF         CloudflareAPI
	IP         IPDetector
}

type Summary struct {
	Total     int
	Created   int
	Updated   int
	Unchanged int
	Failed    int
}

func (s *Service) RunSync(ctx context.Context) Summary {
	logger := s.logger()

	entries, err := config.LoadEntries(s.ConfigPath)
	if err != nil {
		logger.Error("sync failed", "reason", err.Error())
		summary := Summary{Failed: 1}
		logSummary(logger, summary)
		return summary
	}

	logger.Info("loaded config", "entries", len(entries), "path", s.ConfigPath)

	zones, err := s.CF.ListZones(ctx)
	if err != nil {
		logger.Error("sync failed", "reason", fmt.Sprintf("load Cloudflare zones: %v", err))
		summary := Summary{Total: len(entries), Failed: len(entries)}
		logSummary(logger, summary)
		return summary
	}

	ipv4, ipv6 := s.detectIPs(ctx, entries)
	summary := Summary{Total: len(entries)}

	for _, entry := range entries {
		status, err := s.syncEntry(ctx, entry, zones, ipv4, ipv6)
		if err != nil {
			summary.Failed++
			logger.Error("failed", "domain", entry.Domain, "reason", err.Error())
			continue
		}

		switch status {
		case statusCreated:
			summary.Created++
		case statusUpdated:
			summary.Updated++
		default:
			summary.Unchanged++
		}
	}

	logSummary(logger, summary)

	return summary
}

type syncStatus string

const (
	statusCreated syncStatus = "created"
	statusUpdated syncStatus = "updated"
	statusSame    syncStatus = "unchanged"
)

func (s *Service) syncEntry(ctx context.Context, entry config.Entry, zones []cloudflare.Zone, ipv4, ipv6 net.IP) (syncStatus, error) {
	logger := s.logger()

	zone, ok := MatchZone(entry.Domain, zones)
	if !ok {
		return "", errors.New("no matching Cloudflare zone found")
	}
	logger.Info("zone matched", "domain", entry.Domain, "zone", zone.Name)

	recordType, err := config.RecordType(entry.IPVersion)
	if err != nil {
		return "", err
	}

	publicIP, err := selectIP(entry.IPVersion, ipv4, ipv6)
	if err != nil {
		return "", err
	}

	records, err := s.CF.ListDNSRecords(ctx, zone.ID, recordType, entry.Domain)
	if err != nil {
		return "", fmt.Errorf("list dns records: %w", err)
	}

	matching := filterMatchingRecords(records, recordType, entry.Domain)
	request := cloudflare.UpsertRecordRequest{
		Type:    recordType,
		Name:    entry.Domain,
		Content: publicIP.String(),
		TTL:     entry.TTL,
		Proxied: entry.Proxied,
	}

	switch len(matching) {
	case 0:
		if _, err := s.CF.CreateDNSRecord(ctx, zone.ID, request); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "cname") {
				return "", fmt.Errorf("create dns record failed due to CNAME conflict: %w", err)
			}
			return "", fmt.Errorf("create dns record: %w", err)
		}
		logger.Info("created", "domain", entry.Domain, "type", recordType, "ip", publicIP.String(), "proxied", entry.Proxied)
		return statusCreated, nil
	case 1:
		record := matching[0]
		if isUnchanged(record, request) {
			logger.Info("unchanged", "domain", entry.Domain, "type", recordType, "ip", publicIP.String(), "proxied", entry.Proxied)
			return statusSame, nil
		}

		if _, err := s.CF.UpdateDNSRecord(ctx, zone.ID, record.ID, request); err != nil {
			return "", fmt.Errorf("update dns record: %w", err)
		}
		logger.Info("updated", "domain", entry.Domain, "type", recordType, "old_ip", record.Content, "new_ip", publicIP.String(), "proxied", entry.Proxied)
		return statusUpdated, nil
	default:
		logger.Warn("multiple matching DNS records found", "domain", entry.Domain, "type", recordType, "count", len(matching))
		updated := false
		for _, record := range matching {
			if isUnchanged(record, request) {
				continue
			}
			if _, err := s.CF.UpdateDNSRecord(ctx, zone.ID, record.ID, request); err != nil {
				return "", fmt.Errorf("update dns record %s: %w", record.ID, err)
			}
			updated = true
		}
		if updated {
			logger.Info("updated", "domain", entry.Domain, "type", recordType, "new_ip", publicIP.String(), "proxied", entry.Proxied)
			return statusUpdated, nil
		}
		logger.Info("unchanged", "domain", entry.Domain, "type", recordType, "ip", publicIP.String(), "proxied", entry.Proxied)
		return statusSame, nil
	}
}

func MatchZone(domain string, zones []cloudflare.Zone) (cloudflare.Zone, bool) {
	normalizedDomain := strings.TrimPrefix(config.NormalizeDomain(domain), "*.")
	var matched cloudflare.Zone
	found := false

	for _, zone := range zones {
		zoneName := config.NormalizeDomain(zone.Name)
		if normalizedDomain == zoneName || strings.HasSuffix(normalizedDomain, "."+zoneName) {
			if !found || len(zoneName) > len(matched.Name) {
				matched = zone
				found = true
			}
		}
	}

	return matched, found
}

func filterMatchingRecords(records []cloudflare.DNSRecord, recordType, domain string) []cloudflare.DNSRecord {
	wantName := cloudflare.NormalizeRecordName(domain)
	wantType := strings.ToUpper(recordType)
	filtered := make([]cloudflare.DNSRecord, 0, len(records))
	for _, record := range records {
		if strings.ToUpper(record.Type) == wantType && cloudflare.NormalizeRecordName(record.Name) == wantName {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func isUnchanged(record cloudflare.DNSRecord, request cloudflare.UpsertRecordRequest) bool {
	return record.Content == request.Content && record.Proxied == request.Proxied && record.TTL == request.TTL
}

func selectIP(version int, ipv4, ipv6 net.IP) (net.IP, error) {
	switch version {
	case 4:
		if ipv4 == nil {
			return nil, errors.New("public IPv4 could not be determined")
		}
		return ipv4, nil
	case 6:
		if ipv6 == nil {
			return nil, errors.New("public IPv6 could not be determined")
		}
		return ipv6, nil
	default:
		return nil, fmt.Errorf("unsupported ip_version %d", version)
	}
}

func (s *Service) detectIPs(ctx context.Context, entries []config.Entry) (net.IP, net.IP) {
	logger := s.logger()
	var ipv4, ipv6 net.IP

	if config.NeedsIPv4(entries) {
		requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		detected, err := s.IP.DetectIPv4(requestCtx)
		if err != nil {
			logger.Error("failed to detect public IPv4", "reason", err.Error())
		} else {
			ipv4 = detected
			logger.Info("detected public ipv4", "ip", ipv4.String())
		}
	}

	if config.NeedsIPv6(entries) {
		requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		detected, err := s.IP.DetectIPv6(requestCtx)
		if err != nil {
			logger.Error("failed to detect public IPv6", "reason", err.Error())
		} else {
			ipv6 = detected
			logger.Info("detected public ipv6", "ip", ipv6.String())
		}
	}

	return ipv4, ipv6
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func logSummary(logger *slog.Logger, summary Summary) {
	logger.Info("summary",
		"total", summary.Total,
		"created", summary.Created,
		"updated", summary.Updated,
		"unchanged", summary.Unchanged,
		"failed", summary.Failed,
	)
}

func NewDefaultService(token string, logger *slog.Logger) *Service {
	return &Service{
		ConfigPath: config.ConfigPath,
		Logger:     logger,
		CF:         cloudflare.NewClient(token, 15*time.Second),
		IP:         ip.NewDetector(10 * time.Second),
	}
}
