package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContractRuntimeSymbolsRejected(t *testing.T) {
	cases := []string{
		`import "github.com/lengzhao/vm/contractapi"; func F(){contractapi.ResetGas()}`,
		`import api "github.com/lengzhao/vm/contractapi"; func F(){f:=api.InitGas; f(0)}`,
		`import . "github.com/lengzhao/vm/contractapi"; func F(){ResetGas()}`,
		`func __vmConsumeGas(n uint64){}; func F(){}`,
		`func init(){}; func F(){}`,
		`func F(){defer func(){recover()}()}`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			if err := NewSecurityReviewer().Review("package main;" + src); err == nil {
				t.Fatal("runtime bypass accepted")
			}
		})
	}
	if err := NewSecurityReviewer().Review(`package main; import api "github.com/lengzhao/vm/contractapi"; func F() uint64{return api.BlockHeight()}`); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerPreservesLargeNumbers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "contract")
	payload := `{"version":3,"ok":true,"result":{"n":18446744073709551615},"gas":11,"events":[{"Name":"E","Fields":{"n":9007199254740993}}]}`
	if err := os.WriteFile(p, []byte("#!/bin/sh\nprintf '%s' '"+payload+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	r, err := NewProcessRunner(5*time.Second).Run(context.Background(), sealedTestArtifact(t, &CompiledContract{ProtocolVersion: ProtocolVersion, ExecutablePath: p}), CallRequest{Function: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Data) != `{"n":18446744073709551615}` {
		t.Fatalf("precision lost: %s", r.Data)
	}
	b, _ := json.Marshal(r.Events)
	if !strings.Contains(string(b), "9007199254740993") {
		t.Fatalf("event precision lost: %s", b)
	}
}

func TestGasMeterRejectsOverflow(t *testing.T) {
	g := NewGasMetering()
	g.SetGasLimit(math.MaxUint64)
	if err := g.ConsumeGas(math.MaxUint64 - 1); err != nil {
		t.Fatal(err)
	}
	if err := g.ConsumeGas(2); err == nil {
		t.Fatal("overflow accepted")
	}
	if g.GetConsumedGas() != math.MaxUint64-1 {
		t.Fatal("failed charge changed counter")
	}
}

type runnerFunc func(context.Context, *CompiledContract, CallRequest) (*CallResult, error)

func (f runnerFunc) Run(c context.Context, k *CompiledContract, r CallRequest) (*CallResult, error) {
	return f(c, k, r)
}

func TestConcurrentCallsHaveIndependentGas(t *testing.T) {
	e := NewVMEngine(VMConfig{EnableGasMetering: true, MaxGasLimit: 20, ContractStorageDir: t.TempDir()})
	c := compileTestContract(t, t.TempDir(), `package main;func F() int{return 1}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	e = NewVMEngineWithRunner(e.GetConfig(), runnerFunc(func(context.Context, *CompiledContract, CallRequest) (*CallResult, error) {
		arrived <- struct{}{}
		<-release
		return &CallResult{Data: []byte("1"), GasConsumed: 11}, nil
	}))
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := e.ExecuteWithContext(a, "F", nil); errs <- err }()
	}
	<-arrived
	<-arrived
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("independent call failed: %v", err)
		}
	}
}

func TestCallContextIsNotModified(t *testing.T) {
	e := NewVMEngine(VMConfig{EnableGasMetering: true, MaxGasLimit: 20, ContractStorageDir: t.TempDir()})
	c := compileTestContract(t, t.TempDir(), `package main;func F() int{return 1}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	cc := &CallContext{}
	if _, err = e.ExecuteWithContext(a, "F", cc); err != nil {
		t.Fatal(err)
	}
	if cc.ContractAddress != nil || cc.GasLimit != nil {
		t.Fatal("caller context was mutated")
	}
}

func TestRunnerRejectsMultipleResponses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "contract")
	os.WriteFile(p, []byte("#!/bin/sh\necho '{\"version\":3,\"ok\":true,\"result\":1} {\"ok\":true,\"result\":2}'\n"), 0755)
	_, err := NewProcessRunner(5*time.Second).Run(context.Background(), sealedTestArtifact(t, &CompiledContract{ProtocolVersion: ProtocolVersion, ExecutablePath: p}), CallRequest{Function: "F"})
	if err == nil {
		t.Fatal("multiple responses accepted")
	}
}

func TestRunnerBoundsOutput(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "contract")
			redirect := ""
			if stream == "stderr" {
				redirect = " >&2"
			}
			os.WriteFile(p, []byte("#!/bin/sh\nwhile :; do printf '"+strings.Repeat("x", 4096)+"'"+redirect+"; done\n"), 0755)
			r, err := NewProcessRunner(3*time.Second).Run(context.Background(), sealedTestArtifact(t, &CompiledContract{ProtocolVersion: ProtocolVersion, ExecutablePath: p}), CallRequest{Function: "F"})
			if err == nil || !strings.Contains(err.Error(), "output limit") {
				t.Fatalf("expected output limit, got %.200v", err)
			}
			if r != nil && (len(r.Stdout) > 1<<20 || len(r.Stderr) > 1<<20) {
				t.Fatal("unbounded buffer")
			}
		})
	}
}

