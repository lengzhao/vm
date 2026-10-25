package assembly_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	policyplugin "github.com/lengzhao/vm/plugins/policy"
)

type compileOnly struct{}

func (compileOnly) Compile(context.Context, string) (*core.CompiledContract, error) {
	return nil, rejected
}

type reviewOnly struct{}

func (reviewOnly) Review(string) error { return rejected }

type customDeterministic struct{ core.SourcePolicy }

func (customDeterministic) ID() string { return "test-strict-v2" }
func (customDeterministic) Guarantees() core.PolicyGuarantees {
	return core.PolicyGuarantees{Deterministic: true}
}

type reviewerDeps struct {
	Reviewer core.SecurityReviewer `json:"reviewer"`
}

func init() {
	pluginkit.Register("test/compile-only", func(testConfig) (core.ContractCompiler, error) { return compileOnly{}, nil })
	pluginkit.Register("test/review-only", func(testConfig) (core.SecurityReviewer, error) { return reviewOnly{}, nil })
	pluginkit.Register("test/custom-deterministic", func(_ testConfig, d reviewerDeps) (core.SourcePolicy, error) {
		p, err := policyplugin.New(policyplugin.Config{Deterministic: true}, policyplugin.Deps{Reviewer: d.Reviewer})
		return customDeterministic{SourcePolicy: p}, err
	})
	pluginkit.Register("test/memory-store", func(testConfig) (core.ContractStore, error) {
		return &memoryStore{records: map[string]core.CompiledContract{}}, nil
	})
}
func TestCompileOnlyPluginIsSufficient(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["compiler"] = map[string]any{"use": "test/compile-only"}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	if _, err := rt.Engine.Compile(context.Background(), "source"); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
}
func TestReviewerNeedsNoAllowlistMethods(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{EnableSecurityChecks: true, ContractStorageDir: t.TempDir()})
	cfg.Plugins["security"] = map[string]any{"use": "test/review-only"}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	if _, err := rt.Engine.GenerateABI(context.Background(), "source"); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
}
func TestCustomDeterminismAndIndependentStore(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{RequireDeterminism: true, ContractStorageDir: t.TempDir()})
	cfg.Plugins["policy"] = map[string]any{"use": "test/custom-deterministic", "deps": map[string]any{"reviewer": "security"}}
	// The store has no deployment, address generation, ABI-query or Gas methods.
	cfg.Plugins["contracts"] = map[string]any{"use": "test/memory-store"}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Answer() int{return 7}")
	if err != nil {
		t.Fatal(err)
	}
	path := c.ExecutablePath
	a, err := rt.Engine.Deploy(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Address != "" || c.ExecutablePath != path {
		t.Fatal("business pipeline mutated compiler output")
	}
	got, err := rt.Engine.Get(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourcePolicy != "test-strict-v2" || !got.Guarantees.Deterministic {
		t.Fatalf("lost policy contract: %+v", got)
	}
	out, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Answer"})
	if err != nil || string(out.Data) != "7" {
		t.Fatalf("%+v %v", out, err)
	}
}

type memoryStore struct {
	mu      sync.Mutex
	records map[string]core.CompiledContract
}

func (s *memoryStore) Put(ctx context.Context, a string, c *core.CompiledContract) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[a] = *c
	return nil
}
func (s *memoryStore) Get(ctx context.Context, a string) (*core.CompiledContract, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.records[a]
	if !ok {
		return nil, fmt.Errorf("contract not found: %s", a)
	}
	return &c, nil
}
