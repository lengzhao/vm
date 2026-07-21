package vm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func compileTestContract(t *testing.T, dir, source string) *CompiledContract {
	t.Helper()
	compiler := NewContractCompilerWithOptions(dir, true)
	compiled, err := compiler.Compile(source)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	return compiled
}

func TestProcessRunnerSuccess(t *testing.T) {
	dir := t.TempDir()
	contract := compileTestContract(t, dir, `
package main

func Add(a, b int) int {
	return a + b
}
`)

	runner := NewProcessRunner(5 * time.Second)
	result, err := runner.Run(context.Background(), contract, CallRequest{
		Function: "Add",
		Args:     []interface{}{3, 4},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if string(result.Data) != "7" {
		t.Fatalf("expected 7, got %s", string(result.Data))
	}
	if result.GasConsumed < 10 {
		t.Fatalf("expected gas >= 10, got %d", result.GasConsumed)
	}
}

func TestProcessRunnerUnknownFunction(t *testing.T) {
	dir := t.TempDir()
	contract := compileTestContract(t, dir, `
package main

func Add(a, b int) int {
	return a + b
}
`)

	runner := NewProcessRunner(5 * time.Second)
	_, err := runner.Run(context.Background(), contract, CallRequest{
		Function: "Missing",
		Args:     []interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for unknown function")
	}
	if !strings.Contains(err.Error(), "unknown function") {
		t.Fatalf("expected unknown function error, got %v", err)
	}
}

func TestProcessRunnerValidation(t *testing.T) {
	runner := NewProcessRunner(time.Second)

	_, err := runner.Run(context.Background(), nil, CallRequest{Function: "Add"})
	if err == nil {
		t.Fatal("expected nil contract error")
	}

	_, err = runner.Run(context.Background(), &CompiledContract{}, CallRequest{Function: "Add"})
	if err == nil {
		t.Fatal("expected empty executable path error")
	}

	missing := &CompiledContract{ExecutablePath: filepath.Join(t.TempDir(), "missing")}
	_, err = runner.Run(context.Background(), missing, CallRequest{Function: "Add"})
	if err == nil {
		t.Fatal("expected missing executable error")
	}

	dir := t.TempDir()
	contract := compileTestContract(t, dir, `
package main
func Add(a, b int) int { return a + b }
`)
	_, err = runner.Run(context.Background(), contract, CallRequest{Function: ""})
	if err == nil {
		t.Fatal("expected empty function error")
	}
}

func TestProcessRunnerInvalidJSONResponse(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "bad_contract")
	script := "#!/bin/sh\necho 'not-json'\nexit 1\n"
	if err := os.WriteFile(execPath, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runner := NewProcessRunner(time.Second)
	result, err := runner.Run(context.Background(), &CompiledContract{ExecutablePath: execPath}, CallRequest{
		Function: "Any",
	})
	if err == nil {
		t.Fatal("expected invalid response error")
	}
	if result == nil || result.Stdout == "" {
		t.Fatal("expected stdout to be captured")
	}
}

func TestProcessRunnerNonZeroExitWithErrorJSON(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "fail_contract")
	script := "#!/bin/sh\necho '{\"ok\":false,\"error\":\"boom\",\"gas\":3}'\nexit 1\n"
	if err := os.WriteFile(execPath, []byte(script), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runner := NewProcessRunner(time.Second)
	result, err := runner.Run(context.Background(), &CompiledContract{ExecutablePath: execPath}, CallRequest{
		Function: "Any",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected boom error, got %v", err)
	}
	if result.GasConsumed != 3 {
		t.Fatalf("expected gas 3, got %d", result.GasConsumed)
	}
}

func TestProcessRunnerTimeout(t *testing.T) {
	dir := t.TempDir()
	contract := compileTestContract(t, dir, `
package main

import "time"

func Hang() {
	time.Sleep(5 * time.Second)
}
`)

	runner := NewProcessRunner(200 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := runner.Run(ctx, contract, CallRequest{Function: "Hang"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestProcessRunnerGasLimit(t *testing.T) {
	dir := t.TempDir()
	contract := compileTestContract(t, dir, `
package main

func Loop() int {
	sum := 0
	for i := 0; i < 1000; i++ {
		sum = sum + i
	}
	return sum
}
`)

	runner := NewProcessRunner(5 * time.Second)
	ctx := WithGasLimit(context.Background(), 20)
	_, err := runner.Run(ctx, contract, CallRequest{Function: "Loop"})
	if err == nil {
		t.Fatal("expected gas limit exceeded error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "gas") {
		t.Fatalf("expected gas error, got %v", err)
	}
}
