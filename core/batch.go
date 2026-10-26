package core

import "context"

// BatchItemResult is one final execution in input order. Err is a transaction's
// execution error; speculative attempts and their Gas/Events are never returned.
type BatchItemResult struct {
	Result *ExecuteResult
	Err    error
}

// BatchExecutor commits each successful item in input order. An outer error
// stops the batch and returns only the finalized prefix; that prefix remains
// committed. This is not a batch-wide atomic transaction. Deterministic equivalence
// requires deterministic contracts, immutable inputs/artifacts and no external
// state writers during the batch. Resource and deadline failures are not consensus.
// ErrStateCommitUncertain means the current item outside the returned prefix may
// already be durable. Reopen and reconcile before retrying; prefix length alone
// cannot establish that the current item was rolled back.
type BatchExecutor interface {
	ExecuteBatch(context.Context, []ExecutionRequest) ([]BatchItemResult, error)
}
