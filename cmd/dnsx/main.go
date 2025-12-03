package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wkoszek/dnsx/internal/exporter"
	"github.com/wkoszek/dnsx/internal/generator"
	"github.com/wkoszek/dnsx/internal/importer"
	"github.com/wkoszek/dnsx/internal/output"
	"github.com/wkoszek/dnsx/internal/registrar"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printUsage()
		return fmt.Errorf("no command specified")
	}

	if os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		printUsage()
		return nil
	}

	switch os.Args[1] {
	case "export":
		return runExport()
	case "import":
		return runImport()
	case "ls":
		return runLs()
	case "mv":
		return runMv()
	case "set":
		return runSet()
	case "gen":
		return runGen()
	default:
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}

func runImport() error {
	importCmd := flag.NewFlagSet("import", flag.ExitOnError)
	jumpStart := importCmd.Bool("jump-start", true, "scan for existing DNS records")
	target := importCmd.String("to", "cloudflare", "target provider (currently only cloudflare)")

	if len(os.Args) < 3 {
		printUsage()
		return fmt.Errorf("import command requires at least one domain")
	}

	importCmd.Parse(os.Args[2:])
	domains := importCmd.Args()

	if len(domains) == 0 {
		printUsage()
		return fmt.Errorf("import command requires at least one domain")
	}

	if *target != "cloudflare" {
		return fmt.Errorf("only cloudflare is supported as import target")
	}

	imp := importer.NewCloudflareImporter()
	if !imp.IsConfigured() {
		return fmt.Errorf("cloudflare not configured (set CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID)")
	}

	ctx := context.Background()
	results, err := imp.ImportDomains(ctx, domains, *jumpStart)
	if err != nil {
		return err
	}

	fmt.Println()
	var successfulZones []importer.ZoneResult
	for _, r := range results {
		switch r.Status {
		case "created":
			fmt.Printf("✓ %s created\n", r.Domain)
			fmt.Printf("  Zone ID: %s\n", r.ZoneID)
			fmt.Printf("  Nameservers:\n")
			for _, ns := range r.Nameservers {
				fmt.Printf("    - %s\n", ns)
			}
			successfulZones = append(successfulZones, r)
		case "exists":
			fmt.Printf("● %s already exists\n", r.Domain)
			fmt.Printf("  Zone ID: %s\n", r.ZoneID)
			fmt.Printf("  Nameservers:\n")
			for _, ns := range r.Nameservers {
				fmt.Printf("    - %s\n", ns)
			}
			successfulZones = append(successfulZones, r)
		case "error":
			fmt.Printf("✗ %s error: %s\n", r.Domain, r.Error)
		}
		fmt.Println()
	}

	// Print YAML for domains.yaml
	if len(successfulZones) > 0 {
		fmt.Println("Add to domains.yaml:")
		fmt.Println("```yaml")
		fmt.Println("cloudflare:")
		for _, r := range successfulZones {
			fmt.Printf("  - domain: %s\n", r.Domain)
			fmt.Printf("    zone_id: %s\n", r.ZoneID)
		}
		fmt.Println("```")
		fmt.Println()
	}

	fmt.Println("Next steps:")
	fmt.Println("  1. Update nameservers at your registrar to the ones shown above")
	fmt.Println("  2. Wait for DNS propagation (up to 24-48 hours)")
	fmt.Println("  3. Run 'dnsx export cloudflare' to sync YAML files")

	return nil
}

