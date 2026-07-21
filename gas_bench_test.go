package vm

import (
	"testing"
	"time"
)

func benchEngine(b *testing.B) *VMEngine {
	b.Helper()
	return NewVMEngine(VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   b.TempDir(),
	})
}

func BenchmarkExecuteAdd(b *testing.B) {
	engine := benchEngine(b)
	source := `
package main

func Add(a, b int) int {
	return a + b
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		b.Fatalf("deploy: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Execute(address, "Add", 10, 20); err != nil {
			b.Fatalf("execute: %v", err)
		}
	}
}

func BenchmarkExecuteLoop(b *testing.B) {
	engine := benchEngine(b)
	source := `
package main

func Loop() int {
	sum := 0
	for i := 0; i < 100; i++ {
		sum = sum + i
	}
	return sum
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		b.Fatalf("deploy: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Execute(address, "Loop"); err != nil {
			b.Fatalf("execute: %v", err)
		}
	}
}

func BenchmarkEstimateGasStatic(b *testing.B) {
	engine := benchEngine(b)
	source := `
package main

import "github.com/lengzhao/vm/contractapi"

func Info() {
	contractapi.Log("InfoCalled", "ok", true)
}
`
	compiled, err := engine.Compile(source)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	address, err := engine.Deploy(compiled)
	if err != nil {
		b.Fatalf("deploy: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		est, err := engine.EstimateGas(address, "Info", nil)
		if err != nil {
			b.Fatalf("EstimateGas: %v", err)
		}
		if est.Estimated == 0 {
			b.Fatal("Estimated == 0")
		}
	}
}
