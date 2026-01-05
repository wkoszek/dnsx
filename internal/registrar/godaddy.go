package registrar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type GoDaddyRegistrar struct {
	apiKey    string
	apiSecret string
	client    *http.Client
}

func NewGoDaddyRegistrar() *GoDaddyRegistrar {
	return &GoDaddyRegistrar{
		apiKey:    strings.TrimSpace(os.Getenv("GODADDY_API_KEY")),
		apiSecret: strings.TrimSpace(os.Getenv("GODADDY_API_SECRET")),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *GoDaddyRegistrar) Name() string {
	return "godaddy"
}

func (r *GoDaddyRegistrar) IsConfigured() bool {
	return r.apiKey != "" && r.apiSecret != ""
}

// SetNameservers updates the nameservers for a domain at GoDaddy
func (r *GoDaddyRegistrar) SetNameservers(ctx context.Context, domain string, nameservers []string) (*NameserverResult, error) {
	payload := map[string]any{
		"nameServers": nameservers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH",
		fmt.Sprintf("https://api.godaddy.com/v1/domains/%s", domain),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", r.apiKey, r.apiSecret))
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			return &NameserverResult{
				Domain: domain,
				Status: "error",
				Error:  errResp.Message,
			}, nil
		}
		return &NameserverResult{
			Domain: domain,
			Status: "error",
			Error:  fmt.Sprintf("API returned status %d", resp.StatusCode),
		}, nil
	}

	return &NameserverResult{
		Domain:      domain,
		Status:      "updated",
		Nameservers: nameservers,
	}, nil
}

// GetNameservers retrieves the current nameservers for a domain
func (r *GoDaddyRegistrar) GetNameservers(ctx context.Context, domain string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.godaddy.com/v1/domains/%s", domain), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", r.apiKey, r.apiSecret))
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var domainInfo struct {
		NameServers []string `json:"nameServers"`
	}
	if err := json.Unmarshal(body, &domainInfo); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return domainInfo.NameServers, nil
}