// sanitizeDomainName converts a domain to a valid Terraform resource name
func sanitizeDomainName(domain string) string {
	name := strings.ReplaceAll(domain, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToLower(name)
}

func runLs() error {
	if len(os.Args) < 3 {
		printUsage()
		return fmt.Errorf("ls command requires a provider argument")
	}

	provider := os.Args[2]

	exporters := map[string]exporter.Exporter{
		"cloudflare": exporter.NewCloudflareExporter(),
		"gandi":      exporter.NewGandiExporter(),
		"godaddy":    exporter.NewGoDaddyExporter(),
		"porkbun":    exporter.NewPorkbunExporter(),
	}

	exp, ok := exporters[provider]
	if !ok {
		return fmt.Errorf("unknown provider: %s (available: cloudflare, gandi, godaddy, porkbun)", provider)
	}

	if !exp.IsConfigured() {
		return fmt.Errorf("%s not configured (missing environment variables)", provider)
	}

	ctx := context.Background()
	domains, err := exp.ListDomains(ctx)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	for _, domain := range domains {
		fmt.Println(domain)
	}

	return nil
}

func runMv() error {
	if len(os.Args) < 4 {
		printUsage()
		return fmt.Errorf("mv command requires: <source> <destination>")
	}

	source := os.Args[2]
	dest := os.Args[3]

	// Parse source: provider/domain
	sourceParts := strings.SplitN(source, "/", 2)
	if len(sourceParts) != 2 {
		return fmt.Errorf("invalid source format: expected provider/domain, got %s", source)
	}
	sourceProvider := sourceParts[0]
	domain := sourceParts[1]

	// Parse destination: provider/
	destProvider := strings.TrimSuffix(dest, "/")

	if destProvider != "cloudflare" {
		return fmt.Errorf("only cloudflare is supported as destination provider")
	}

	// Supported source registrars for NS updates
	supportedRegistrars := map[string]bool{
		"gandi":   true,
		"porkbun": true,
	}

	if !supportedRegistrars[sourceProvider] {
		return fmt.Errorf("source provider %s not supported for NS updates (available: gandi, porkbun)", sourceProvider)
	}

	// Step 1: Create zone in Cloudflare
	fmt.Printf("Creating zone %s in Cloudflare...\n", domain)

	imp := importer.NewCloudflareImporter()
	if !imp.IsConfigured() {
		return fmt.Errorf("cloudflare not configured (set CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID)")
	}

	ctx := context.Background()
	results, err := imp.ImportDomains(ctx, []string{domain}, true)
	if err != nil {
		return fmt.Errorf("create zone: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no results from zone creation")
	}

	result := results[0]
	if result.Status == "error" {
		return fmt.Errorf("zone creation failed: %s", result.Error)
	}

	fmt.Printf("✓ Zone created (ID: %s)\n", result.ZoneID)
	fmt.Printf("  Nameservers:\n")
	for _, ns := range result.Nameservers {
		fmt.Printf("    - %s\n", ns)
	}
	fmt.Println()

	// Step 2: Update nameservers at source registrar
	fmt.Printf("Updating nameservers at %s...\n", sourceProvider)

	var reg registrar.Registrar
	switch sourceProvider {
	case "gandi":
		reg = registrar.NewGandiRegistrar()
	case "porkbun":
		reg = registrar.NewPorkbunRegistrar()
	}

	if !reg.IsConfigured() {
		fmt.Printf("Warning: %s registrar not configured, skipping NS update\n", sourceProvider)
		fmt.Println()
		fmt.Println("To update nameservers manually, run:")
		fmt.Printf("  dnsx set ns --registrar %s %s %s\n", sourceProvider, domain, strings.Join(result.Nameservers, " "))
		return nil
	}

	nsResult, err := reg.SetNameservers(ctx, domain, result.Nameservers)
	if err != nil {
		fmt.Printf("Warning: failed to update nameservers: %v\n", err)
		fmt.Println()
		fmt.Println("To update nameservers manually, run:")
		fmt.Printf("  dnsx set ns --registrar %s %s %s\n", sourceProvider, domain, strings.Join(result.Nameservers, " "))
		return nil
	}

	if nsResult.Status == "error" {
		fmt.Printf("Warning: failed to update nameservers: %s\n", nsResult.Error)
		fmt.Println()
		fmt.Println("To update nameservers manually, run:")
		fmt.Printf("  dnsx set ns --registrar %s %s %s\n", sourceProvider, domain, strings.Join(result.Nameservers, " "))
		return nil
	}

	fmt.Printf("✓ Nameservers updated at %s\n", sourceProvider)
	fmt.Println()

	// Print YAML for reference
	fmt.Println("Add to your YAML file:")
	fmt.Println("```yaml")
	fmt.Printf("domain: %s\n", domain)
	fmt.Printf("zone_id: %s\n", result.ZoneID)
	fmt.Println("metadata:")
	fmt.Println("  provider: cloudflare")
	fmt.Println("```")
	fmt.Println()

	fmt.Println("Next steps:")
	fmt.Println("  1. Wait for DNS propagation (up to 24-48 hours)")
	fmt.Println("  2. Run 'dnsx export cloudflare' to sync YAML files")

	return nil
}

func runSet() error {
	if len(os.Args) < 3 {
		printUsage()
		return fmt.Errorf("set command requires a subcommand (ns)")
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "ns":
		return runSetNS()
	default:
		return fmt.Errorf("unknown set subcommand: %s (available: ns)", subcommand)
	}
}

func runSetNS() error {
	setCmd := flag.NewFlagSet("set ns", flag.ExitOnError)
	registrarName := setCmd.String("registrar", "gandi", "registrar to update (gandi)")

	if len(os.Args) < 4 {
		printUsage()
		return fmt.Errorf("set ns requires: <domain> <ns1> <ns2> [ns3...]")
	}

	setCmd.Parse(os.Args[3:])
	args := setCmd.Args()

	if len(args) < 3 {
		printUsage()
		return fmt.Errorf("set ns requires: <domain> <ns1> <ns2> [ns3...]")
	}

	domain := args[0]
	nameservers := args[1:]

	var reg registrar.Registrar

	switch *registrarName {
	case "gandi":
		reg = registrar.NewGandiRegistrar()
	case "porkbun":
		reg = registrar.NewPorkbunRegistrar()
	default:
		return fmt.Errorf("unknown registrar: %s (available: gandi, porkbun)", *registrarName)
	}

	if !reg.IsConfigured() {
		envVars := map[string]string{
			"gandi":   "GANDI_API_KEY",
			"porkbun": "PORKBUN_API_KEY, PORKBUN_SECRET_KEY",
		}
		return fmt.Errorf("%s not configured (set %s)", reg.Name(), envVars[reg.Name()])
	}

	ctx := context.Background()

	// Show current nameservers
	currentNS, err := reg.GetNameservers(ctx, domain)
	if err != nil {
		fmt.Printf("Warning: couldn't get current nameservers: %v\n", err)
	} else {
		fmt.Printf("Current nameservers for %s:\n", domain)
		for _, ns := range currentNS {
			fmt.Printf("  - %s\n", ns)
		}
		fmt.Println()
	}

	// Update nameservers
	fmt.Printf("Setting nameservers for %s to:\n", domain)
	for _, ns := range nameservers {
		fmt.Printf("  - %s\n", ns)
	}
	fmt.Println()

	result, err := reg.SetNameservers(ctx, domain, nameservers)
	if err != nil {
		return err
	}

	if result.Status == "error" {
		fmt.Printf("✗ Failed: %s\n", result.Error)
		return fmt.Errorf("failed to update nameservers")
	}

	fmt.Printf("✓ Nameservers updated successfully!\n")
	fmt.Println()
	fmt.Println("Note: DNS propagation may take up to 24-48 hours.")

	return nil
}

func runGen() error {
	if len(os.Args) < 3 {
		printUsage()
		return fmt.Errorf("gen command requires a subcommand (terraform, import)")
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "terraform":
		genCmd := flag.NewFlagSet("gen terraform", flag.ExitOnError)
		inputDir := genCmd.String("indir", "yaml", "input directory containing YAML files")
		genCmd.Parse(os.Args[3:])

		args := genCmd.Args()
		if len(args) == 0 {
			printUsage()
			return fmt.Errorf("gen terraform requires an output directory argument")
		}

		return generator.GenerateTerraform(*inputDir, args[0])

	case "import":
		genCmd := flag.NewFlagSet("gen import", flag.ExitOnError)
		inputDir := genCmd.String("indir", "yaml", "input directory containing YAML files")
		genCmd.Parse(os.Args[3:])

		return generator.GenerateImports(*inputDir)

	default:
		return fmt.Errorf("unknown gen subcommand: %s (available: terraform, import)", subcommand)
	}
}

func runExport() error {

	exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
	outdir := exportCmd.String("outdir", "yaml", "output directory for YAML files")

	if len(os.Args) < 3 {
		printUsage()
		return fmt.Errorf("export command requires a provider argument")
	}

	exportCmd.Parse(os.Args[2:])

	args := exportCmd.Args()
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("export command requires a provider argument")
	}

	provider := args[0]

	exporters := map[string]exporter.Exporter{
		"cloudflare": exporter.NewCloudflareExporter(),
		"gandi":      exporter.NewGandiExporter(),
		"godaddy":    exporter.NewGoDaddyExporter(),
		"porkbun":    exporter.NewPorkbunExporter(),
	}

	ctx := context.Background()

	if provider == "all" {
		return exportAll(ctx, exporters, *outdir)
	}

	exp, ok := exporters[provider]
	if !ok {
		return fmt.Errorf("unknown provider: %s (available: cloudflare, gandi, godaddy, porkbun, all)", provider)
	}

	_, err := exportProvider(ctx, exp, *outdir)
	return err
}

