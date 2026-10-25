package core

import (
	"context"
	"errors"
)

var (
	ErrStateClosed   = errors.New("state transaction closed")
	ErrStateConflict = errors.New("state transaction conflicts with committed state")
)

// StateKey isolates opaque keys by contract. Empty keys and addresses are valid
// storage keys; authorization and address validation belong to the caller.
type StateKey struct {
	Contract Address
	Key      string
}

// StateWrite describes the final operation on a key. Deleted distinguishes a
// deletion from storing an empty value.
type StateWrite struct {
	Key     StateKey
	Value   []byte
	Deleted bool
}

// StateStore begins a consistent snapshot. The Begin context bounds the entire
// transaction; each operation may impose a shorter context of its own.
type StateStore interface {
	Begin(context.Context) (StateTransaction, error)
}

// StateTransaction provides isolated reads and local writes until atomic Commit.
// Byte slices and access sets are caller-owned copies. Sets are unique and sorted
// by contract then key, and remain inspectable after completion. ReadSet includes
// missing keys and reads of local writes; WriteSet contains final operations.
// Failed commits publish nothing and leave the transaction open for Rollback.
// Successful commits close the transaction. Rollback is context-free and
// idempotent, including after commit. Implementations support concurrent methods.
type StateTransaction interface {
	Get(context.Context, StateKey) ([]byte, bool, error)
	Set(context.Context, StateKey, []byte) error
	Delete(context.Context, StateKey) error
	ReadSet() []StateKey
	WriteSet() []StateWrite
	Commit(context.Context) error
	Rollback() error
}
