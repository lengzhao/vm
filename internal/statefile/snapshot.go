// Package statefile shares consistent bbolt snapshot copying between the live
// backend and the read-only maintenance CLI. Callers own database lifecycle.
package statefile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"io"
)

const RootBucket = "vm-state"
const ValuesBucket = "values"
const SchemaKey = "schema"

// ValidateSchema rejects uninitialized/foreign databases instead of upgrading
// them into empty VM backups. Initialization belongs only to the live backend.
func ValidateSchema(tx *bolt.Tx) error {
	root := tx.Bucket([]byte(RootBucket))
	if root == nil || !bytes.Equal(root.Get([]byte(SchemaKey)), []byte{1}) || root.Bucket([]byte(ValuesBucket)) == nil {
		return fmt.Errorf("unsupported or missing VM state database schema")
	}
	return nil
}

func Copy(ctx context.Context, db *bolt.DB, dst io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.View(func(tx *bolt.Tx) error {
		if err := ValidateSchema(tx); err != nil {
			return err
		}
		w := &contextWriter{ctx: ctx, dst: dst}
		_, err := tx.WriteTo(w)
		if err != nil {
			return errors.Join(err, w.err)
		}
		return ctx.Err()
	})
}

type contextWriter struct {
	ctx context.Context
	dst io.Writer
	err error
}

func (w *contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return 0, err
	}
	n, err := w.dst.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
