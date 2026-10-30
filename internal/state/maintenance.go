package state

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/statefile"
	bolt "go.etcd.io/bbolt"
)

var _ core.StateBackupper = (*Bolt)(nil)
var _ core.StateInspector = (*Bolt)(nil)

// Backup holds the store gate so Commit/Finalize/Stop cannot race the read
// transaction or block bbolt mmap growth behind our own long-lived reader.
// Cancellation is observed between writes; an arbitrary blocked Writer cannot
// be forcibly interrupted. The caller must impose its own I/O deadline.
func (s *Bolt) Backup(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("backup writer required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.available(); err != nil {
		return err
	}
	return statefile.Copy(ctx, s.db, w)
}

func (s *Bolt) StateStats(ctx context.Context) (core.StateStats, error) {
	var out core.StateStats
	if err := ctx.Err(); err != nil {
		return out, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := s.available(); err != nil {
		return out, err
	}
	info, err := os.Stat(s.options.Path)
	if err != nil {
		return out, err
	}
	out.DatabaseBytes = info.Size()
	out.PendingBatchID = s.pendingID
	err = s.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(boltRoot)
		if err := root.Bucket(boltValues).ForEach(func(k, v []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			out.StateKeys++
			out.StateValueBytes += uint64(len(v))
			return nil
		}); err != nil {
			return err
		}
		batches := root.Bucket(boltBatches)
		if batches == nil {
			return nil
		}
		return batches.ForEach(func(k, v []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			b := batches.Bucket(k)
			if b == nil {
				return fmt.Errorf("invalid batch bucket")
			}
			out.Batches++
			return b.ForEach(func(key, value []byte) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if string(key) == string(boltBatchHeader) {
					return nil
				}
				out.Receipts++
				out.ReceiptBytes += uint64(len(value))
				return nil
			})
		})
	})
	if err != nil {
		return core.StateStats{}, err
	}
	return out, ctx.Err()
}
