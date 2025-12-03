package registrar

import (
	"context"
)

// Registrar interface for updating nameservers at domain registrars
type Registrar interface {
	Name() string
	IsConfigured() bool
	SetNameservers(ctx context.Context, domain string, nameservers []string) (*NameserverResult, error)
	GetNameservers(ctx context.Context, domain string) ([]string, error)
}

// Common Cloudflare nameservers (examples - actual ones are assigned per zone)
var CloudflareNameservers = []string{
	"ns1.cloudflare.com",
	"ns2.cloudflare.com",
}
