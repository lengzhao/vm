package core

import "context"

// SnapshotStore begins a transaction that can validate its observed keys without
// publishing writes. It is independent of the minimal StateStore capability.
type SnapshotStore interface {
	BeginSnapshot(context.Context) (SnapshotTransaction, error)
}

// SnapshotTransaction validates all read and written keys, including absent keys,
// against changes since BeginSnapshot. Validate neither closes nor publishes the
// transaction. Commit atomically validates and publishes, even for read-only calls.
// Failed validation leaves the transaction available for Rollback.
type SnapshotTransaction interface {
	StateTransaction
	Validate(context.Context) error
}
