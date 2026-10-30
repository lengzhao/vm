package engine

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
)

// Batched composes independent application capabilities without widening Engine.
// The batch node's executor and this facade's engine must refer to the same engine
// instance; the preset wires both explicitly through PluginKit.
type Batched struct {
	core.Engine
	core.BatchExecutor
}
type BatchedDeps struct {
	Engine core.Engine        `json:"engine"`
	Batch  core.BatchExecutor `json:"batch"`
}

func NewBatched(_ struct{}, d BatchedDeps) (*Batched, error) {
	if d.Engine == nil || d.Batch == nil {
		return nil, fmt.Errorf("engine and batch capabilities required")
	}
	return &Batched{Engine: d.Engine, BatchExecutor: d.Batch}, nil
}
func init() { pluginkit.Register("engine/batched", NewBatched) }
