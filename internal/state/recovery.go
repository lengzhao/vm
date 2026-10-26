package state

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/lengzhao/vm/core"
	bolt "go.etcd.io/bbolt"
)

var boltPending = []byte("pending-batch-v1")

var boltBatches = []byte("batches-v1")
var boltBatchHeader = []byte("header")

const maxReceiptBytes = 4 << 20

var _ core.RecoverableBatchStore = (*Bolt)(nil)

type batchHeader struct {
	Identity core.BatchIdentity
	Bytes    int
}
type boltBatchSession struct {
	owner    *Bolt
	identity core.BatchIdentity
	ctx      context.Context
	cancel   context.CancelFunc
	receipts [][]byte
	bytes    int
}

func validBatchIdentity(id core.BatchIdentity) bool {
	digest, e := hex.DecodeString(id.Fingerprint)
	return utf8.ValidString(id.ID) && len(id.ID) > 0 && len(id.ID) <= 256 && id.Count >= 0 && id.Count <= 65536 && e == nil && len(digest) == 32
}
func (s *Bolt) AcquireBatch(ctx context.Context, id core.BatchIdentity) (core.BatchSession, error) {
	if !validBatchIdentity(id) {
		return nil, fmt.Errorf("invalid batch identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting++
	defer func() { s.waiting--; s.notify() }()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.available(); err != nil {
			return nil, err
		}
		if s.session == nil && len(s.active) == 0 {
			if s.pendingID != "" && s.pendingID != id.ID {
				return nil, fmt.Errorf("%w: %s", core.ErrBatchPending, s.pendingID)
			}
			break
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		s.mu.Lock()
	}
	session := &boltBatchSession{owner: s, identity: id}
	var found bool
	err := s.db.View(func(tx *bolt.Tx) error {
		batches := tx.Bucket(boltRoot).Bucket(boltBatches)
		if batches == nil {
			return nil
		}
		b := batches.Bucket([]byte(id.ID))
		if b == nil {
			return nil
		}
		found = true
		var header batchHeader
		if err := decodeBatchJSON(b.Get(boltBatchHeader), &header); err != nil {
			return err
		}
		if header.Identity != id {
			return core.ErrBatchIdentityConflict
		}
		err := b.ForEach(func(k, v []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if bytes.Equal(k, boltBatchHeader) {
				return nil
			}
			if len(k) != 8 || v == nil || len(v) > maxReceiptBytes || binary.BigEndian.Uint64(k) != uint64(len(session.receipts)) {
				return fmt.Errorf("corrupt batch receipt sequence")
			}
			var receipt core.BatchReceipt
			if err := decodeBatchJSON(v, &receipt); err != nil {
				return err
			}
			if receipt.Index != len(session.receipts) || receipt.Index >= id.Count || !validReceipt(receipt) {
				return fmt.Errorf("corrupt batch receipt")
			}
			session.bytes += len(v)
			if session.bytes > s.options.MaxBatchReceiptMB<<20 {
				return fmt.Errorf("batch receipt byte limit exceeded")
			}
			session.receipts = append(session.receipts, bytes.Clone(v))
			return nil
		})
		if err != nil {
			return err
		}
		if header.Bytes != session.bytes {
			return fmt.Errorf("corrupt batch receipt accounting")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found || (len(session.receipts) < id.Count && s.pendingID == "") {
		data, err := json.Marshal(batchHeader{Identity: id, Bytes: session.bytes})
		if err != nil {
			return nil, err
		}
		if err := s.persist(nil, ctx.Err, func(tx *bolt.Tx) error {
			root := tx.Bucket(boltRoot)
			batches, err := root.CreateBucketIfNotExists(boltBatches)
			if err != nil {
				return err
			}
			b := batches.Bucket([]byte(id.ID))
			if b == nil {
				b, err = batches.CreateBucket([]byte(id.ID))
				if err != nil {
					return err
				}
			}
			if err := b.Put(boltBatchHeader, data); err != nil {
				return err
			}
			if len(session.receipts) < id.Count {
				return root.Put(boltPending, []byte(id.ID))
			}
			return nil
		}); err != nil {
			return nil, err
		}
		if len(session.receipts) < id.Count {
			s.pendingID = id.ID
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session.ctx, session.cancel = context.WithCancel(ctx)
	s.session = session
	return session, nil
}
func (b *boltBatchSession) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.owner.available(); err != nil {
		return err
	}
	if b.owner.session != b {
		return core.ErrStateClosed
	}
	return b.ctx.Err()
}
func (b *boltBatchSession) BeginSnapshot(ctx context.Context) (core.SnapshotTransaction, error) {
	return b.owner.beginSnapshot(ctx, b)
}
func (b *boltBatchSession) Prefix(ctx context.Context) ([]core.BatchReceipt, error) {
	b.owner.mu.Lock()
	defer b.owner.mu.Unlock()
	if err := b.check(ctx); err != nil {
		return nil, err
	}
	out := make([]core.BatchReceipt, len(b.receipts))
	for i, data := range b.receipts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := decodeBatchJSON(data, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func validReceipt(r core.BatchReceipt) bool {
	if r.Failure == nil {
		return r.Result != nil
	}
	return r.Failure.Code != "" && len(r.Failure.Code) <= 128 && len(r.Failure.Message) <= 1<<20 && (r.Result == nil || (len(r.Result.Data) == 0 && len(r.Result.Events) == 0))
}
func (b *boltBatchSession) Finalize(ctx context.Context, tx core.SnapshotTransaction, r core.BatchReceipt) error {
	s := b.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := b.check(ctx); err != nil {
		return err
	}
	t, ok := tx.(*boltTransaction)
	if !ok || t.owner != s || t.session != b {
		return fmt.Errorf("snapshot does not belong to batch session")
	}
	if r.Index != len(b.receipts) || r.Index >= b.identity.Count || !validReceipt(r) {
		return fmt.Errorf("invalid or nonconsecutive batch receipt")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(data) > maxReceiptBytes || b.bytes+len(data) > s.options.MaxBatchReceiptMB<<20 {
		return fmt.Errorf("batch receipt byte limit exceeded")
	}
	// Allocate receipt publication before crossing the durable commit boundary.
	next := make([][]byte, len(b.receipts)+1)
	copy(next, b.receipts)
	next[len(b.receipts)] = data
	size := b.bytes + len(data)
	header, err := json.Marshal(batchHeader{Identity: b.identity, Bytes: size})
	if err != nil {
		return err
	}
	err = t.mvccTransaction.finalize(ctx, r.Failure == nil, func(writes map[core.StateKey]core.StateWrite) error {
		return s.persist(writes, func() error {
			if err := b.check(ctx); err != nil {
				return err
			}
			return t.check(ctx)
		}, func(disk *bolt.Tx) error {
			bucket := disk.Bucket(boltRoot).Bucket(boltBatches).Bucket([]byte(b.identity.ID))
			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, uint64(r.Index))
			if err := bucket.Put(key, data); err != nil {
				return err
			}
			if err := bucket.Put(boltBatchHeader, header); err != nil {
				return err
			}
			if r.Index+1 == b.identity.Count {
				return disk.Bucket(boltRoot).Delete(boltPending)
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	b.receipts, b.bytes = next, size
	if len(next) == b.identity.Count {
		s.pendingID = ""
	}
	delete(s.active, t)
	t.cancel()
	s.notify()
	return nil
}
func (b *boltBatchSession) Close() error {
	s := b.owner
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != b {
		return nil
	}
	b.cancel()
	for tx := range s.active {
		if tx.session == b {
			tx.mvccTransaction.Rollback()
			tx.cancel()
			delete(s.active, tx)
		}
	}
	s.session = nil
	s.notify()
	return nil
}
func decodeBatchJSON(data []byte, value any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid batch record trailing data")
	}
	return nil
}

// persist runs with Bolt.mu held. The caller has prepared in-memory publication.
func (s *Bolt) persist(writes map[core.StateKey]core.StateWrite, check func() error, extra func(*bolt.Tx) error) error {
	if err := check(); err != nil {
		return err
	}
	disk, err := s.db.Begin(true)
	if err != nil {
		return err
	}
	defer disk.Rollback()
	values := disk.Bucket(boltRoot).Bucket(boltValues)
	for k, w := range writes {
		if err := check(); err != nil {
			return err
		}
		if w.Deleted {
			err = values.Delete(encodeStateKey(k))
		} else {
			err = values.Put(encodeStateKey(k), w.Value)
		}
		if err != nil {
			return err
		}
	}
	if extra != nil {
		if err := extra(disk); err != nil {
			return err
		}
	}
	if err := check(); err != nil {
		return err
	}
	if err := s.commitDisk(disk); err != nil {
		s.fault = errors.Join(core.ErrStateCommitUncertain, err)
		for tx := range s.active {
			tx.cancel()
		}
		clear(s.active)
		if s.session != nil {
			s.session.cancel()
		}
		s.notify()
		return s.fault
	}
	return nil
}
