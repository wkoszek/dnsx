package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

type PorkbunExporter struct {
	apiKey    string
	secretKey string
	client    *http.Client
}

func NewPorkbunExporter() *PorkbunExporter {
	return &PorkbunExporter{
		apiKey:    os.Getenv("PORKBUN_API_KEY"),
		secretKey: os.Getenv("PORKBUN_SECRET_KEY"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *PorkbunExporter) Name() string {
	return "porkbun"
}

func (e *PorkbunExporter) IsConfigured() bool {
	return e.apiKey != "" && e.secretKey != ""
}

func (e *PorkbunExporter) ListDomains(ctx context.Context, opts ListOptions) ([]string, error) {
	return e.getAllDomains(ctx)
}

func (e *PorkbunExporter) Export(ctx context.Context) ([]DomainResult, error) {
	domains, err := e.getAllDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("get domains: %w", err)
	}

	var results []DomainResult
	for _, domain := range domains {
		records, err := e.getDNSRecords(ctx, domain)
		if err != nil {
			results = append(results, DomainResult{
				Domain: domain,
				Status: StatusError,
				Reason: err.Error(),
			})
			continue
		}

		data := &DomainData{
			Domain:   domain,
			Records:  e.convertRecords(records),
			Metadata: NewMetadata("porkbun"),
		}
		results = append(results, DomainResult{
			Domain:  domain,
			Status:  StatusOK,
			Records: len(data.Records),
			Data:    data,
		})
	}

	return results, nil
}

type pbDomainResponse struct {
	Status  string `json:"status"`
	Domains []struct {
		Domain string `json:"domain"`
	} `json:"domains"`
}

type pbRecordResponse struct {
	Status  string     `json:"status"`
	Records []pbRecord `json:"records"`
}

type pbRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     string `json:"ttl"`
	Prio    string `json:"prio"`
}

func (e *PorkbunExporter) getAllDomains(ctx context.Context) ([]string, error) {
	payload := map[string]string{
		"apikey":       e.apiKey,
		"secretapikey": e.secretKey,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.porkbun.com/api/json/v3/domain/listAll", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var domainResp pbDomainResponse
	if err := json.Unmarshal(respBody, &domainResp); err != nil {
		return nil, err
	}

	if domainResp.Status != "SUCCESS" {
		return nil, fmt.Errorf("API returned status: %s", domainResp.Status)
	}

	var domains []string
	for _, d := range domainResp.Domains {
		domains = append(domains, d.Domain)
	}

	return domains, nil
}

func (e *PorkbunExporter) getDNSRecords(ctx context.Context, domain string) ([]pbRecord, error) {
	payload := map[string]string{
		"apikey":       e.apiKey,
		"secretapikey": e.secretKey,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.porkbun.com/api/json/v3/dns/retrieve/%s", domain),
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var recordResp pbRecordResponse
	if err := json.Unmarshal(respBody, &recordResp); err != nil {
		return nil, err
	}

	if recordResp.Status != "SUCCESS" {
		return nil, fmt.Errorf("API returned status: %s", recordResp.Status)
	}

	return recordResp.Records, nil
}

func (e *PorkbunExporter) convertRecords(records []pbRecord) []DNSRecord {
	var result []DNSRecord
	for _, r := range records {
		name := r.Name
		if name == "" {
			name = "@"
		}

		ttl, _ := strconv.Atoi(r.TTL)
		if ttl == 0 {
			ttl = 600
		}

		rec := DNSRecord{
			Name:  name,
			Type:  r.Type,
			Value: r.Content,
			TTL:   ttl,
		}

		if r.Prio != "" && r.Prio != "0" {
			if prio, err := strconv.Atoi(r.Prio); err == nil {
				rec.Priority = &prio
			}
		}

		result = append(result, rec)
	}
	return result
}
