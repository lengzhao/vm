package compiler

import (
	"context"
	"github.com/lengzhao/vm/plugins/abi"
	"github.com/lengzhao/vm/plugins/gas"
	"github.com/lengzhao/vm/plugins/policy"
	"github.com/lengzhao/vm/plugins/security"
	"testing"
)

func TestWASICompilerProducesDistinctTarget(t *testing.T) {
	reviewer, _ := security.New(security.Config{})
	sourcePolicy, _ := policy.New(policy.Config{EnableSecurity: true, Deterministic: true}, policy.Deps{Reviewer: reviewer})
	generator, _ := abi.New(abi.Config{})
	injector, _ := gas.NewInjector(gas.Config{})
	profiler, _ := gas.NewProfiler(gas.Config{})
	deps := Deps{Policy: sourcePolicy, ABI: generator, Injector: injector, Profiler: profiler}
	cfg := Config{OutputDir: t.TempDir(), RequireDeterminism: true}
	wasi, err := NewWASI(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	source := "package main\nfunc Add(a,b int64)int64{return a+b}"
	c, err := wasi.Compile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if c.Manifest.Build.Toolchain.GOOS != "wasip1" || c.Manifest.Build.Toolchain.GOARCH != "wasm" {
		t.Fatalf("wrong target: %+v", c.Manifest.Build.Toolchain)
	}
	native, err := New(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	n, err := native.Compile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if c.SourceHash == n.SourceHash || c.ExecutablePath == n.ExecutablePath {
		t.Fatal("WASI hit native cache")
	}
}
