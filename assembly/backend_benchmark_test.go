package assembly_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
)

const backendBenchmarkSource = `package main
import "github.com/lengzhao/vm/contractapi"
func Add(a,b int64)int64 {return a+b}
func Loop(n int) int {v:=0;for i:=0;i<n;i++ {v+=i};return v}
func Write(v string)error {return contractapi.DefaultObject().Set("value",[]byte(v))}
func Child(address string)error {_,err:=contractapi.Call(address,"Write","value");return err}
`

// BenchmarkBackend measures complete Engine calls with fresh guest instances.
// Docker and WASI use the same immutable build image when VM_DOCKER_IMAGE is set.
func BenchmarkBackend(b *testing.B) {
	for _, backend := range []string{"process", "docker", "wazero"} {
		b.Run(backend, func(b *testing.B) {
			image := os.Getenv("VM_DOCKER_IMAGE")
			version := os.Getenv("VM_DOCKER_GO_VERSION")
			if backend == "docker" && image == "" {
				b.Skip("VM_DOCKER_IMAGE required")
			}
			cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: b.TempDir(), EnableSecurityChecks: true, RequireDeterminism: true, EnableGasMetering: true, MaxGasLimit: 1000000, ExecutionTimeout: 30 * time.Second})
			cc := cfg.Plugins["compiler"].(map[string]any)
			if backend != "process" && image != "" {
				cc["config"].(map[string]any)["sandbox"] = map[string]any{"image": image}
				cc["config"].(map[string]any)["goVersion"] = version
				cc["config"].(map[string]any)["buildTimeout"] = "2m"
			}
			switch backend {
			case "docker":
				cfg.Plugins["runner"] = map[string]any{"use": "runner/docker", "config": map[string]any{"sandbox": map[string]any{"image": image}}}
				cfg.Plugins["validator"].(map[string]any)["config"] = map[string]any{"targetOS": "linux"}
			case "wazero":
				cc["use"] = "compiler/wasip1"
				cfg.Plugins["runner"] = map[string]any{"use": "runner/wazero", "config": map[string]any{"engine": "compiler", "memoryMB": 256}}
				cfg.Plugins["validator"].(map[string]any)["config"] = map[string]any{"targetOS": "wasip1", "targetArch": "wasm"}
			}
			ctx := context.Background()
			rt, err := assembly.Build(ctx, cfg)
			if err != nil {
				b.Fatal(err)
			}
			defer rt.Stop(ctx)
			start := time.Now()
			c, err := rt.Engine.Compile(ctx, backendBenchmarkSource)
			buildMS := float64(time.Since(start)) / float64(time.Millisecond)
			if err != nil {
				b.Fatal(err)
			}
			info, err := os.Stat(c.ExecutablePath)
			if err != nil {
				b.Fatal(err)
			}
			deploy := func(n uint64) string {
				a, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "bench", Sender: "test", Nonce: n}})
				if err != nil {
					b.Fatal(err)
				}
				return a
			}
			a, child := deploy(0), deploy(1)
			b.Logf("toolchain=%s target=%s/%s image=%s build_ms=%.3f artifact_bytes=%d", c.Manifest.Build.Toolchain.Version, c.Manifest.Build.Toolchain.GOOS, c.Manifest.Build.Toolchain.GOARCH, c.Manifest.Build.Toolchain.ContainerImage, buildMS, info.Size())
			for _, test := range []struct {
				name, fn string
				args     []any
			}{{"Add", "Add", []any{int64(2), int64(3)}}, {"Loop1000", "Loop", []any{1000}}, {"ObjectWrite", "Write", []any{"value"}}, {"CrossCall", "Child", []any{child}}} {
				b.Run(test.name, func(b *testing.B) {
					req := core.ExecutionRequest{Address: a, Function: test.fn, Args: test.args}
					// Warm the host runtime/toolchain and validate before timing fresh guest calls.
					if _, err := rt.Engine.Execute(ctx, req); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := rt.Engine.Execute(ctx, req); err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					b.ReportMetric(buildMS, "build-ms")
					b.ReportMetric(float64(info.Size()), "artifact-bytes")
				})
			}
		})
	}
}
