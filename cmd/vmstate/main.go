// vmstate performs exclusive, offline maintenance. Embedded hosts may use the
// optional core.StateBackupper/StateInspector capabilities for online snapshots.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/lengzhao/vm/internal/statefile"
	bolt "go.etcd.io/bbolt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/lengzhao/vm/core"
	stateplugin "github.com/lengzhao/vm/plugins/state"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("usage: vmstate backup|restore|status -db EXISTING_DB [-out NEW_DB] [-max-batch-receipt-mb 64]")
	}
	op := args[0]
	if op != "backup" && op != "restore" && op != "status" {
		return fmt.Errorf("unknown operation %q", op)
	}
	flags := flag.NewFlagSet(op, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("db", "", "existing database; stop its owner before CLI maintenance")
	dest := flags.String("out", "", "new database path; never overwritten")
	limit := flags.Int("max-batch-receipt-mb", 64, "receipt limit for reopened database")
	if err = flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *path == "" || (op != "status" && *dest == "") || (op == "status" && *dest != "") {
		return fmt.Errorf("invalid arguments: -db required; backup/restore also require -out")
	}
	info, err := os.Stat(*path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("source must be a nonempty regular database file")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// ReadOnly enforces source immutability and supports read-only backup media.
	// Its shared lock still conflicts with a live VM's exclusive writer lock.
	db, err := bolt.Open(*path, 0400, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	s := snapshotSource{db}
	if op == "status" {
		tmp, e := stageSnapshot(ctx, s, os.TempDir())
		if e != nil {
			return e
		}
		defer os.Remove(tmp)
		stats, e := inspectSnapshot(ctx, tmp, *limit)
		if e != nil {
			return e
		}
		stats.DatabaseBytes = info.Size()
		return json.NewEncoder(out).Encode(stats)
	}

	if err = writeSnapshot(ctx, s, *dest, *limit); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, *dest)
	return err
}

type snapshotSource struct{ db *bolt.DB }

func (s snapshotSource) Backup(ctx context.Context, w io.Writer) error {
	return statefile.Copy(ctx, s.db, w)
}

// stageSnapshot never publishes the partial file; the caller removes a successful
// stage after inspection/publication. Source files are never opened for writing.
func stageSnapshot(ctx context.Context, s core.StateBackupper, dir string) (path string, err error) {
	f, err := os.CreateTemp(dir, ".vmstate-*")
	if err != nil {
		return "", err
	}
	path = f.Name()
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	if err = s.Backup(ctx, f); err != nil {
		return path, errors.Join(err, f.Close())
	}
	err = errors.Join(f.Sync(), f.Close())
	return path, err
}
func inspectSnapshot(ctx context.Context, path string, limit int) (out core.StateStats, err error) {
	s, err := stateplugin.NewBolt(stateplugin.BoltConfig{Path: path, MaxBatchReceiptMB: limit})
	if err != nil {
		return out, err
	}
	if err = s.Start(ctx); err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, s.Stop(context.Background())) }()
	return s.StateStats(ctx)
}

// Validate the destination copy, then atomically link without replacement.
func writeSnapshot(ctx context.Context, s core.StateBackupper, dest string, limit int) (err error) {
	dir := filepath.Dir(dest)
	tmp, err := stageSnapshot(ctx, s, dir)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err = inspectSnapshot(ctx, tmp, limit); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Link(tmp, dest); err != nil {
		return err
	}
	// Preserve published files if directory synchronization fails; report the path.
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("snapshot published at %s, directory sync unavailable: %w", dest, err)
	}
	if err = errors.Join(d.Sync(), d.Close()); err != nil {
		return fmt.Errorf("snapshot published at %s, directory sync failed: %w", dest, err)
	}
	return nil
}
