package storage

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
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
	if err := store.Put(context.Background(), "contract_test", artifacttest.Seal(t, c)); err != nil {
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
	if err := store.Put(context.Background(), "same", artifacttest.Seal(t, first)); err != nil {
		t.Fatal(err)
	}
	second := &core.CompiledContract{ExecutablePath: z, ProtocolVersion: core.ProtocolVersion, ABI: &abi.ABI{Version: abi.SchemaVersion}}
	if err := store.Put(context.Background(), "same", artifacttest.Seal(t, second)); err != nil {
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

func TestCreateIdempotentAndRejectsReplacement(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	original := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	if err := store.Create(context.Background(), "same", original); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "same", "metadata.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "same", original); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("retry rewrote existing metadata")
	}
	replacement := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourceHash: "different"})
	err = store.Create(context.Background(), "same", replacement)
	var ee *core.ExecutionError
	if !errors.As(err, &ee) || ee.Code != core.CodeDeploymentConflict {
		t.Fatalf("replacement not rejected: %v", err)
	}
	current, err := store.Get(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	if current.ManifestSHA256 != original.ManifestSHA256 {
		t.Fatal("conflict overwrote original")
	}
}

func TestCreateConcurrentAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	first, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	start := make(chan struct{})
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		store := first
		if i%2 == 1 {
			store = second
		}
		go func() { <-start; done <- store.Create(context.Background(), "same", record) }()
	}
	close(start)
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "same" {
		t.Fatalf("partial records left behind: %v", entries)
	}
	got, err := first.Get(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	if got.ManifestSHA256 != record.ManifestSHA256 {
		t.Fatal("wrong published record")
	}
}
func TestCreateConcurrentConflictsDoNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	first, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourceHash: "a"})
	b := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourceHash: "b"})
	start := make(chan struct{})
	done := make(chan error, 2)
	go func() { <-start; done <- first.Create(context.Background(), "same", a) }()
	go func() { <-start; done <- second.Create(context.Background(), "same", b) }()
	close(start)
	success, conflict := 0, 0
	for i := 0; i < 2; i++ {
		err := <-done
		if err == nil {
			success++
			continue
		}
		var ee *core.ExecutionError
		if !errors.As(err, &ee) || ee.Code != core.CodeDeploymentConflict {
			t.Fatal(err)
		}
		conflict++
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("unexpected race outcome: %d success %d conflict", success, conflict)
	}
	got, err := first.Get(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	if got.ManifestSHA256 != a.ManifestSHA256 && got.ManifestSHA256 != b.ManifestSHA256 {
		t.Fatal("record was partially mixed")
	}
}
func TestCreateRejectsDamagedRecordAndCancellation(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	if err := store.Create(context.Background(), "same", record); err != nil {
		t.Fatal(err)
	}
	damaged := filepath.Join(dir, "same", "artifact")
	if err := os.WriteFile(damaged, []byte("damaged"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "same", record); err == nil {
		t.Fatal("damaged record silently repaired")
	}
	bytes, err := os.ReadFile(damaged)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != "damaged" {
		t.Fatal("damaged record overwritten")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Create(ctx, "canceled", record); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "canceled")); !os.IsNotExist(err) {
		t.Fatal("canceled creation wrote files")
	}
}
