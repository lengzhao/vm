package batch

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/batch"
)

type RecoverableConfig struct {
	Workers     int    `json:"workers"`
	MaxBatch    int    `json:"maxBatch"`
	ExecutionID string `json:"executionID"`
}
type RecoverableDeps struct {
	State     core.RecoverableBatchStore `json:"state"`
	Executor  core.TransactionExecutor   `json:"executor"`
	Contracts core.ContractReader        `json:"contracts"`
}

func NewRecoverable(c RecoverableConfig, d RecoverableDeps) (core.RecoverableBatchExecutor, error) {
	return impl.NewRecoverable(impl.RecoverableOptions{Workers: c.Workers, MaxBatch: c.MaxBatch, ExecutionID: c.ExecutionID}, impl.RecoverableDeps{State: d.State, Executor: d.Executor, Contracts: d.Contracts})
}
func init() { pluginkit.Register("batch/recoverable", NewRecoverable) }
