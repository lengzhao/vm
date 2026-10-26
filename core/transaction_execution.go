package core

import "context"

// TransactionExecutor executes one complete call tree in a caller-owned
// transaction. It never commits or rolls it back. The caller must discard writes
// after an execution error. Failed results carry no Data or Events.
// This is a trusted scheduling capability, not a contract-visible API.
type TransactionExecutor interface {
	ExecuteTransaction(context.Context, ExecutionRequest, StateTransaction) (*ExecuteResult, error)
}
