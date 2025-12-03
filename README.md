# 🌐 dnsx - Your DNS records deserve a backup too!* 💾

A minimalistic DNS management utility for backing up and migrating DNS records across multiple providers.
The idea is to get YAML from all providers, and then use it as ground truth with Terraform.

A fast, minimal DNS utility that backs up your DNS records from multiple providers to YAML. Because losing DNS records is no fun. 😱

## ✨ Features

- 🔄 **Multi-provider support:**
  - ☁️ Cloudflare
  - 🦩 Gandi
  - 🟢 GoDaddy
  - 🐷 Porkbun
  - 🤝 PRs welcome for more!
- 📦 Export all providers with one command
- 📥 Import domains to Cloudflare (migrate from other providers)
- 🚚 Move domains between providers with one command
- 📋 List domains in any provider
- 📝 YAML output - easy to read, diff, and version control
- 🔐 Environment variable config - no config files to leak
- 📄 Handles pagination for large accounts
- 🎯 Machine-parseable output for scripting

## 🚀 Quick Start

```bash
go install github.com/wkoszek/dnsx/cmd/dnsx@latest
cd dnsx

# Build the binary
make build

# Install to ~/bin
make install

# Or install to a custom location
cp dnsx /usr/local/bin/
```

## Usage

```bash
# Export from a specific provider
dnsx export cloudflare
dnsx export gandi
dnsx export godaddy
dnsx export porkbun

# Export from all configured providers
dnsx export all

# Custom output directory
dnsx export cloudflare --outdir /tmp/dns-backup
dnsx export all --outdir ./backups

# Import domains to Cloudflare (creates zones)
dnsx import example.com
dnsx import domain1.com domain2.com domain3.com

# Generate Terraform from YAML
dnsx gen terraform terraform/

# Show help
dnsx --help
```

## Listing Domains

List all domains in a provider:

```bash
# List domains in Porkbun
dnsx ls porkbun

# List domains in Cloudflare
dnsx ls cloudflare

# List domains in Gandi
dnsx ls gandi

# With 1Password
op run --env-file=.env.1password -- dnsx ls porkbun
```

## Moving Domains Between Providers

The `mv` command migrates a domain from one provider to another. It creates the zone in the destination provider and updates nameservers at the source registrar:

```bash
# Move a domain from Porkbun to Cloudflare
dnsx mv porkbun/example.com cloudflare/

# Move a domain from Gandi to Cloudflare
dnsx mv gandi/example.com cloudflare/

# With 1Password
op run --env-file=.env.1password -- dnsx mv porkbun/example.com cloudflare/
```

This will:
1. Create a new zone in Cloudflare for the domain
2. Get the assigned Cloudflare nameservers
3. Update nameservers at the source registrar (Porkbun or Gandi)
4. Print YAML for adding to your configuration

## Setting Nameservers at Registrar

The `set ns` command updates nameservers at your domain registrar (supports Gandi and Porkbun):

```bash
# Set nameservers for a domain at Gandi
dnsx set ns freebsd.io adam.ns.cloudflare.com bella.ns.cloudflare.com

# Set nameservers at Porkbun
dnsx set ns --registrar porkbun freebsd.io adam.ns.cloudflare.com bella.ns.cloudflare.com

# With 1Password
op run --env-file=.env.1password -- dnsx set ns freebsd.io adam.ns.cloudflare.com bella.ns.cloudflare.com
```

This is useful when migrating to Cloudflare - first add the domain in Cloudflare's dashboard, get the assigned nameservers, then use this command to point your domain to them.

## Importing Domains to Cloudflare

The `import` command creates new zones in Cloudflare for domains currently hosted elsewhere:

```bash
# Import a single domain
dnsx import freebsd.io

# Import multiple domains at once
dnsx import freebsd.io lnkr.xyz example.com

# With 1Password
op run --env-file=.env.1password -- dnsx import freebsd.io lnkr.xyz
```

This will:
1. Create a new zone in Cloudflare for each domain
2. Enable `jump_start` to auto-scan existing DNS records
3. Return the Cloudflare nameservers you need to set at your registrar

Example output:
```
✓ freebsd.io created
  Zone ID: abc123def456
  Nameservers:
    - adam.ns.cloudflare.com
    - bella.ns.cloudflare.com

Next steps:
  1. Update nameservers at your registrar to the ones shown above
  2. Wait for DNS propagation (up to 24-48 hours)
  3. Run 'dnsx export cloudflare' to sync YAML files
```

## Configuration

All configuration is done via environment variables. Set the appropriate variables for the providers you want to use:

### Cloudflare

```bash
export CLOUDFLARE_API_TOKEN="your-api-token"
export CLOUDFLARE_ACCOUNT_ID="your-account-id"  # optional
```

You can also use Terraform variable names:
```bash
export TF_VAR_cloudflare_api_token="your-api-token"
export TF_VAR_cloudflare_account_id="your-account-id"
```

