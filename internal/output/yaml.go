package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wkoszek/dnsx/internal/exporter"
	"gopkg.in/yaml.v3"
)

func WriteDomainData(outdir string, data exporter.DomainData) error {
	if err := os.MkdirAll(outdir, 0750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	safeDomain := filepath.Base(strings.ReplaceAll(data.Domain, "..", ""))
	filename := filepath.Join(outdir, safeDomain+".yaml")
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create file %s: %w", filename, err)
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}

	return nil
}
