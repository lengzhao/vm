package compiler

import (
	"strings"
	"testing"
)

func TestDockerCompilerRejectsMutableImage(t *testing.T) {
	_, err := New(Config{Sandbox: &SandboxConfig{Image: "golang:latest"}}, Deps{})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("mutable image not rejected: %v", err)
	}
}