Get your API token at: https://dash.cloudflare.com/profile/api-tokens

### Gandi

```bash
export GANDI_API_KEY="your-api-key"
```

Get your API key at: https://account.gandi.net/en/users/USER/security

### GoDaddy

```bash
export GODADDY_API_KEY="your-api-key"
export GODADDY_API_SECRET="your-api-secret"
```

Get your API credentials at: https://developer.godaddy.com/keys

### Porkbun

```bash
export PORKBUN_API_KEY="your-api-key"
export PORKBUN_SECRET_KEY="your-secret-key"
```

Get your API credentials at: https://porkbun.com/account/api

## Using with 1Password (Recommended)

```bash
    brew install --cask 1password-cli
```

- Open 1Password
- Create items for each provider (e.g., "Cloudflare API", "Gandi API")
- Add your API keys/tokens as fields
- Save
- Right-click on any field in 1Password
- Select "Copy Secret Reference"
- It will look like: `op://Private/Cloudflare API/api_token`

Edit the example file:

```bash
    vim .env.1password
```

Run dnsx with 1Password:

```bash
    # The op CLI injects secrets as environment variables
    op run --env-file=.env.1password -- dnsx export all

    # Or for a specific provider
    op run --env-file=.env.1password -- dnsx export cloudflare
```

## Output Format

Records are exported to YAML files, one file per domain:

```yaml
domain: example.com
zone_id: abc123  # Cloudflare only
records:
  - name: "@"
    type: A
    value: 192.0.2.1
    ttl: 3600
  - name: www
    type: CNAME
    value: example.com
    ttl: 3600
  - name: "@"
    type: MX
    value: mail.example.com
    ttl: 3600
    priority: 10
metadata:
  provider: cloudflare
  exported_at: 2025-11-27T12:00:00Z
  account_id: your-account-id  # if available
```

## Development

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make release

# Format code
make fmt

# Run vet
make vet

# Clean build artifacts
make clean
```

### Project Structure

```
dnsx/
├── cmd/dnsx/           # CLI entry point
│   └── main.go
├── internal/
│   ├── exporter/       # Export from providers
│   │   ├── exporter.go
│   │   ├── cloudflare.go
│   │   ├── gandi.go
│   │   ├── godaddy.go
│   │   └── porkbun.go
│   ├── importer/       # Import to providers
│   │   └── cloudflare.go
│   ├── registrar/      # Registrar operations (set NS)
│   │   ├── registrar.go
│   │   ├── gandi.go
│   │   └── porkbun.go
│   ├── generator/      # Terraform generation
│   │   └── terraform.go
│   └── output/         # YAML output handling
│       └── yaml.go
├── go.mod
├── Makefile
└── README.md
```

### Adding a New Provider

To add a new DNS provider:

1. Create a new file in `internal/exporter/` (e.g., `route53.go`)
2. Implement the `Exporter` interface:
   ```go
   type Exporter interface {
       Name() string
       IsConfigured() bool
       Export(ctx context.Context) ([]DomainData, error)
   }
   ```
3. Add the exporter to the map in `cmd/dnsx/main.go`
4. Update the help text and README

## Examples

### Daily Backup Script

```bash
#!/bin/bash
# backup-dns.sh

BACKUP_DIR="/backups/dns/$(date +%Y-%m-%d)"

dnsx export all --outdir "$BACKUP_DIR"

# Commit to git
cd "$BACKUP_DIR"
git add .
git commit -m "DNS backup $(date +%Y-%m-%d)"
git push
```

### Compare DNS Records

```bash
# Export current state
dnsx export cloudflare --outdir current

# Compare with previous backup
diff -ur previous/ current/
```

### Migrate Domains to Cloudflare

```bash
# Option 1: One-command migration (recommended)
# Moves domain from Porkbun to Cloudflare, updates NS automatically
dnsx mv porkbun/freebsd.io cloudflare/

# Option 2: Manual step-by-step migration
# 1. Import domain to Cloudflare (creates zone, shows assigned nameservers)
dnsx import freebsd.io

# 2. Update nameservers at your registrar via API
dnsx set ns --registrar porkbun freebsd.io adam.ns.cloudflare.com bella.ns.cloudflare.com

# 3. Wait for propagation, then export from Cloudflare
dnsx export cloudflare --outdir yaml/

# 4. Generate Terraform and apply
dnsx gen terraform terraform/
```

## Security Notes

- Never commit API keys to version control
- Use environment variables or secret management tools
- The tool doesn't store or cache credentials
- API keys are only used for the duration of the export
- Output directory permissions are set to `0750` (owner read/write/execute only)

## License

MIT License - See LICENSE file for details

## Contributing

Contributions welcome! Please feel free to submit a Pull Request.

## Author

Created by Adam Koszek adam@koszek.com
