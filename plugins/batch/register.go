// Package batch registers optional deterministic ordered scheduling.
package batch

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/batch"
)

type Config struct {
	Workers  int `json:"workers"`
	MaxBatch int `json:"maxBatch"`
}
type Deps struct {
	State    core.SnapshotStore       `json:"state"`
	Executor core.TransactionExecutor `json:"executor"`
}

func New(c Config, d Deps) (core.BatchExecutor, error) {
	return impl.New(impl.Options{Workers: c.Workers, MaxBatch: c.MaxBatch}, impl.Deps{State: d.State, Executor: d.Executor})
}
func init() { pluginkit.Register("batch/ordered", New) }
