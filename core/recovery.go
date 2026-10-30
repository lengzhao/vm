package core

import (
	"context"
	"errors"
)

var ErrBatchPending = errors.New("unfinished batch must be resumed before other writes")

var ErrBatchIdentityConflict = errors.New("batch ID already belongs to different content or execution rules")

type BatchIdentity struct {
	ID          string
	Fingerprint string
	Count       int
}
type BatchFailure struct {
	Code    ErrorCode
	Message string
}
type BatchReceipt struct {
	Index   int
	Result  *ExecuteResult
	Failure *BatchFailure
}

// RecoverableBatchStore serializes all normal transactions and batch sessions
// through the same backend. Acquisition waits for prior transactions to finish.
type RecoverableBatchStore interface {
	AcquireBatch(context.Context, BatchIdentity) (BatchSession, error)
}

// BatchSession owns an exclusive writer scope. Finalize validates its own snapshot
// and atomically records the next consecutive receipt together with successful
// writes. A failure receipt discards guest writes. Plain Commit on a session
// transaction is forbidden. Close cancels and discards remaining snapshots.
// ErrStateCommitUncertain requires closing/reopening the backend and reacquiring
// the same identity to inspect Prefix before retrying.
type BatchSession interface {
	SnapshotStore
	Prefix(context.Context) ([]BatchReceipt, error)
	Finalize(context.Context, SnapshotTransaction, BatchReceipt) error
	Close() error
}

type RecoverableBatchRequest struct {
	ID    string
	Calls []ExecutionRequest
}
type RecoverableBatchExecutor interface {
	ExecuteRecoverableBatch(context.Context, RecoverableBatchRequest) ([]BatchItemResult, error)
}
