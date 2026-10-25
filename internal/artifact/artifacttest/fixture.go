// Package artifacttest creates explicitly trusted producers' fixtures for tests.
// Production consumers never seal or repair artifacts on their callers' behalf.
package artifacttest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
)

func Seal(t testing.TB, c *core.CompiledContract) *core.CompiledContract {
	t.Helper()
	if c.ExecutablePath == "" {
		c.ExecutablePath = filepath.Join(t.TempDir(), "artifact")
		if err := os.WriteFile(c.ExecutablePath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if c.ABI == nil {
		c.ABI = &abi.ABI{Version: abi.SchemaVersion}
	}
	if c.SourcePolicy == "" {
		c.SourcePolicy = "go-development-v1"
	}
	digest := artifact.Digest([]byte("test fixture"))
	build := core.ArtifactBuild{CompilerVersion: "test-producer-v1", Toolchain: core.ArtifactToolchain{Version: runtime.Version(), BinarySHA256: digest, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}, RuntimeVersion: core.ArtifactRuntimeVersion, GasVersion: core.ArtifactGasVersion, RuntimeSHA256: artifact.RuntimeDigest(), SourceSHA256: digest, InjectedSHA256: digest, EntrySHA256: digest}
	if c.Manifest != nil {
		build = c.Manifest.Build
	}
	if err := artifact.Seal(context.Background(), c, build); err != nil {
		t.Fatal(err)
	}
	return c
}
