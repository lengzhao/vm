package state

import (
	"context"
	"fmt"
	"testing"

	"github.com/lengzhao/vm/core"
)

func benchmarkSingleKeyCommit(b *testing.B, factory func() core.StateStore, size int) {
	s := factory()
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		b.Fatal(err)
	}
	for i := range size {
		if err := tx.Set(ctx, core.StateKey{Contract: "bench", Key: fmt.Sprint(i)}, []byte("value")); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tx, err := s.Begin(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if err := tx.Set(ctx, core.StateKey{Contract: "bench", Key: "0"}, []byte("value")); err != nil {
			b.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSingleKeyCommit(b *testing.B) {
	for _, f := range []struct {
		name string
		new  func() core.StateStore
	}{
		{"memory", func() core.StateStore { return New() }},
		{"mvcc", func() core.StateStore { return NewMVCC() }},
	} {
		for _, n := range []int{1000, 10000, 100000} {
			b.Run(fmt.Sprintf("%s/keys=%d", f.name, n), func(b *testing.B) { benchmarkSingleKeyCommit(b, f.new, n) })
		}
	}
}

// Keep small writes from allocating a full copy of a large database. This is an
// allocation budget, not a machine-dependent latency assertion.
func TestSingleKeyCommitAllocationBudget(t *testing.T) {
	for _, f := range []struct {
		name string
		new  func() core.StateStore
	}{
		{"memory", func() core.StateStore { return New() }},
		{"mvcc", func() core.StateStore { return NewMVCC() }},
	} {
		t.Run(f.name, func(t *testing.T) {
			result := testing.Benchmark(func(b *testing.B) { benchmarkSingleKeyCommit(b, f.new, 100000) })
			if result.AllocedBytesPerOp() > 1<<20 {
				t.Fatalf("single-key commit allocated %d bytes; budget is 1 MiB", result.AllocedBytesPerOp())
			}
		})
	}
}
