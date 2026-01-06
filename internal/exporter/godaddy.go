package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type GoDaddyExporter struct {
	apiKey    string
	apiSecret string
	client    *http.Client
}

func NewGoDaddyExporter() *GoDaddyExporter {
	return &GoDaddyExporter{
		apiKey:    strings.TrimSpace(os.Getenv("GODADDY_API_KEY")),
		apiSecret: strings.TrimSpace(os.Getenv("GODADDY_API_SECRET")),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *GoDaddyExporter) Name() string {
	return "godaddy"
}

func (e *GoDaddyExporter) IsConfigured() bool {
	return e.apiKey != "" && e.apiSecret != ""
}

func (e *GoDaddyExporter) ListDomains(ctx context.Context, opts ListOptions) ([]string, error) {
	domainInfos, err := e.getAllDomains(ctx)
	if err != nil {
		return nil, err
	}
	var domains []string
	for _, d := range domainInfos {
		// Skip cancelled domains unless explicitly requested
		if !opts.IncludeCancelled && d.Status != "ACTIVE" {
			continue
		}
		domains = append(domains, d.Domain)
	}
	return domains, nil
}

func (e *GoDaddyExporter) Export(ctx context.Context) ([]DomainResult, error) {
	domainInfos, err := e.getAllDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("get domains: %w", err)
	}

	var results []DomainResult
	for _, info := range domainInfos {
		// Skip expired or inactive domains
		if info.Status != "ACTIVE" {
			results = append(results, DomainResult{
				Domain: info.Domain,
				Status: StatusSkipped,
				Reason: strings.ToLower(info.Status),
			})
			continue
		}

		records, err := e.getDNSRecords(ctx, info.Domain)
		if err != nil {
			results = append(results, DomainResult{
				Domain: info.Domain,
				Status: StatusError,
				Reason: err.Error(),
			})
			continue
		}

		data := &DomainData{
			Domain:   info.Domain,
			Records:  e.convertRecords(records),
			Metadata: NewMetadata("godaddy"),
		}
		results = append(results, DomainResult{
			Domain:  info.Domain,
			Status:  StatusOK,
			Records: len(data.Records),
			Data:    data,
		})
	}

	return results, nil
}

type gdDomainInfo struct {
	Domain string `json:"domain"`
	Status string `json:"status"`
}

type gdRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Data     string `json:"data"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority"`
	Weight   *int   `json:"weight"`
	Port     *int   `json:"port"`
}

func (e *GoDaddyExporter) getAllDomains(ctx context.Context) ([]gdDomainInfo, error) {
	var allDomains []gdDomainInfo
	marker := ""

	for {
		url := "https://api.godaddy.com/v1/domains?limit=100"
		if marker != "" {
			url += "&marker=" + marker
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", e.apiKey, e.apiSecret))
		req.Header.Set("Accept", "application/json")

		resp, err := e.client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		var domainInfos []gdDomainInfo
		if err := json.Unmarshal(body, &domainInfos); err != nil {
			return nil, err
		}

		if len(domainInfos) == 0 {
			break
		}

		allDomains = append(allDomains, domainInfos...)

		if len(domainInfos) < 100 {
			break
		}

		marker = domainInfos[len(domainInfos)-1].Domain
	}

	return allDomains, nil
}

func (e *GoDaddyExporter) getDNSRecords(ctx context.Context, domain string) ([]gdRecord, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records", domain), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", e.apiKey, e.apiSecret))
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var records []gdRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, err
	}

	return records, nil
}

func (e *GoDaddyExporter) convertRecords(records []gdRecord) []DNSRecord {
	var result []DNSRecord
	for _, r := range records {
		rec := DNSRecord{
			Name:     r.Name,
			Type:     r.Type,
			Value:    r.Data,
			TTL:      r.TTL,
			Priority: r.Priority,
			Weight:   r.Weight,
			Port:     r.Port,
		}
		result = append(result, rec)
	}
	return result
}
