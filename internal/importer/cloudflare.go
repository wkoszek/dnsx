package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type CloudflareImporter struct {
	apiToken  string
	apiKey    string // Global API Key
	apiEmail  string // Email for Global API Key auth
	accountID string
	client    *http.Client
}

func NewCloudflareImporter() *CloudflareImporter {
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if apiToken == "" {
		apiToken = os.Getenv("TF_VAR_cloudflare_api_token")
	}

	apiKey := os.Getenv("CLOUDFLARE_API_KEY")
	apiEmail := os.Getenv("CLOUDFLARE_EMAIL")

	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	if accountID == "" {
		accountID = os.Getenv("TF_VAR_cloudflare_account_id")
	}

	return &CloudflareImporter{
		apiToken:  apiToken,
		apiKey:    apiKey,
		apiEmail:  apiEmail,
		accountID: accountID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (i *CloudflareImporter) IsConfigured() bool {
	hasToken := i.apiToken != ""
	hasGlobalKey := i.apiKey != "" && i.apiEmail != ""
	return (hasToken || hasGlobalKey) && i.accountID != ""
}

func (i *CloudflareImporter) setAuthHeaders(req *http.Request) {
	if i.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+i.apiToken)
	} else {
		req.Header.Set("X-Auth-Key", i.apiKey)
		req.Header.Set("X-Auth-Email", i.apiEmail)
	}
}

type ZoneResult struct {
	Domain      string
	ZoneID      string
	Nameservers []string
	Status      string
	Error       string
}

type cfResponse struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

type cfZoneResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	NameServers []string `json:"name_servers"`
	Status      string   `json:"status"`
}

// CreateZone creates a new zone in Cloudflare for the given domain
func (i *CloudflareImporter) CreateZone(ctx context.Context, domain string, jumpStart bool) (*ZoneResult, error) {
	payload := map[string]interface{}{
		"name": domain,
		"account": map[string]string{
			"id": i.accountID,
		},
		"jump_start": jumpStart,
		"type":       "full",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.cloudflare.com/client/v4/zones", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	i.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var cfResp cfResponse
	if err := json.Unmarshal(respBody, &cfResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !cfResp.Success {
		// Check if zone already exists
		for _, e := range cfResp.Errors {
			if e.Code == 1061 { // Zone already exists
				return i.getExistingZone(ctx, domain)
			}
		}
		if len(cfResp.Errors) > 0 {
			return &ZoneResult{
				Domain: domain,
				Status: "error",
				Error:  cfResp.Errors[0].Message,
			}, nil
		}
		return &ZoneResult{
			Domain: domain,
			Status: "error",
			Error:  "unknown error",
		}, nil
	}

	var zone cfZoneResult
	if err := json.Unmarshal(cfResp.Result, &zone); err != nil {
		return nil, fmt.Errorf("unmarshal zone: %w", err)
	}

	return &ZoneResult{
		Domain:      zone.Name,
		ZoneID:      zone.ID,
		Nameservers: zone.NameServers,
		Status:      "created",
	}, nil
}

func (i *CloudflareImporter) getExistingZone(ctx context.Context, domain string) (*ZoneResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", domain), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	i.setAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var cfResp cfResponse
	if err := json.Unmarshal(respBody, &cfResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if !cfResp.Success {
		return nil, fmt.Errorf("API error: %v", cfResp.Errors)
	}

	var zones []cfZoneResult
	if err := json.Unmarshal(cfResp.Result, &zones); err != nil {
		return nil, fmt.Errorf("unmarshal zones: %w", err)
	}

	if len(zones) == 0 {
		return &ZoneResult{
			Domain: domain,
			Status: "error",
			Error:  "zone not found",
		}, nil
	}

	return &ZoneResult{
		Domain:      zones[0].Name,
		ZoneID:      zones[0].ID,
		Nameservers: zones[0].NameServers,
		Status:      "exists",
	}, nil
}

// ImportDomains creates zones for multiple domains
func (i *CloudflareImporter) ImportDomains(ctx context.Context, domains []string, jumpStart bool) ([]ZoneResult, error) {
	var results []ZoneResult

	for _, domain := range domains {
		result, err := i.CreateZone(ctx, domain, jumpStart)
		if err != nil {
			results = append(results, ZoneResult{
				Domain: domain,
				Status: "error",
				Error:  err.Error(),
			})
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}
