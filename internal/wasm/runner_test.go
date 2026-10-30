package wasm

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWazeroRejectsUnsafeConfiguration(t *testing.T) {
	for _, c := range []Options{{MemoryMB: -1}, {MemoryMB: 4097}, {MaxModuleMB: -1}, {Timeout: -time.Second}} {
		if _, err := New(c); err == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}
func TestWazeroRejectsNativeArtifacts(t *testing.T) {
	r, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	if _, err := r.Run(context.Background(), c, core.CallRequest{Function: "F"}); err == nil {
		t.Fatal("native artifact accepted")
	}
}
func TestWazeroTerminatesUnmeteredLoop(t *testing.T) {
	// Module with _start containing an unconditional loop (no source Gas hooks).
	data := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 4, 1, 96, 0, 0, 3, 2, 1, 0, 7, 10, 1, 6, 95, 115, 116, 97, 114, 116, 0, 0, 10, 9, 1, 7, 0, 3, 64, 12, 0, 11, 11}
	path := filepath.Join(t.TempDir(), "loop.wasm")
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatal(err)
	}
	c := artifacttest.Seal(t, &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion})
	b := c.Manifest.Build
	b.Toolchain.GOOS = "wasip1"
	b.Toolchain.GOARCH = "wasm"
	if err := artifact.Seal(context.Background(), c, b); err != nil {
		t.Fatal(err)
	}
	r, _ := New(Options{Timeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	started := time.Now()
	_, err := r.Run(context.Background(), c, core.CallRequest{Function: "F"})
	var e *core.ExecutionError
	if !errors.As(err, &e) || e.Code != core.CodeTimeout || time.Since(started) > 3*time.Second {
		t.Fatalf("unmetered loop: %v", err)
	}
}

func TestWazeroRejectsOversizedAndMalformedModules(t *testing.T) {
	for _, mode := range []string{"oversize", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("not wasm")
			if mode == "oversize" {
				data = make([]byte, 2<<20)
			}
			path := filepath.Join(t.TempDir(), "artifact")
			if err := os.WriteFile(path, data, 0755); err != nil {
				t.Fatal(err)
			}
			c := artifacttest.Seal(t, &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion})
			b := c.Manifest.Build
			b.Toolchain.GOOS = "wasip1"
			b.Toolchain.GOARCH = "wasm"
			if err := artifact.Seal(context.Background(), c, b); err != nil {
				t.Fatal(err)
			}
			r, _ := New(Options{MaxModuleMB: 1})
			t.Cleanup(func() { _ = r.Stop(context.Background()) })
			_, err := r.Run(context.Background(), c, core.CallRequest{Function: "F"})
			var e *core.ExecutionError
			if !errors.As(err, &e) || e.Code != core.CodeProcessFailure {
				t.Fatalf("invalid module: %v", err)
			}
		})
	}
}
func TestWazeroPreCanceledDoesNotReadArtifact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ := New(Options{})
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	_, err := r.Run(ctx, nil, core.CallRequest{Function: "F"})
	var e *core.ExecutionError
	if !errors.As(err, &e) || e.Code != core.CodeCanceled {
		t.Fatalf("pre-canceled: %v", err)
	}
}

func TestWazeroArtifactValidationKeepsCancellationCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := validateArtifact(ctx, nil)
	var e *core.ExecutionError
	if !errors.As(err, &e) || e.Code != core.CodeCanceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("validation cancellation: %v", err)
	}
}

func TestWazeroRejectsUnprovidedHostImports(t *testing.T) {
	data := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 4, 1, 96, 0, 0, 2, 12, 1, 3, 101, 110, 118, 4, 101, 118, 105, 108, 0, 0}
	path := filepath.Join(t.TempDir(), "import.wasm")
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatal(err)
	}
	c := artifacttest.Seal(t, &core.CompiledContract{ExecutablePath: path, ProtocolVersion: core.ProtocolVersion})
	b := c.Manifest.Build
	b.Toolchain.GOOS = "wasip1"
	b.Toolchain.GOARCH = "wasm"
	if err := artifact.Seal(context.Background(), c, b); err != nil {
		t.Fatal(err)
	}
	r, _ := New(Options{})
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	_, err := r.Run(context.Background(), c, core.CallRequest{Function: "F"})
	if err == nil || !strings.Contains(err.Error(), "env") {
		t.Fatalf("unknown host import: %v", err)
	}
}
