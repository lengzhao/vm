package wasm

import (
	"context"
	"errors"
	"sync"

	"github.com/lengzhao/vm/internal/execution"
	"github.com/tetratelabs/wazero"
)

type compiledCode struct {
	digest string
	cache  wazero.CompilationCache
	gate   chan struct{}
	refs   int
}

// Keep one resident artifact. Retired generations live only as long as their
// in-flight guests; they are never closed while executable code is in use.
type compilationCaches struct {
	mu       sync.Mutex
	current  *compiledCode
	entries  map[*compiledCode]struct{}
	changed  chan struct{}
	stopping bool
	closeErr error
}

func (r *Runner) acquireCache(ctx context.Context, digest string) (*compiledCode, func() error, error) {
	c := &r.code
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := execution.Interruption(ctx); err != nil {
		return nil, nil, err
	}
	if c.stopping {
		return nil, nil, failure("WASI runner stopped", nil)
	}
	if c.current == nil || c.current.digest != digest {
		if old := c.current; old != nil && old.refs == 0 {
			if err := c.close(old); err != nil {
				return nil, nil, failure("close WASI compilation cache", err)
			}
		}
		c.current = &compiledCode{digest: digest, cache: wazero.NewCompilationCache(), gate: make(chan struct{}, 1)}
		if c.entries == nil {
			c.entries = make(map[*compiledCode]struct{})
		}
		c.entries[c.current] = struct{}{}
	}
	if c.changed == nil {
		c.changed = make(chan struct{})
	}
	entry := c.current
	entry.refs++
	return entry, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		entry.refs--
		var err error
		if entry.refs == 0 && (c.stopping || c.current != entry) {
			err = c.close(entry)
		}
		close(c.changed)
		c.changed = make(chan struct{})
		return err
	}, nil
}

// close runs under mu, only after the last runtime using this entry has closed.
func (c *compilationCaches) close(entry *compiledCode) error {
	err := entry.cache.Close(context.Background())
	c.closeErr = errors.Join(c.closeErr, err)
	delete(c.entries, entry)
	if c.current == entry {
		c.current = nil
	}
	return err
}

func (r *Runner) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c := &r.code
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopping = true
	for entry := range c.entries {
		if entry.refs == 0 {
			c.close(entry)
		}
	}
	for len(c.entries) > 0 {
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
		c.mu.Lock()
		if err := ctx.Err(); err != nil {
			return errors.Join(c.closeErr, err)
		}
	}
	return c.closeErr
}

func (c *compiledCode) compile(ctx context.Context, runtime wazero.Runtime, data []byte) (wazero.CompiledModule, error) {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.gate }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return runtime.CompileModule(ctx, data)
}
