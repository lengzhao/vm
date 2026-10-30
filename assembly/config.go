// Package assembly delegates plugin graphs to PluginKit and manages the
// resulting instances. Import plugins/all or the desired individual plugins.
package assembly

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

const MaxConfigBytes = 1 << 20

type Config struct {
	Root    string         `yaml:"root" json:"root"`
	Plugins map[string]any `yaml:"plugins" json:"plugins"`
}

// Load accepts one bounded YAML document and rejects unknown envelope fields.
func Load(ctx context.Context, r io.Reader) (*Runtime, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxConfigBytes {
		return nil, fmt.Errorf("configuration exceeds size limit")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected a single configuration document")
	}
	return Build(ctx, cfg)
}
