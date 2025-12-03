package registrar

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

type PorkbunRegistrar struct {
	apiKey    string
	secretKey string
	client    *http.Client
}

func NewPorkbunRegistrar() *PorkbunRegistrar {
	return &PorkbunRegistrar{
		apiKey:    os.Getenv("PORKBUN_API_KEY"),
		secretKey: os.Getenv("PORKBUN_SECRET_KEY"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *PorkbunRegistrar) Name() string {
	return "porkbun"
}

func (r *PorkbunRegistrar) IsConfigured() bool {
	return r.apiKey != "" && r.secretKey != ""
}

// SetNameservers updates the nameservers for a domain at Porkbun
func (r *PorkbunRegistrar) SetNameservers(ctx context.Context, domain string, nameservers []string) (*NameserverResult, error) {
	payload := map[string]interface{}{
		"apikey":       r.apiKey,
		"secretapikey": r.secretKey,
		"ns":           nameservers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.porkbun.com/api/json/v3/domain/updateNs/%s", domain),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
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

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Status != "SUCCESS" {
		return &NameserverResult{
			Domain: domain,
			Status: "error",
			Error:  result.Message,
		}, nil
	}

	return &NameserverResult{
		Domain:      domain,
		Status:      "updated",
		Nameservers: nameservers,
	}, nil
}

// GetNameservers retrieves the current nameservers for a domain
func (r *PorkbunRegistrar) GetNameservers(ctx context.Context, domain string) ([]string, error) {
	payload := map[string]string{
		"apikey":       r.apiKey,
		"secretapikey": r.secretKey,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.porkbun.com/api/json/v3/domain/getNs/%s", domain),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
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

	var result struct {
		Status string   `json:"status"`
		Ns     []string `json:"ns"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Status != "SUCCESS" {
		return nil, fmt.Errorf("API returned status: %s", result.Status)
	}

	return result.Ns, nil
}
