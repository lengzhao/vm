// Package state registers independent state transaction backends.
package state

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/state"
)

type Config struct{}

func New(Config) (core.StateStore, error) { return impl.New(), nil }
func init()                               { pluginkit.Register("state/memory", New) }