func exportAll(ctx context.Context, exporters map[string]exporter.Exporter, outdir string) error {
	providers := []string{"cloudflare", "gandi", "godaddy", "porkbun"}

	var executed int
	var totalDomains int

	for _, name := range providers {
		exp := exporters[name]
		if !exp.IsConfigured() {
			fmt.Printf("exporting %s provider skipped not-configured\n", name)
			continue
		}

		domains, err := exportProvider(ctx, exp, outdir)
		if err != nil {
			fmt.Printf("exporting %s provider error %s\n", name, err.Error())
			continue
		}

		executed++
		totalDomains += domains
	}

	if executed == 0 {
		return fmt.Errorf("no providers configured (set environment variables)")
	}

	return nil
}

func exportProvider(ctx context.Context, exp exporter.Exporter, outdir string) (int, error) {
	if !exp.IsConfigured() {
		return 0, fmt.Errorf("%s exporter not configured (missing environment variables)", exp.Name())
	}

	provider := exp.Name()

	results, err := exp.Export(ctx)
	if err != nil {
		return 0, err
	}

	exported := 0
	for _, result := range results {
		switch result.Status {
		case exporter.StatusOK:
			if result.Data == nil || len(result.Data.Records) == 0 {
				fmt.Printf("exporting %s domain %s records 0 ok\n", provider, result.Domain)
				continue
			}
			if err := output.WriteDomainData(outdir, *result.Data); err != nil {
				fmt.Printf("exporting %s domain %s error %s\n", provider, result.Domain, err.Error())
				continue
			}
			fmt.Printf("exporting %s domain %s records %d ok\n", provider, result.Domain, result.Records)
			exported++
		case exporter.StatusSkipped:
			fmt.Printf("exporting %s domain %s skipped %s\n", provider, result.Domain, result.Reason)
		case exporter.StatusError:
			fmt.Printf("exporting %s domain %s error %s\n", provider, result.Domain, result.Reason)
		}
	}

	return exported, nil
}

