package exporter

import (
	"context"
	"time"
)

type DNSRecord struct {
	Name     string                 `yaml:"name"`
	Type     string                 `yaml:"type"`
	Value    string                 `yaml:"value"`
	TTL      int                    `yaml:"ttl"`
	Proxied  *bool                  `yaml:"proxied,omitempty"`
	Priority *int                   `yaml:"priority,omitempty"`
	Weight   *int                   `yaml:"weight,omitempty"`
	Port     *int                   `yaml:"port,omitempty"`
	Comment  string                 `yaml:"comment,omitempty"`
	Extra    map[string]interface{} `yaml:",inline,omitempty"`
}

type DomainData struct {
	Domain   string                 `yaml:"domain"`
	ZoneID   string                 `yaml:"zone_id,omitempty"`
	Records  []DNSRecord            `yaml:"records"`
	Metadata map[string]interface{} `yaml:"metadata"`
}

type DomainStatus string

const (
	StatusOK      DomainStatus = "ok"
	StatusSkipped DomainStatus = "skipped"
	StatusError   DomainStatus = "error"
)

type DomainResult struct {
	Domain  string
	Status  DomainStatus
	Reason  string // for skipped/error: reason or error message
	Records int    // record count for ok status
	Data    *DomainData
}

type Exporter interface {
	Name() string
	IsConfigured() bool
	Export(ctx context.Context) ([]DomainResult, error)
}

func NewMetadata(provider string) map[string]interface{} {
	return map[string]interface{}{
		"provider":    provider,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
	}
}
