package core

import (
	"context"
	"io"
)

// StateBackupper writes one consistent database snapshot, including batch
// identities, receipts and the pending fence. The caller owns the destination
// and must discard partial output on error. Artifacts/configuration/requests
// are separate backup responsibilities. Writers must not reenter the store.
type StateBackupper interface {
	Backup(context.Context, io.Writer) error
}

// StateStats is a point-in-time capacity inventory. Bytes are encoded payload
// sizes (not RSS); DatabaseBytes includes database free space. Enumerating state
// and receipts can be expensive. PendingBatchID is empty when no fence exists.
type StateStats struct {
	DatabaseBytes   int64
	StateKeys       uint64
	StateValueBytes uint64
	Batches         uint64
	Receipts        uint64
	ReceiptBytes    uint64
	PendingBatchID  string
}

// StateInspector is an optional administrative capability, independent of
// StateStore and Engine. It does not prune data or alter replay semantics.
type StateInspector interface {
	StateStats(context.Context) (StateStats, error)
}
