package vm

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDeterministicPolicyRejectsAmbientCapabilities(t *testing.T) {
	cases := []string{
		`import "time";func F() int64{return time.Now().Unix()}`,
		`import clock "time";func F() int64{return clock.Now().Unix()}`,
		`import "math";func F() float64{return math.Sqrt(2)}`,
		`import "fmt";func F() string{return fmt.Sprintf("%p",new(int))}`,
		`func F() float64{return 1.5}`,
		`func F() complex128{return 1i}`,
		`func F() uintptr{return 0}`,
		`func F() *int{return new(int)}`,
		`func F() bool{x:=1;return &x==&x}`,
		`func F(){println("nondeterministic output")}`,
		`func F(){panic("error")}`,
		`import "strconv";func F() (float64,error){return strconv.ParseFloat("1.5",64)}`,
		`import conv "strconv";func F() int{return conv.IntSize}`,
		`import . "strconv";func F() string{return Itoa(1)}`,
		`import _ "errors";func F(){}`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			if err := ReviewDeterministic("package main;" + src); err == nil {
				t.Fatal("ambient capability accepted")
			}
		})
	}
}

func TestDeterministicPolicyAllowsValueComputations(t *testing.T) {
	cases := []string{
		`func Add(a,b int64) int64{return a+b}`,
		`import conv "strconv";func F(v int64) string{return conv.FormatInt(v,10)}`,
		`import "errors";func F() error{return errors.New("fixed")}`,
		`import api "github.com/lengzhao/vm/contractapi";func F() uint64{return api.BlockTime()}`,
		`import conv "strconv";func F(v int64) int{conv.FormatInt(v,10);conv:=struct{IntSize int}{7};return conv.IntSize}`,
		`func F(v []int64) int64{n:=int64(0);for _,x:=range v{n+=x};return n}`,
	}
	for _, src := range cases {
		if err := ReviewDeterministic("package main;" + src); err != nil {
			t.Errorf("valid computation rejected: %v", err)
		}
	}
}

func TestDeterministicModeCannotDisableReview(t *testing.T) {
	e := NewVMEngine(VMConfig{RequireDeterminism: true, EnableSecurityChecks: false, ContractStorageDir: t.TempDir()})
	code := `package main;import "time";func F() int64{return time.Now().Unix()}`
	if _, err := e.Compile(code); err == nil {
		t.Fatal("compiler bypassed deterministic review")
	}
	if _, err := e.GenerateABI(code); err == nil {
		t.Fatal("GenerateABI bypassed deterministic review")
	}
}

func TestDeterministicReplay(t *testing.T) {
	e := NewVMEngine(VMConfig{RequireDeterminism: true, EnableGasMetering: true, MaxGasLimit: 100, ContractStorageDir: t.TempDir()})
	c, err := e.Compile(`package main;import api "github.com/lengzhao/vm/contractapi";func F(n uint64) uint64{api.Log("E","height",api.BlockHeight(),"time",api.BlockTime());return api.BlockTime()+n}`)
	if err != nil {
		t.Fatal(err)
	}
	address, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	cc := &CallContext{BlockHeight: Uint64Ptr(9), BlockTime: Uint64Ptr(1234), Sender: AddressPtr("alice")}
	first, err := e.ExecuteWithContext(address, "F", cc, uint64(7))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		next, err := e.ExecuteWithContext(address, "F", cc, uint64(7))
		if err != nil {
			t.Fatal(err)
		}
		firstEvents, _ := json.Marshal(first.Events)
		nextEvents, _ := json.Marshal(next.Events)
		if !bytes.Equal(first.Data, next.Data) || !bytes.Equal(firstEvents, nextEvents) || first.GasConsumed != next.GasConsumed {
			t.Fatal("replay changed result, events or gas")
		}
		if len(next.Events) != 1 || !strings.Contains(string(next.Data), "1241") {
			t.Fatalf("bad replay: %+v", next)
		}
	}
}

func TestDeterministicModeRejectsDevelopmentArtifacts(t *testing.T) {
	dir := t.TempDir()
	dev := NewVMEngine(VMConfig{EnableSecurityChecks: true, ContractStorageDir: dir})
	c, err := dev.Compile(`package main;func F() int{return 1}`)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := dev.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	strict := NewVMEngine(VMConfig{RequireDeterminism: true, ContractStorageDir: dir})
	if _, err := strict.Deploy(c); err == nil {
		t.Fatal("development artifact deployed under deterministic policy")
	}
	if _, err := strict.Execute(addr, "F"); err == nil {
		t.Fatal("development artifact executed under deterministic policy")
	}
}

func TestDeterministicArtifactPolicyPersistsAndSeparatesCache(t *testing.T) {
	dir := t.TempDir()
	src := `package main;func F() int{return 1}`
	dev, err := NewContractCompilerWithOptions(dir, true).Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	e := NewVMEngine(VMConfig{RequireDeterminism: true, ContractStorageDir: dir})
	c, err := e.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	if dev.ExecutablePath == c.ExecutablePath {
		t.Fatal("source policies share an artifact cache")
	}
	addr, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewVMEngine(VMConfig{RequireDeterminism: true, ContractStorageDir: dir})
	if _, err := reloaded.Execute(addr, "F"); err != nil {
		t.Fatal(err)
	}
}

func TestDeterministicPolicyRejectsCompilerDirectives(t *testing.T) {
	src := "package main\n//go:nosplit\nfunc F(){}"
	if err := ReviewDeterministic(src); err == nil {
		t.Fatal("compiler directive accepted")
	}
}

func TestDeterministicBuildUsesNativeTarget(t *testing.T) {
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "386")
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("GOFLAGS", "-invalid-ambient-flag")
	t.Setenv("GOEXPERIMENT", "invalid-ambient-experiment")
	t.Setenv("GOWORK", "/missing/ambient/go.work")
	e := NewVMEngine(VMConfig{RequireDeterminism: true, ContractStorageDir: t.TempDir()})
	c, err := e.Compile(`package main;func F() int64{return 7}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.SourcePolicy != DeterministicPolicyVersion {
		t.Fatal("missing source policy")
	}
	addr, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Execute(addr, "F")
	if err != nil {
		t.Fatal(err)
	}
	if string(r) != "7" {
		t.Fatalf("bad result %s", r)
	}
}
