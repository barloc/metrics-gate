package index

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Overlay is repo-local aliases + apply_column (warehouse column names).
type Overlay struct {
	Metrics []OverlayMetric `yaml:"metrics"`
}

// OverlayMetric maps a metric id to aliases and apply_column.
type OverlayMetric struct {
	ID          string   `yaml:"id"`
	ApplyColumn string   `yaml:"apply_column"`
	Aliases     []string `yaml:"aliases"`
}

// LoadOverlay reads overlay YAML from disk. Empty path → empty overlay.
func LoadOverlay(path string) (Overlay, error) {
	if path == "" {
		return Overlay{}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Overlay{}, fmt.Errorf("overlay: read %s: %w", path, err)
	}
	return ParseOverlay(body)
}

// ParseOverlay unmarshals overlay YAML bytes.
func ParseOverlay(body []byte) (Overlay, error) {
	var ov Overlay
	if err := yaml.Unmarshal(body, &ov); err != nil {
		return Overlay{}, fmt.Errorf("overlay: parse: %w", err)
	}
	return ov, nil
}
