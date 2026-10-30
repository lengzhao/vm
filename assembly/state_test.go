package assembly_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	stateplugin "github.com/lengzhao/vm/plugins/state"
)

type beginOnlyState struct {
	backend core.StateStore
	begins  atomic.Int64
}

func (s *beginOnlyState) Begin(ctx context.Context) (core.StateTransaction, error) {
	s.begins.Add(1)
	return s.backend.Begin(ctx)
}
func init() {
	pluginkit.Register("test/begin-only-state", func(testConfig) (core.StateStore, error) {
		s, err := stateplugin.New(stateplugin.Config{})
		return &beginOnlyState{backend: s}, err
	})
}
func TestEngineAcceptsBeginOnlyStatePlugin(t *testing.T) {
	ctx := context.Background()
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["state"] = map[string]any{"use": "test/begin-only-state"}
	rt, err := assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(ctx)
	c, err := rt.Engine.Compile(ctx, `package main;import "github.com/lengzhao/vm/contractapi";func F()error{return contractapi.DefaultObject().Set("x",[]byte("y"))}`)
	if err != nil {
		t.Fatal(err)
	}
	address, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test", Sender: "alice"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: address, Function: "F"}); err != nil {
		t.Fatal(err)
	}
	s, ok := build.GetByID[*beginOnlyState](rt.Result, "state")
	if !ok || s.begins.Load() != 1 {
		t.Fatal("engine did not use minimal state plugin")
	}
	tx, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	value, exists, err := tx.Get(ctx, core.StateKey{Contract: core.Address(address), Key: "x"})
	if err != nil || !exists || string(value) != "y" {
		t.Fatalf("state=%q %v %v", value, exists, err)
	}
}
