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

type CloudflareExporter struct {
	apiToken  string
	accountID string
	client    *http.Client
}

func NewCloudflareExporter() *CloudflareExporter {
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if apiToken == "" {
		apiToken = os.Getenv("TF_VAR_cloudflare_api_token")
	}

	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if accountID == "" {
		accountID = os.Getenv("TF_VAR_cloudflare_account_id")
	}

	return &CloudflareExporter{
		apiToken:  apiToken,
		accountID: accountID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *CloudflareExporter) Name() string {
	return "cloudflare"
}

func (e *CloudflareExporter) IsConfigured() bool {
	return e.apiToken != ""
}

func (e *CloudflareExporter) ListDomains(ctx context.Context, opts ListOptions) ([]string, error) {
	zones, err := e.getZones(ctx)
	if err != nil {
		return nil, err
	}
	var domains []string
	for _, z := range zones {
		domains = append(domains, z.Name)
	}
	return domains, nil
}

func (e *CloudflareExporter) Export(ctx context.Context) ([]DomainResult, error) {
	zones, err := e.getZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("get zones: %w", err)
	}

	var results []DomainResult
	for _, zone := range zones {
		records, err := e.getDNSRecords(ctx, zone.ID)
		if err != nil {
			results = append(results, DomainResult{
				Domain: zone.Name,
				Status: StatusError,
				Reason: err.Error(),
			})
			continue
		}

		metadata := NewMetadata("cloudflare")
		if e.accountID != "" {
			metadata["account_id"] = e.accountID
		}

		data := &DomainData{
			Domain:   zone.Name,
			ZoneID:   zone.ID,
			Records:  e.convertRecords(zone.Name, records),
			Metadata: metadata,
		}
		results = append(results, DomainResult{
			Domain:  zone.Name,
			Status:  StatusOK,
			Records: len(data.Records),
			Data:    data,
		})
	}

	return results, nil
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfResponse struct {
	Success bool `json:"success"`
	Result  json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type cfRecord struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Content  string  `json:"content"`
	TTL      int     `json:"ttl"`
	Proxied  *bool   `json:"proxied"`
	Priority *int    `json:"priority"`
	Comment  string  `json:"comment"`
}

func (e *CloudflareExporter) getZones(ctx context.Context) ([]cfZone, error) {
	var allZones []cfZone
	page := 1

	for {
		req, err := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?page=%d&per_page=50", page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+e.apiToken)
		req.Header.Set("Content-Type", "application/json")

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

		var cfResp cfResponse
		if err := json.Unmarshal(body, &cfResp); err != nil {
			return nil, err
		}

		if !cfResp.Success {
			return nil, fmt.Errorf("API error: %v", cfResp.Errors)
		}

		var zones []cfZone
		if err := json.Unmarshal(cfResp.Result, &zones); err != nil {
			return nil, err
		}

		allZones = append(allZones, zones...)

		if page >= cfResp.ResultInfo.TotalPages {
			break
		}
		page++
	}

	return allZones, nil
}

func (e *CloudflareExporter) getDNSRecords(ctx context.Context, zoneID string) ([]cfRecord, error) {
	var allRecords []cfRecord
	page := 1

	for {
		req, err := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?page=%d&per_page=100", zoneID, page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+e.apiToken)
		req.Header.Set("Content-Type", "application/json")

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

		var cfResp cfResponse
		if err := json.Unmarshal(body, &cfResp); err != nil {
			return nil, err
		}

		if !cfResp.Success {
			return nil, fmt.Errorf("API error: %v", cfResp.Errors)
		}

		var records []cfRecord
		if err := json.Unmarshal(cfResp.Result, &records); err != nil {
			return nil, err
		}

		allRecords = append(allRecords, records...)

		if page >= cfResp.ResultInfo.TotalPages {
			break
		}
		page++
	}

	return allRecords, nil
}

func (e *CloudflareExporter) convertRecords(zoneName string, records []cfRecord) []DNSRecord {
	var result []DNSRecord
	for _, r := range records {
		name := r.Name
		if name == zoneName {
			name = "@"
		} else if strings.HasSuffix(name, "."+zoneName) {
			name = name[:len(name)-len(zoneName)-1]
		}

		rec := DNSRecord{
			Name:     name,
			Type:     r.Type,
			Value:    r.Content,
			TTL:      r.TTL,
			Proxied:  r.Proxied,
			Priority: r.Priority,
		}

		if r.Comment != "" {
			rec.Comment = r.Comment
		}

		result = append(result, rec)
	}
	return result
}
