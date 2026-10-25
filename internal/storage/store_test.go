package storage

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"os"
	"path/filepath"
	"testing"
)

func TestPutDoesNotMutateAndPersistsGuarantees(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(source, []byte("artifact"), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := &core.CompiledContract{ExecutablePath: source, SourcePolicy: "custom-v2", Guarantees: core.PolicyGuarantees{Deterministic: true}, ProtocolVersion: core.ProtocolVersion, ABI: &abi.ABI{Version: abi.SchemaVersion}}
	if err := store.Put(context.Background(), "contract_test", c); err != nil {
		t.Fatal(err)
	}
	if c.ExecutablePath != source || c.Address != "" {
		t.Fatal("storage mutated compile artifact")
	}
	got, err := store.Get(context.Background(), "contract_test")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Guarantees.Deterministic || got.SourcePolicy != "custom-v2" {
		t.Fatalf("lost policy metadata: %+v", got)
	}
}
func TestCanceledStoreHasNoSideEffects(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Put(ctx, "contract_test", &core.CompiledContract{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "contract_test"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "contract_test")); !os.IsNotExist(err) {
		t.Fatalf("canceled write touched storage: %v", err)
	}
}

func TestPutReplacesCompleteRecord(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inputs := t.TempDir()
	a := filepath.Join(inputs, "a")
	z := filepath.Join(inputs, "z")
	for path, content := range map[string]string{a: "old", z: "new"} {
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	first := &core.CompiledContract{ExecutablePath: a, ProtocolVersion: core.ProtocolVersion, ABI: &abi.ABI{Version: abi.SchemaVersion}, GasProfile: &core.GasProfile{Version: 1}}
	if err := store.Put(context.Background(), "same", first); err != nil {
		t.Fatal(err)
	}
	second := &core.CompiledContract{ExecutablePath: z, ProtocolVersion: core.ProtocolVersion, ABI: &abi.ABI{Version: abi.SchemaVersion}}
	if err := store.Put(context.Background(), "same", second); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(got.ExecutablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != "new" {
		t.Fatalf("loaded stale executable: %s", bytes)
	}
	if got.GasProfile != nil {
		t.Fatal("retained old optional GasProfile")
	}
}
