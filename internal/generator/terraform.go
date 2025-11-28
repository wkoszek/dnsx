package generator

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wkoszek/dnsx/internal/exporter"
	"gopkg.in/yaml.v3"
)

// sanitizeResourceName converts a name to a valid Terraform resource name.
func sanitizeResourceName(name string) string {
	// Replace special characters with underscores
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, "@", "root")

	// Remove any other invalid characters
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	name = re.ReplaceAllString(name, "_")

	// Ensure it doesn't start with a number
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		name = "r_" + name
	}

	return strings.ToLower(name)
}

// escapeHCLString escapes special characters in HCL strings.
func escapeHCLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

// generateCloudflareZone generates Cloudflare zone resource HCL.
func generateCloudflareZone(domain string) string {
	resourceName := sanitizeResourceName(domain)
	return fmt.Sprintf(`resource "cloudflare_zone" "%s" {
  account_id = var.cloudflare_account_id
  zone       = "%s"
}
`, resourceName, domain)
}

// generateCloudflareRecord generates Cloudflare DNS record resource HCL.
func generateCloudflareRecord(domain string, record exporter.DNSRecord, zoneResourceName string) string {
	name := record.Name
	recordType := record.Type
	value := escapeHCLString(record.Value)
	ttl := record.TTL
	if ttl == 0 {
		ttl = 1 // 1 = automatic in Cloudflare
	}

	// Create unique resource name with hash suffix
	resourceName := fmt.Sprintf("%s_%s_%s", sanitizeResourceName(domain), sanitizeResourceName(name), strings.ToLower(recordType))
	valueHash := fmt.Sprintf("%x", md5.Sum([]byte(value)))[:8]
	resourceName = fmt.Sprintf("%s_%s", resourceName, valueHash)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`resource "cloudflare_record" "%s" {
  zone_id = cloudflare_zone.%s.id
  name    = "%s"
  type    = "%s"
  content = "%s"
  ttl     = %d
`, resourceName, zoneResourceName, name, recordType, value, ttl))

	// Add optional fields
	if record.Proxied != nil {
		sb.WriteString(fmt.Sprintf("  proxied = %t\n", *record.Proxied))
	}

	if record.Priority != nil {
		sb.WriteString(fmt.Sprintf("  priority = %d\n", *record.Priority))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// generateGoDaddyRecord generates GoDaddy DNS record resource HCL.
func generateGoDaddyRecord(domain string, record exporter.DNSRecord) string {
	name := record.Name
	recordType := record.Type
	value := escapeHCLString(record.Value)
	ttl := record.TTL
	if ttl == 0 {
		ttl = 3600
	}

	// Create unique resource name with hash suffix
	resourceName := fmt.Sprintf("%s_%s_%s", sanitizeResourceName(domain), sanitizeResourceName(name), strings.ToLower(recordType))
	valueHash := fmt.Sprintf("%x", md5.Sum([]byte(value)))[:8]
	resourceName = fmt.Sprintf("%s_%s", resourceName, valueHash)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`resource "godaddy_domain_record" "%s" {
  domain = "%s"
  name   = "%s"
  type   = "%s"
  data   = "%s"
  ttl    = %d
`, resourceName, domain, name, recordType, value, ttl))

	if record.Priority != nil {
		sb.WriteString(fmt.Sprintf("  priority = %d\n", *record.Priority))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// generateGandiRecord generates Gandi DNS record resource HCL.
func generateGandiRecord(domain string, record exporter.DNSRecord) string {
	name := record.Name
	recordType := record.Type
	value := escapeHCLString(record.Value)
	ttl := record.TTL
	if ttl == 0 {
		ttl = 3600
	}

	// Create unique resource name with hash suffix
	resourceName := fmt.Sprintf("%s_%s_%s", sanitizeResourceName(domain), sanitizeResourceName(name), strings.ToLower(recordType))
	valueHash := fmt.Sprintf("%x", md5.Sum([]byte(value)))[:8]
	resourceName = fmt.Sprintf("%s_%s", resourceName, valueHash)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`resource "gandi_livedns_record" "%s" {
  zone   = "%s"
  name   = "%s"
  type   = "%s"
  values = ["%s"]
  ttl    = %d
}
`, resourceName, domain, name, recordType, value, ttl))

	return sb.String()
}

