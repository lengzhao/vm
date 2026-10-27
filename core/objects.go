package core

import "context"

// CodeObjectFailure identifies an object business rejection, not a storage fault.
const CodeObjectFailure ErrorCode = "OBJECT_FAILURE"

// ObjectScope is bound by the host; the embedding application authenticates root Sender.
type ObjectScope struct{ Contract, Sender Address }
type ObjectCommand struct {
	ID, Op, Key string
	Value       []byte
	Owner       Address
}
type ObjectReply struct {
	Exists bool
	Value  []byte
	Owner  Address
}

// ObjectAuthorizer supplies one coherent access policy. Implementations are trusted.
type ObjectAuthorizer interface {
	Authorize(context.Context, ObjectScope, Address, string) error
}

// ObjectService binds an execution frame to its existing transaction and identity.
type ObjectService interface {
	Bind(StateTransaction, ObjectScope) ObjectSession
}

// ObjectSession errors are latched: a caller cannot ignore failed operations.
// Close revokes the frame capability; it does not complete the transaction.
type ObjectSession interface {
	Apply(context.Context, ObjectCommand) (ObjectReply, error)
	Err() error
	Close()
}
type objectsContextKey struct{}

func WithObjects(ctx context.Context, s ObjectSession) context.Context {
	return context.WithValue(ctx, objectsContextKey{}, s)
}
func ObjectsFromContext(ctx context.Context) (ObjectSession, bool) {
	s, ok := ctx.Value(objectsContextKey{}).(ObjectSession)
	return s, ok && s != nil
}
