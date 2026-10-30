package engine

import (
	"fmt"
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/engine"
)

type ObjectsDeps struct {
	Components *Deps              `json:"components"`
	Objects    core.ObjectService `json:"objects"`
}

func NewObjects(c Config, d ObjectsDeps) (*impl.Engine, error) {
	if d.Components == nil || d.Objects == nil {
		return nil, fmt.Errorf("engine components and objects required")
	}
	base, e := New(c, *d.Components)
	if e != nil {
		return nil, e
	}
	engine := base.(*impl.Engine)
	return engine.WithObjects(d.Objects), nil
}
func init() { pluginkit.Register("engine/objects", NewObjects) }
