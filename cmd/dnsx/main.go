package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/wkoszek/dnsx/internal/exporter"
	"github.com/wkoszek/dnsx/internal/output"
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

	if os.Args[1] != "export" {
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}

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
	fmt.Println("dnsx - DNS export utility")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  dnsx export <provider> [--outdir <path>]")
	fmt.Println()
	fmt.Println("Providers:")
	fmt.Println("  cloudflare    Export from Cloudflare (requires: CLOUDFLARE_API_TOKEN)")
	fmt.Println("  gandi         Export from Gandi (requires: GANDI_API_KEY)")
	fmt.Println("  godaddy       Export from GoDaddy (requires: GODADDY_API_KEY, GODADDY_API_SECRET)")
	fmt.Println("  porkbun       Export from Porkbun (requires: PORKBUN_API_KEY, PORKBUN_SECRET_KEY)")
	fmt.Println("  all           Export from all configured providers")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --outdir <path>   Output directory (default: yaml)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  dnsx export cloudflare")
	fmt.Println("  dnsx export all --outdir /tmp/dns-backup")
}
