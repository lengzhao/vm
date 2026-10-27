package state

import (
	"fmt"
	"testing"

	"github.com/lengzhao/vm/core"
)

func TestManyKeySnapshotIsolation(t *testing.T) {
	for _, f := range []struct {
		name string
		new  func() core.StateStore
	}{
		{"memory", func() core.StateStore { return New() }},
		{"mvcc", func() core.StateStore { return NewMVCC() }},
	} {
		t.Run(f.name, func(t *testing.T) {
			s := f.new()
			tx := begin(t, s)
			for i := range 2048 {
				set(t, tx, core.StateKey{Contract: "contract", Key: fmt.Sprint(i)}, []byte("old"))
			}
			commit(t, tx)
			old := begin(t, s)
			tx = begin(t, s)
			for i := range 2048 {
				k := core.StateKey{Contract: "contract", Key: fmt.Sprint(i)}
				if i%2 == 0 {
					if err := tx.Delete(bg, k); err != nil {
						t.Fatal(err)
					}
				} else {
					set(t, tx, k, []byte("new"))
				}
			}
			commit(t, tx)
			current := begin(t, s)
			for i := range 2048 {
				k := core.StateKey{Contract: "contract", Key: fmt.Sprint(i)}
				get(t, old, k, "old", true)
				if i%2 == 0 {
					get(t, current, k, "", false)
				} else {
					get(t, current, k, "new", true)
				}
			}
		})
	}
}
