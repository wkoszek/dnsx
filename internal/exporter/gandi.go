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

type GandiExporter struct {
	apiKey string
	client *http.Client
}

func NewGandiExporter() *GandiExporter {
	return &GandiExporter{
		apiKey: strings.TrimSpace(os.Getenv("GANDI_API_KEY")),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *GandiExporter) Name() string {
	return "gandi"
}

func (e *GandiExporter) IsConfigured() bool {
	return e.apiKey != ""
}

func (e *GandiExporter) Export(ctx context.Context) ([]DomainResult, error) {
	domainInfos, err := e.getAllDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("get domains: %w", err)
	}

	var results []DomainResult
	for _, info := range domainInfos {
		// Skip domains not using Gandi LiveDNS
		if !info.isLiveDNS {
			results = append(results, DomainResult{
				Domain: info.fqdn,
				Status: StatusSkipped,
				Reason: "not-livedns",
			})
			continue
		}

		records, err := e.getDNSRecords(ctx, info.fqdn)
		if err != nil {
			results = append(results, DomainResult{
				Domain: info.fqdn,
				Status: StatusError,
				Reason: err.Error(),
			})
			continue
		}

		data := &DomainData{
			Domain:   info.fqdn,
			Records:  e.convertRecords(records),
			Metadata: NewMetadata("gandi"),
		}
		results = append(results, DomainResult{
			Domain:  info.fqdn,
			Status:  StatusOK,
			Records: len(data.Records),
			Data:    data,
		})
	}

	return results, nil
}

type gandiDomain struct {
	FQDN       string      `json:"fqdn"`
	Nameserver interface{} `json:"nameserver"`
}

type gandiDomainInfo struct {
	fqdn      string
	isLiveDNS bool
}

type gandiRecord struct {
	RRSetName   string   `json:"rrset_name"`
	RRSetType   string   `json:"rrset_type"`
	RRSetValues []string `json:"rrset_values"`
	RRSetTTL    int      `json:"rrset_ttl"`
}

func (e *GandiExporter) getAllDomains(ctx context.Context) ([]gandiDomainInfo, error) {
	var allDomains []gandiDomain
	page := 1

	for {
		req, err := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.gandi.net/v5/domain/domains?page=%d&per_page=100", page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
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

		var domains []gandiDomain
		if err := json.Unmarshal(body, &domains); err != nil {
			return nil, err
		}

		if len(domains) == 0 {
			break
		}

		allDomains = append(allDomains, domains...)

		if len(domains) < 100 {
			break
		}
		page++
	}

	var domainInfos []gandiDomainInfo
	for _, d := range allDomains {
		var nsCurrent string
		switch ns := d.Nameserver.(type) {
		case map[string]interface{}:
			if current, ok := ns["current"].(string); ok {
				nsCurrent = current
			}
		case string:
			nsCurrent = ns
		}

		domainInfos = append(domainInfos, gandiDomainInfo{
			fqdn:      d.FQDN,
			isLiveDNS: strings.Contains(nsCurrent, "livedns"),
		})
	}

	return domainInfos, nil
}

func (e *GandiExporter) getDNSRecords(ctx context.Context, domain string) ([]gandiRecord, error) {
	var allRecords []gandiRecord
	page := 1

	for {
		req, err := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.gandi.net/v5/livedns/domains/%s/records?page=%d&per_page=100", domain, page), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
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

		var records []gandiRecord
		if err := json.Unmarshal(body, &records); err != nil {
			return nil, err
		}

		if len(records) == 0 {
			break
		}

		allRecords = append(allRecords, records...)

		if len(records) < 100 {
			break
		}
		page++
	}

	return allRecords, nil
}

func (e *GandiExporter) convertRecords(records []gandiRecord) []DNSRecord {
	var result []DNSRecord
	for _, r := range records {
		for _, value := range r.RRSetValues {
			result = append(result, DNSRecord{
				Name:  r.RRSetName,
				Type:  r.RRSetType,
				Value: value,
				TTL:   r.RRSetTTL,
			})
		}
	}
	return result
}

