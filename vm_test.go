package vm_test

import (
	"context"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"os"
	"path/filepath"
	"testing"

	vm "github.com/lengzhao/vm"
)

type compatibilityRunner struct{ started, stopped bool }

func (r *compatibilityRunner) Run(context.Context, *vm.CompiledContract, vm.CallRequest) (*vm.CallResult, error) {
	return &vm.CallResult{Data: []byte(`99`)}, nil
}
func (r *compatibilityRunner) Start(context.Context) error { r.started = true; return nil }
func (r *compatibilityRunner) Stop(context.Context) error  { r.stopped = true; return nil }
func TestCompatibilityFacadeAndRunnerLifecycle(t *testing.T) {
	r := &compatibilityRunner{}
	e := vm.NewVMEngineWithRunner(vm.VMConfig{ContractStorageDir: t.TempDir()}, r)
	if !r.started {
		t.Fatal("programmatic runner not started")
	}
	c, err := e.Compile("package main\nfunc Answer() int{return 1}")
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.Execute(a, "Answer")
	if err != nil || string(got) != "99" {
		t.Fatalf("%s %v", got, err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if !r.stopped {
		t.Fatal("programmatic runner not stopped")
	}
}
func TestOpenReportsStorageError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.OpenVMEngine(context.Background(), vm.VMConfig{ContractStorageDir: path}); err == nil {
		t.Fatal("storage initialization error hidden")
	}
}
func TestStandaloneCompilerCompatibility(t *testing.T) {
	compiler := vm.NewContractCompilerWithOptions(t.TempDir(), true)
	c, err := compiler.Compile("package main\nfunc Answer() int{return 3}")
	if err != nil {
		t.Fatal(err)
	}
	r := vm.NewProcessRunner(0)
	result, err := r.Run(context.Background(), c, vm.CallRequest{Function: "Answer"})
	if err != nil || string(result.Data) != "3" {
		t.Fatalf("%v %v", result, err)
	}
}

func TestLegacyConstructorsAcceptTheSameDevelopmentArtifacts(t *testing.T) {
	compiled, err := vm.NewContractCompilerWithOptions(t.TempDir(), true).Compile("package main\nfunc Answer()int{return 1}")
	if err != nil {
		t.Fatal(err)
	}
	compiled.SourcePolicy = "foreign-development-v2"
	// Simulate a trusted producer using a foreign policy, with matching integrity metadata.
	artifacttest.Seal(t, compiled)
	for _, tt := range []struct {
		name string
		new  func(vm.VMConfig) *vm.VMEngine
	}{
		{"default", vm.NewVMEngine},
		{"nil-runner", func(c vm.VMConfig) *vm.VMEngine { return vm.NewVMEngineWithRunner(c, nil) }},
		{"custom-runner", func(c vm.VMConfig) *vm.VMEngine { return vm.NewVMEngineWithRunner(c, &compatibilityRunner{}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.new(vm.VMConfig{ContractStorageDir: t.TempDir()})
			defer e.Stop()
			copy := *compiled
			if _, err := e.Deploy(&copy); err != nil {
				t.Fatalf("constructor changed development artifact acceptance: %v", err)
			}
		})
	}
}
