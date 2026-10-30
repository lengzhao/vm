package assembly_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	_ "github.com/lengzhao/vm/plugins/all"
)

type testConfig struct{}
type answerRunner struct{}

func (*answerRunner) Run(context.Context, *core.CompiledContract, core.CallRequest) (*core.CallResult, error) {
	return &core.CallResult{Data: json.RawMessage(`42`), GasConsumed: 7}, nil
}
func init() {
	pluginkit.Register("test/answer", func(testConfig) (core.Runner, error) { return &answerRunner{}, nil })
}

func TestDefaultPipeline(t *testing.T) {
	rt, err := assembly.Build(context.Background(), assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: true, EnableGasMetering: true, MaxGasLimit: 100}))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Add(a,b int) int { return a+b }")
	if err != nil {
		t.Fatal(err)
	}
	address, err := rt.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test-chain", Sender: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: address, Function: "Add", Args: []any{2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data.Data) != "5" {
		t.Fatalf("got %s", data.Data)
	}
}

func TestReplaceRunner(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["runner"] = map[string]any{"use": "test/answer"}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Answer() int {return 1}")
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test-chain", Sender: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Answer"})
	if err != nil || string(got.Data) != "42" {
		t.Fatalf("%s %v", got.Data, err)
	}
}

func TestInvalidGraphs(t *testing.T) {
	for _, mode := range []string{"missing", "cycle", "type", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
			switch mode {
			case "missing":
				delete(cfg.Plugins, "compiler")
			case "cycle":
				cfg.Plugins["runner"] = map[string]any{"use": "engine/default", "deps": map[string]any{"runner": "engine"}}
			case "type":
				cfg.Plugins["runner"] = map[string]any{"use": "abi/go"}
			case "unknown":
				cfg.Plugins["runner"] = map[string]any{"use": "missing/plugin"}
			}
			if rt, err := assembly.Build(context.Background(), cfg); err == nil {
				rt.Stop(context.Background())
				t.Fatal("invalid graph accepted")
			}
		})
	}
}

func TestLoadRejectsMalformedConfig(t *testing.T) {
	for _, s := range []string{"root: engine\nplugins: {}\nextra: true", "root: x\nplugins: {}\n---\nroot: y", "root: x", strings.Repeat(" ", assembly.MaxConfigBytes+1)} {
		if _, err := assembly.Load(context.Background(), strings.NewReader(s)); err == nil {
			t.Fatal("invalid YAML accepted")
		}
	}
}
func TestCanceledBuild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := assembly.Build(ctx, assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})); err == nil {
		t.Fatal("canceled build accepted")
	}
}

func TestDefaultPresetLoads(t *testing.T) {
	data, err := os.ReadFile("../presets/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	yaml := strings.ReplaceAll(string(data), "./contracts", t.TempDir())
	rt, err := assembly.Load(context.Background(), strings.NewReader(yaml))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Add(a,b int)int{return a+b}")
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test-chain", Sender: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Add", Args: []any{2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Data) != "5" || out.GasConsumed == 0 {
		t.Fatalf("preset execution: %+v", out)
	}
}