// generatePorkbunRecord generates Porkbun DNS record resource HCL.
func generatePorkbunRecord(domain string, record exporter.DNSRecord) string {
	name := record.Name
	recordType := record.Type
	value := escapeHCLString(record.Value)
	ttl := record.TTL
	if ttl == 0 {
		ttl = 600
	}

	// Create unique resource name with hash suffix
	resourceName := fmt.Sprintf("%s_%s_%s", sanitizeResourceName(domain), sanitizeResourceName(name), strings.ToLower(recordType))
	valueHash := fmt.Sprintf("%x", md5.Sum([]byte(value)))[:8]
	resourceName = fmt.Sprintf("%s_%s", resourceName, valueHash)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`resource "porkbun_dns_record" "%s" {
  domain  = "%s"
  name    = "%s"
  type    = "%s"
  content = "%s"
  ttl     = "%d"
`, resourceName, domain, name, recordType, value, ttl))

	if record.Priority != nil {
		sb.WriteString(fmt.Sprintf("  prio = \"%d\"\n", *record.Priority))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// GenerateTerraformFromYAML generates Terraform HCL from a YAML file.
func GenerateTerraformFromYAML(yamlFile string) (string, error) {
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return "", fmt.Errorf("read yaml file: %w", err)
	}

	var domainData exporter.DomainData
	if err := yaml.Unmarshal(data, &domainData); err != nil {
		return "", fmt.Errorf("parse yaml: %w", err)
	}

	domain := domainData.Domain
	records := domainData.Records
	provider := "unknown"
	if p, ok := domainData.Metadata["provider"].(string); ok {
		provider = p
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Generated from %s\n", filepath.Base(yamlFile)))
	sb.WriteString(fmt.Sprintf("# Domain: %s\n", domain))
	sb.WriteString(fmt.Sprintf("# Provider: %s\n", provider))
	sb.WriteString(fmt.Sprintf("# Records: %d\n\n", len(records)))

	switch provider {
	case "cloudflare":
		zoneResourceName := sanitizeResourceName(domain)
		sb.WriteString(generateCloudflareZone(domain))
		sb.WriteString("\n")
		for _, record := range records {
			sb.WriteString(generateCloudflareRecord(domain, record, zoneResourceName))
			sb.WriteString("\n")
		}

	case "godaddy":
		for _, record := range records {
			sb.WriteString(generateGoDaddyRecord(domain, record))
			sb.WriteString("\n")
		}

	case "gandi":
		for _, record := range records {
			sb.WriteString(generateGandiRecord(domain, record))
			sb.WriteString("\n")
		}

	case "porkbun":
		for _, record := range records {
			sb.WriteString(generatePorkbunRecord(domain, record))
			sb.WriteString("\n")
		}

	default:
		sb.WriteString(fmt.Sprintf("# Unknown provider: %s\n", provider))
	}

	return sb.String(), nil
}

// GenerateTerraform reads YAML files from inputDir and generates .tf files in outputDir.
func GenerateTerraform(inputDir, outputDir string) error {
	// Check if input directory exists
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		return fmt.Errorf("input directory not found: %s", inputDir)
	}

	// Find all YAML files
	pattern := filepath.Join(inputDir, "*.yaml")
	yamlFiles, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob yaml files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return fmt.Errorf("no YAML files found in %s", inputDir)
	}

	fmt.Printf("Found %d YAML files\n", len(yamlFiles))

	// Create output directory
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Generate Terraform for each YAML file
	generated := 0
	for _, yamlFile := range yamlFiles {
		basename := filepath.Base(yamlFile)
		domain := strings.TrimSuffix(basename, ".yaml")

		fmt.Printf("generating terraform %s ", domain)

		hcl, err := GenerateTerraformFromYAML(yamlFile)
		if err != nil {
			fmt.Printf("error %s\n", err.Error())
			continue
		}

		tfFile := filepath.Join(outputDir, domain+".tf")
		if err := os.WriteFile(tfFile, []byte(hcl), 0640); err != nil {
			fmt.Printf("error %s\n", err.Error())
			continue
		}

		fmt.Printf("ok\n")
		generated++
	}

	fmt.Printf("\nGenerated %d/%d Terraform files in %s\n", generated, len(yamlFiles), outputDir)
	return nil
}
