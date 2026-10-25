package core

import (
	"context"
	"github.com/lengzhao/vm/contractapi"
)

const (
	MaxObjectKeyBytes             = contractapi.MaxObjectKeyBytes
	MaxObjectValueBytes           = contractapi.MaxObjectValueBytes
	MaxObjectCalls                = contractapi.MaxObjectCalls
	CodeStateFailure    ErrorCode = "STATE_FAILURE"
	CodeStateConflict   ErrorCode = "STATE_CONFLICT"
)

// ObjectStore is a scoped view of one contract's default object. It exposes no
// address selection, transaction completion or snapshot administration.
type ObjectStore interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte) error
	Delete(context.Context, string) error
}
type objectContextKey struct{}

func WithObjectStore(ctx context.Context, store ObjectStore) context.Context {
	return context.WithValue(ctx, objectContextKey{}, store)
}
func ObjectStoreFromContext(ctx context.Context) (ObjectStore, bool) {
	s, ok := ctx.Value(objectContextKey{}).(ObjectStore)
	return s, ok && s != nil
}
