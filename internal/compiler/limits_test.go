package compiler

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/abi"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type inspectingPolicy struct{ review func(string) error }

func (p inspectingPolicy) ID() string            { return "limits-test" }
func (p inspectingPolicy) Review(s string) error { return p.review(s) }

type inspectingABI struct{ called *bool }

func (a inspectingABI) Generate(string) (*abi.ABI, error) {
	*a.called = true
	return &abi.ABI{Version: abi.SchemaVersion}, nil
}

func TestCompileRejectsLargeSourceBeforePolicy(t *testing.T) {
	called := false
	c, err := New(Options{OutputDir: t.TempDir()}, Deps{
		Policy: inspectingPolicy{func(string) error { called = true; return errors.New("policy called") }},
		ABI:    testABI{}, Injector: testInjector{}, Profiler: testProfiler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Compile(context.Background(), strings.Repeat("x", (1<<20)+1))
	if err == nil || called || !strings.Contains(err.Error(), "source exceeds") {
		t.Fatalf("policy called=%v error=%v", called, err)
	}
}

func TestCompileBoundsToolDiagnostics(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'go version " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "'; exit 0; fi\ndd if=/dev/zero bs=65536 count=2 2>/dev/null | tr '\\000' x\nexit 1\n"
	if err := os.WriteFile(tool, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	c, err := New(Options{GoBinary: tool, OutputDir: t.TempDir()}, Deps{Policy: testPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Compile(context.Background(), "package main")
	if err == nil {
		t.Fatal("expected build failure")
	}
	if len(err.Error()) > 70000 || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected bounded, marked diagnostics; error length=%d", len(err.Error()))
	}
}

func TestCompileSourceLimitOptions(t *testing.T) {
	deps := Deps{Policy: testPolicy{}, ABI: testABI{}, Injector: testInjector{}, Profiler: testProfiler{}}
	if _, err := New(Options{MaxSourceBytes: -1}, deps); err == nil {
		t.Fatal("negative limit accepted")
	}
	called := false
	deps.Policy = inspectingPolicy{func(string) error { called = true; return errors.New("policy called") }}
	c, err := New(Options{MaxSourceBytes: 12}, deps)
	if err != nil {
		t.Fatal(err)
	}
	// The limit is inclusive. Exact-sized input reaches the policy.
	_, _ = c.Compile(context.Background(), "package main")
	if !called {
		t.Fatal("source at limit rejected")
	}
	called = false
	_, err = c.Compile(context.Background(), "package main ")
	if err == nil || called {
		t.Fatalf("oversized source reached policy: %v", err)
	}
}

func TestCompileBudgetIncludesFrontend(t *testing.T) {
	called := false
	c, err := New(Options{OutputDir: t.TempDir(), BuildTimeout: time.Millisecond}, Deps{
		Policy: inspectingPolicy{func(string) error { time.Sleep(10 * time.Millisecond); return nil }},
		ABI:    inspectingABI{&called}, Injector: testInjector{}, Profiler: testProfiler{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Compile(context.Background(), "package main")
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("frontend exceeded budget, ABI called=%v: %v", called, err)
	}
}
