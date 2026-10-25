package artifact_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
)

func expectCode(t *testing.T, err error, want core.ErrorCode) {
	t.Helper()
	var ee *core.ExecutionError
	if !errors.As(err, &ee) || ee.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}
func TestManifestBindsMetadataAndBuildRecord(t *testing.T) {
	changes := map[string]func(*core.CompiledContract){
		"binary": func(c *core.CompiledContract) {
			if err := os.WriteFile(c.ExecutablePath, []byte("changed"), 0755); err != nil {
				t.Fatal(err)
			}
		},
		"ABI":            func(c *core.CompiledContract) { c.ABI.PackageName = "changed" },
		"policy":         func(c *core.CompiledContract) { c.SourcePolicy = "changed" },
		"guarantee":      func(c *core.CompiledContract) { c.Guarantees.Deterministic = true },
		"build key":      func(c *core.CompiledContract) { c.SourceHash = "changed" },
		"source digest":  func(c *core.CompiledContract) { c.Manifest.Build.SourceSHA256 = artifact.Digest([]byte("changed")) },
		"toolchain":      func(c *core.CompiledContract) { c.Manifest.Build.Toolchain.Version = "changed" },
		"root digest":    func(c *core.CompiledContract) { c.ManifestSHA256 = artifact.Digest([]byte("changed")) },
		"invalid digest": func(c *core.CompiledContract) { c.Manifest.BinarySHA256 = "bad" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
			change(c)
			expectCode(t, artifact.Validate(context.Background(), c), core.CodeArtifactIntegrity)
		})
	}
}
func TestManifestVersionRequired(t *testing.T) {
	for _, version := range []int{0, 2} {
		c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
		c.Manifest.Version = version
		expectCode(t, artifact.Validate(context.Background(), c), core.CodeUnsupportedVersion)
	}
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	c.Manifest = nil
	expectCode(t, artifact.Validate(context.Background(), c), core.CodeUnsupportedVersion)
}
func TestDigestRejectsSpecialFilesAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := artifact.FileDigest(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	if err := artifact.Validate(ctx, c); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(c.ExecutablePath, link); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.FileDigest(context.Background(), link); err == nil {
		t.Fatal("symlink executable accepted")
	}
	if _, err := artifact.FileDigest(context.Background(), t.TempDir()); err == nil {
		t.Fatal("directory executable accepted")
	}
	if err := os.Chmod(c.ExecutablePath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.FileDigest(context.Background(), c.ExecutablePath); err == nil {
		t.Fatal("non-executable accepted")
	}
}
