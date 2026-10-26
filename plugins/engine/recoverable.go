package engine

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
)

type Recoverable struct {
	core.Engine
	core.RecoverableBatchExecutor
}
type RecoverableDeps struct {
	Engine core.Engine                   `json:"engine"`
	Batch  core.RecoverableBatchExecutor `json:"batch"`
}

func NewRecoverable(_ struct{}, d RecoverableDeps) (*Recoverable, error) {
	if d.Engine == nil || d.Batch == nil {
		return nil, fmt.Errorf("engine and recoverable batch capabilities required")
	}
	return &Recoverable{Engine: d.Engine, RecoverableBatchExecutor: d.Batch}, nil
}
func init() { pluginkit.Register("engine/recoverable", NewRecoverable) }