func TestCompilerReturnsAbsolutePathForRelativeStorage(t *testing.T) {
	relative, err := filepath.Rel(mustWorkingDir(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := compileTestContract(t, relative, `package main;func F() int{return 1}`)
	if !filepath.IsAbs(c.ExecutablePath) {
		t.Fatalf("relative executable path: %s", c.ExecutablePath)
	}
	r, err := NewProcessRunner(5*time.Second).Run(context.Background(), c, CallRequest{Function: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Data) != "1" {
		t.Fatal(string(r.Data))
	}
}
func mustWorkingDir(t *testing.T) string {
	t.Helper()
	p, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConcurrentCompilationPublishesCompleteArtifact(t *testing.T) {
	cacheDir := t.TempDir()
	c := NewContractCompilerWithOptions(cacheDir, true)
	start := make(chan struct{})
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			<-start
			k, err := c.Compile(`package main;func F() int{return 123}`)
			if err == nil {
				if k.Manifest == nil || k.ManifestSHA256 == "" {
					done <- fmt.Errorf("incomplete published manifest")
					return
				}
				_, err = NewProcessRunner(5*time.Second).Run(context.Background(), k, CallRequest{Function: "F"})
			}
			done <- err
		}()
	}
	close(start)
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() || !strings.HasPrefix(entries[0].Name(), "contract_") {
		t.Fatalf("incomplete or duplicate cache publication: %v", entries)
	}
}

func TestCompilerBuildTimeout(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "go")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	c := compilerWithTimeout(t.TempDir(), 50*time.Millisecond)
	begin := time.Now()
	_, err := c.Compile(`package main;func F() int{return 1}`)
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected build deadline, got %v", err)
	}
	if time.Since(begin) > 2*time.Second {
		t.Fatal("build timeout not enforced")
	}
}