func printUsage() {
	fmt.Println("dnsx - DNS management utility")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  dnsx export <provider> [--outdir <path>]")
	fmt.Println("  dnsx import [--to cloudflare] <domain> [domain...]")
	fmt.Println("  dnsx ls <provider>")
	fmt.Println("  dnsx mv <provider>/<domain> <provider>/")
	fmt.Println("  dnsx set ns [--registrar gandi] <domain> <ns1> <ns2> [ns3...]")
	fmt.Println("  dnsx gen terraform [--indir <path>] <outdir>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  export        Export DNS records from providers to YAML")
	fmt.Println("  import        Import domains to a DNS provider (create zones)")
	fmt.Println("  ls            List domains in a provider")
	fmt.Println("  mv            Move a domain from one provider to another")
	fmt.Println("  set           Set domain configuration at registrar")
	fmt.Println("  gen           Generate infrastructure code from YAML")
	fmt.Println()
	fmt.Println("Export Providers:")
	fmt.Println("  cloudflare    Export from Cloudflare (requires: CLOUDFLARE_API_TOKEN)")
	fmt.Println("  gandi         Export from Gandi (requires: GANDI_API_KEY)")
	fmt.Println("  godaddy       Export from GoDaddy (requires: GODADDY_API_KEY, GODADDY_API_SECRET)")
	fmt.Println("  porkbun       Export from Porkbun (requires: PORKBUN_API_KEY, PORKBUN_SECRET_KEY)")
	fmt.Println("  all           Export from all configured providers")
	fmt.Println()
	fmt.Println("Import Targets:")
	fmt.Println("  cloudflare    Import to Cloudflare (requires: CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID)")
	fmt.Println()
	fmt.Println("Set Subcommands:")
	fmt.Println("  ns            Update nameservers at registrar (gandi, porkbun)")
	fmt.Println()
	fmt.Println("Gen Subcommands:")
	fmt.Println("  terraform     Generate Terraform/OpenTofu HCL from YAML files")
	fmt.Println("  import        Generate tofu import commands for Cloudflare zones")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --outdir <path>       Output directory for export (default: yaml)")
	fmt.Println("  --indir <path>        Input directory for gen (default: yaml)")
	fmt.Println("  --to <provider>       Target provider for import (default: cloudflare)")
	fmt.Println("  --registrar <name>    Registrar for set ns (default: gandi)")
	fmt.Println("  --jump-start=false    Disable auto-scanning existing DNS records")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  dnsx export cloudflare")
	fmt.Println("  dnsx export all --outdir /tmp/dns-backup")
	fmt.Println("  dnsx ls porkbun")
	fmt.Println("  dnsx mv porkbun/example.com cloudflare/")
	fmt.Println("  dnsx import freebsd.io lnkr.xyz")
	fmt.Println("  dnsx set ns --registrar porkbun freebsd.io adam.ns.cloudflare.com bella.ns.cloudflare.com")
	fmt.Println("  dnsx gen terraform outdir/")
	fmt.Println("  dnsx gen import")
}
