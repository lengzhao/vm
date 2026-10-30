package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	bolt "go.etcd.io/bbolt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lengzhao/vm/core"
	stateplugin "github.com/lengzhao/vm/plugins/state"
)

func TestBackupRestoreAndRefuseOverwrite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	live := filepath.Join(dir, "live.db")
	s, e := stateplugin.NewBolt(stateplugin.BoltConfig{Path: live})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	tx, e := s.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Set(ctx, core.StateKey{Key: "k"}, []byte("value")); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(dir, "backup.db")
	restored := filepath.Join(dir, "restored.db")
	var out bytes.Buffer
	for _, args := range [][]string{
		{"backup", "-db", live, "-out", backup},
		{"restore", "-db", backup, "-out", restored},
		{"status", "-db", restored},
	} {
		if e = run(ctx, args, &out); e != nil {
			t.Fatalf("%v: %v", args, e)
		}
	}
	if !bytes.Contains(out.Bytes(), []byte(`"StateKeys":1`)) {
		t.Fatal(out.String())
	}
	if e = run(ctx, []string{"restore", "-db", backup, "-out", restored}, &out); e == nil {
		t.Fatal("overwrote existing destination")
	}
	r, e := stateplugin.NewBolt(stateplugin.BoltConfig{Path: restored})
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer r.Stop(ctx)
	read, e := r.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer read.Rollback()
	v, ok, e := read.Get(ctx, core.StateKey{Key: "k"})
	if e != nil || !ok || string(v) != "value" {
		t.Fatal(string(v), ok, e)
	}
}
func TestMaintenanceCLIRejectsMissingAndCorruptSources(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.db")
	os.WriteFile(bad, []byte("corrupt"), 0600)
	for _, source := range []string{bad, filepath.Join(dir, "missing.db")} {
		dest := filepath.Join(dir, "out.db")
		if e := run(context.Background(), []string{"restore", "-db", source, "-out", dest}, &bytes.Buffer{}); e == nil {
			t.Fatal("bad source accepted")
		}
		if _, e := os.Stat(dest); !os.IsNotExist(e) {
			t.Fatal("failed restore published", e)
		}
	}
	if _, e := os.Stat(filepath.Join(dir, "missing.db")); !os.IsNotExist(e) {
		t.Fatal("missing source created")
	}
}

func TestMaintenanceNeverMutatesReadOnlySource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "source.db")
	s, e := stateplugin.NewBolt(stateplugin.BoltConfig{Path: path})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Start(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(path, 0400); e != nil {
		t.Fatal(e)
	}
	for _, op := range []string{"backup", "restore", "status"} {
		args := []string{op, "-db", path}
		if op != "status" {
			args = append(args, "-out", filepath.Join(dir, op+".db"))
		}
		if e = run(ctx, args, &bytes.Buffer{}); e != nil {
			t.Fatalf("%s read-only source: %v", op, e)
		}
		after, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if sha256.Sum256(after) != sha256.Sum256(original) {
			t.Fatalf("%s modified source", op)
		}
	}
}

func TestMaintenanceRejectsUninitializedBoltDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foreign.db")
	db, e := bolt.Open(path, 0600, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	for _, op := range []string{"backup", "restore", "status"} {
		dest := filepath.Join(dir, op+".db")
		args := []string{op, "-db", path}
		if op != "status" {
			args = append(args, "-out", dest)
		}
		if e = run(context.Background(), args, &bytes.Buffer{}); e == nil {
			t.Fatalf("%s accepted non-VM database", op)
		}
		if _, e = os.Stat(dest); !os.IsNotExist(e) {
			t.Fatal("invalid source published", e)
		}
	}
}
