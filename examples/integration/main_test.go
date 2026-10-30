package main

import (
	"context"
	"github.com/lengzhao/vm/assembly"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedRecoverableIntegration(t *testing.T) {
	data, e := os.ReadFile("../../presets/objects.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var cfg assembly.Config
	if e = yaml.Unmarshal(data, &cfg); e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	cfg.Plugins["state"].(map[string]any)["config"].(map[string]any)["path"] = filepath.Join(dir, "state.db")
	cfg.Plugins["compiler"].(map[string]any)["config"].(map[string]any)["outputDir"] = filepath.Join(dir, "contracts")
	cfg.Plugins["contracts"].(map[string]any)["config"].(map[string]any)["directory"] = filepath.Join(dir, "contracts")
	if e = run(context.Background(), cfg); e != nil {
		t.Fatal(e)
	}
}

func TestDockerOwnedRecoverableIntegration(t *testing.T) {
	image := os.Getenv("VM_DOCKER_IMAGE")
	if image == "" {
		t.Skip("set VM_DOCKER_IMAGE for real Docker integration")
	}
	version := os.Getenv("VM_DOCKER_GO_VERSION")
	if version == "" {
		t.Fatal("VM_DOCKER_GO_VERSION required")
	}
	data, e := os.ReadFile("../../presets/objects-docker.yaml")
	if e != nil {
		t.Fatal(e)
	}
	data = []byte(strings.ReplaceAll(strings.ReplaceAll(string(data), "IMAGE_ID", image), "GO_VERSION", version))
	var cfg assembly.Config
	if e = yaml.Unmarshal(data, &cfg); e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	cfg.Plugins["state"].(map[string]any)["config"].(map[string]any)["path"] = filepath.Join(dir, "state.db")
	cfg.Plugins["compiler"].(map[string]any)["config"].(map[string]any)["outputDir"] = filepath.Join(dir, "contracts")
	cfg.Plugins["contracts"].(map[string]any)["config"].(map[string]any)["directory"] = filepath.Join(dir, "contracts")
	if e = run(context.Background(), cfg); e != nil {
		t.Fatal(e)
	}
}
