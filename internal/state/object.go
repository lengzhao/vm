package state

import (
	"context"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/lengzhao/vm/core"
)

// Object binds the storage namespace at a trusted host boundary. Errors are
// latched so a custom Runner cannot ignore failed writes and commit partial work.
type Object struct {
	tx      core.StateTransaction
	address core.Address
	mu      sync.Mutex
	err     error
}

func Bind(tx core.StateTransaction, address core.Address) *Object {
	return &Object{tx: tx, address: address}
}
func (o *Object) Err() error { o.mu.Lock(); defer o.mu.Unlock(); return o.err }
func (o *Object) record(err error) error {
	if o.err == nil {
		o.err = err
	}
	return err
}
func (o *Object) check(ctx context.Context, key string) error {
	if o.err != nil {
		return o.err
	}
	if err := ctx.Err(); err != nil {
		return o.record(err)
	}
	if !utf8.ValidString(key) || len(key) > core.MaxObjectKeyBytes {
		return o.record(fmt.Errorf("invalid object field name"))
	}
	return nil
}
func (o *Object) Get(ctx context.Context, key string) ([]byte, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.check(ctx, key); err != nil {
		return nil, false, err
	}
	value, ok, err := o.tx.Get(ctx, core.StateKey{Contract: o.address, Key: key})
	if err == nil && len(value) > core.MaxObjectValueBytes {
		err = fmt.Errorf("object value exceeds 64 KiB")
	}
	if err != nil {
		return nil, false, o.record(err)
	}
	return value, ok, nil
}
func (o *Object) Set(ctx context.Context, key string, value []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.check(ctx, key); err != nil {
		return err
	}
	if len(value) > core.MaxObjectValueBytes {
		return o.record(fmt.Errorf("object value exceeds 64 KiB"))
	}
	return o.record(o.tx.Set(ctx, core.StateKey{Contract: o.address, Key: key}, value))
}
func (o *Object) Delete(ctx context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.check(ctx, key); err != nil {
		return err
	}
	return o.record(o.tx.Delete(ctx, core.StateKey{Contract: o.address, Key: key}))
}
