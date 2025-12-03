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

type GandiRegistrar struct {
	apiKey string
	client *http.Client
}

func NewGandiRegistrar() *GandiRegistrar {
	return &GandiRegistrar{
		apiKey: strings.TrimSpace(os.Getenv("GANDI_API_KEY")),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *GandiRegistrar) Name() string {
	return "gandi"
}

func (r *GandiRegistrar) IsConfigured() bool {
	return r.apiKey != ""
}

type NameserverResult struct {
	Domain      string
	Status      string // "updated", "error"
	Nameservers []string
	Error       string
}

// SetNameservers updates the nameservers for a domain at Gandi
func (r *GandiRegistrar) SetNameservers(ctx context.Context, domain string, nameservers []string) (*NameserverResult, error) {
	payload := map[string]interface{}{
		"nameservers": nameservers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH",
		fmt.Sprintf("https://api.gandi.net/v5/domain/domains/%s", domain),
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
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
			Message string `json:"message"`
			Errors  []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			errMsg := errResp.Message
			if len(errResp.Errors) > 0 {
				errMsg = errResp.Errors[0].Description
			}
			return &NameserverResult{
				Domain: domain,
				Status: "error",
				Error:  errMsg,
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
func (r *GandiRegistrar) GetNameservers(ctx context.Context, domain string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.gandi.net/v5/domain/domains/%s", domain), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
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
		Nameservers []string `json:"nameservers"`
	}
	if err := json.Unmarshal(body, &domainInfo); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return domainInfo.Nameservers, nil
}