func TestEngineRunnerInjection(t *testing.T) {
	called := false
	runner := runnerFunc(func(context.Context, *CompiledContract, CallRequest) (*CallResult, error) {
		called = true
		return &CallResult{Data: []byte("99"), GasConsumed: 11}, nil
	})
	e := NewVMEngineWithRunner(VMConfig{ContractStorageDir: t.TempDir()}, runner)
	c := compileTestContract(t, t.TempDir(), `package main;func F() int{return 1}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Execute(a, "F")
	if err != nil {
		t.Fatal(err)
	}
	if !called || string(r) != "99" {
		t.Fatal("injected runner was not used")
	}
}

func TestGasChargesFunctionLiterals(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;func F() {f:=func(){};for i:=0;i<3;i++{f()}}`)
	r, err := NewProcessRunner(5*time.Second).Run(context.Background(), c, CallRequest{Function: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if r.GasConsumed != 17 {
		t.Fatalf("want 17 gas including function literal calls, got %d", r.GasConsumed)
	}
}
func TestRuntimeBoundaryCannotBeDisabled(t *testing.T) {
	c := NewContractCompilerWithOptions(t.TempDir(), false)
	if err := c.Validate(`package main;import api "github.com/lengzhao/vm/contractapi";func F(){api.ResetGas()}`); err == nil {
		t.Fatal("runtime boundary disabled")
	}
}
func TestEntryGasFailureReturnsStructuredResponse(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	r, err := NewProcessRunner(5*time.Second).Run(WithGasLimit(context.Background(), 1), c, CallRequest{Function: "F"})
	if err == nil {
		t.Fatal("entry gas should fail")
	}
	var response map[string]any
	if r == nil || json.Unmarshal([]byte(r.Stdout), &response) != nil {
		t.Fatal("entry gas panic escaped response handler")
	}
	if response["ok"] != false {
		t.Fatal("expected structured failure")
	}
}

func TestCallContextPointersAreIsolated(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	limit := uint64(20)
	sender := Address("alice")
	cc := &CallContext{GasLimit: &limit, Sender: &sender}
	e = NewVMEngineWithRunner(e.GetConfig(), runnerFunc(func(ctx context.Context, _ *CompiledContract, _ CallRequest) (*CallResult, error) {
		snapshot, _ := CallContextFrom(ctx)
		*snapshot.GasLimit = 0
		*snapshot.Sender = "bob"
		return &CallResult{}, nil
	}))
	if _, err := e.ExecuteWithContext(a, "F", cc); err != nil {
		t.Fatal(err)
	}
	if limit != 20 || sender != "alice" {
		t.Fatal("runner mutated caller-owned pointer values")
	}
}

func TestRunnerDeadlineBoundsInheritedPipes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "contract")
	os.WriteFile(p, []byte("#!/bin/sh\n/bin/sleep 5 &\nwait\n"), 0755)
	begin := time.Now()
	_, err := NewProcessRunner(50*time.Millisecond).Run(context.Background(), sealedTestArtifact(t, &CompiledContract{ProtocolVersion: ProtocolVersion, ExecutablePath: p}), CallRequest{Function: "F"})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(begin) > 2*time.Second {
		t.Fatal("inherited pipe defeated timeout")
	}
}

func TestRuntimePolicyAllowsLocalAliasShadow(t *testing.T) {
	code := `package main;import api "github.com/lengzhao/vm/contractapi";func F() int {api.BlockHeight();api:=struct{Value int}{1};return api.Value}`
	if err := NewSecurityReviewer().Review(code); err != nil {
		t.Fatalf("local shadow rejected: %v", err)
	}
}

func TestCallGasLimitCannotRaiseEngineCeiling(t *testing.T) {
	e := NewVMEngine(VMConfig{EnableGasMetering: true, MaxGasLimit: 20, ContractStorageDir: t.TempDir()})
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	e = NewVMEngineWithRunner(e.GetConfig(), runnerFunc(func(ctx context.Context, _ *CompiledContract, _ CallRequest) (*CallResult, error) {
		cc, _ := CallContextFrom(ctx)
		if cc.GasLimit == nil || *cc.GasLimit != 20 {
			t.Errorf("engine ceiling not enforced: %+v", cc.GasLimit)
		}
		return &CallResult{GasConsumed: 11}, nil
	}))
	for _, limit := range []uint64{0, 100} {
		if _, err := e.ExecuteWithContext(a, "F", &CallContext{GasLimit: &limit}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCallGasLimitUsedByHostValidation(t *testing.T) {
	e := NewVMEngine(VMConfig{EnableGasMetering: true, MaxGasLimit: 20, ContractStorageDir: t.TempDir()})
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	e = NewVMEngineWithRunner(e.GetConfig(), runnerFunc(func(context.Context, *CompiledContract, CallRequest) (*CallResult, error) {
		return &CallResult{GasConsumed: 11}, nil
	}))
	if _, err := e.ExecuteWithContext(a, "F", &CallContext{GasLimit: Uint64Ptr(5)}); err == nil {
		t.Fatal("call-specific ceiling ignored by host")
	}
}
