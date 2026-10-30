package compiler

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type testPolicy struct{}

func (testPolicy) Review(string) error { return nil }
func (testPolicy) ID() string          { return "test-custom-policy" }
func (testPolicy) Guarantees() core.PolicyGuarantees {
	return core.PolicyGuarantees{Deterministic: true}
}

type testABI struct{}

func (testABI) Generate(string) (*abi.ABI, error) { return &abi.ABI{Version: abi.SchemaVersion}, nil }

type testInjector struct{}

func (testInjector) InjectGas(s string) (string, error) { return s, nil }

type testProfiler struct{}

func (testProfiler) BuildProfile(string) (*core.GasProfile, error) { return &core.GasProfile{}, nil }

func TestCompileCancellationStopsBuildWithoutPublishing(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "started")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo \"go version " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "\"; exit 0; fi\nprintf started > '" + marker + "'\nexec /bin/sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := t.TempDir()
	c, err := New(Options{OutputDir: out, BuildTimeout: 15 * time.Second, RequireDeterminism: true}, Deps{Policy: testPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Compile(ctx, "package main\nfunc Add(a,b int)int{return a+b}\n"); done <- err }()
	// Startup under the race runtime is separate from cancellation latency.
	// Synchronize on the child signal; keep the 500ms cancellation assertion.
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("build exited before startup signal: %v", err)
		case <-deadline:
			t.Fatal("build process never started")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("build ignored caller cancellation")
	}
	files, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("canceled build published or retained files: %v", files)
	}
}

func TestGenerateEntryUsesContractAPIGas(t *testing.T) {
	compiler := &Compiler{}
	entry, err := compiler.generateEntryFile(&abi.ABI{Version: abi.SchemaVersion})
	if err != nil {
		t.Fatalf("generateEntryFile: %v", err)
	}
	for _, want := range []string{
		"__vmAPI.InitGas",
		"__vmAPI.ConsumeGas",
		"__vmAPI.GasUsed()",
		"__vmAPI.ResetGas()",
		"__vmAPI.GasEntryBase",
	} {
		if !strings.Contains(entry, want) {
			t.Fatalf("entry missing %q, got:\n%s", want, entry)
		}
	}
	if strings.Contains(entry, "__vmGasConsumed +=") {
		t.Fatal("entry should not maintain local __vmGasConsumed counter")
	}
	if strings.Contains(entry, "var __vmGasConsumed") || strings.Contains(entry, "var __vmGasLimit") {
		t.Fatal("entry should not declare local __vmGasConsumed/__vmGasLimit")
	}
}

func TestCustomDeterministicPolicyUsesHermeticBuild(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "environment")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo \"go version " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "\"; exit 0; fi\nprintf '%s' \"$CGO_ENABLED,$GOTOOLCHAIN,$GOWORK,$GOFLAGS,$GOEXPERIMENT\" > '" + marker + "'\nprintf artifact > \"$4\"\nchmod +x \"$4\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("GOTOOLCHAIN", "auto")
	t.Setenv("GOWORK", "host-workspace")
	t.Setenv("GOFLAGS", "host-flags")
	t.Setenv("GOEXPERIMENT", "host-experiments")
	c, err := New(Options{OutputDir: t.TempDir()}, Deps{Policy: testPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Compile(context.Background(), "package main\nfunc Add(a,b int)int{return a+b}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Guarantees.Deterministic || result.SourcePolicy != "test-custom-policy" {
		t.Fatalf("lost custom policy guarantees: %+v", result)
	}
	env, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(env) != "0,local,off,," {
		t.Fatalf("non-hermetic environment %q", env)
	}
}

func TestCompileRejectsActualToolchainMismatchBeforeBuild(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "built")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then printf 'go version go0.0.0 " + runtime.GOOS + "/" + runtime.GOARCH + "\\n'; exit 0; fi\nprintf built > '" + marker + "'\nprintf artifact > \"$4\"\nchmod +x \"$4\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	c, err := New(Options{OutputDir: t.TempDir()}, Deps{Policy: testPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Compile(context.Background(), "package main\nfunc Add(a,b int)int{return a+b}\n")
	if err == nil || !strings.Contains(err.Error(), "toolchain") {
		t.Fatalf("expected toolchain mismatch, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("mismatched toolchain was used to build")
	}
}

type developmentTestPolicy struct{}

func (developmentTestPolicy) Review(string) error { return nil }
func (developmentTestPolicy) ID() string          { return "test-development" }

func TestExplicitToolchainAndAmbientEnvironment(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "environment")
	version := "go9.8.7"
	tool := filepath.Join(bin, "pinned-go")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'go version " + version + " " + runtime.GOOS + "/" + runtime.GOARCH + "'; exit 0; fi\nprintf '%s' \"$GOENV,$GOOS,$GOARCH,$CGO_ENABLED,$GOTOOLCHAIN,$GOWORK,$GOFLAGS,$GOEXPERIMENT,$GOROOT,$GOARM64\" > '" + marker + "'\nprintf artifact > \"$4\"\nchmod +x \"$4\"\n"
	if err := os.WriteFile(tool, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GOFLAGS", "GOEXPERIMENT", "GOWORK", "GOROOT", "GOARM64", "GOENV", "GOOS", "GOARCH", "GOTOOLCHAIN"} {
		t.Setenv(key, "ambient-invalid")
	}
	c, err := New(Options{GoBinary: tool, GoVersion: version, OutputDir: t.TempDir()}, Deps{Policy: developmentTestPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}})
	if err != nil {
		t.Fatal(err)
	}
	source := "package main\nfunc Add(a,b int)int{return a+b}\n"
	first, err := c.Compile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	want := "off," + runtime.GOOS + "," + runtime.GOARCH + ",0,local,off,,,,"
	if string(got) != want {
		t.Fatalf("environment: got %q want %q", got, want)
	}
	// An existing cache entry must not bypass version validation.
	if err := os.WriteFile(tool, []byte(strings.ReplaceAll(script, version, "go0.0.0")), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Compile(context.Background(), source); err == nil || !strings.Contains(err.Error(), "toolchain mismatch") {
		t.Fatalf("cached executable bypassed toolchain validation: %v", err)
	}
	if _, err := os.Stat(first.ExecutablePath); err != nil {
		t.Fatal(err)
	}
}

func TestToolchainProbeCancellation(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "probe-started")
	tool := filepath.Join(bin, "go")
	script := "#!/bin/sh\nprintf started > '" + marker + "'\nexec /bin/sleep 30\n"
	if err := os.WriteFile(tool, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	c, err := New(Options{GoBinary: tool, OutputDir: t.TempDir()}, Deps{Policy: testPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Compile(ctx, "package main\nfunc Add()int{return 1}\n"); done <- err }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("probe exited early: %v", err)
		case <-deadline:
			t.Fatal("probe never started")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected canceled probe: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("toolchain probe ignored cancellation")
	}
}
