// Package gas registers instrumentation, profiling and per-call meters.
package gas

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/gas"
)

type Config struct{}

func NewInjector(Config) (core.GasInjector, error)   { return &impl.Injector{}, nil }
func NewProfiler(Config) (core.GasProfiler, error)   { return &impl.Profiler{}, nil }
func NewMeters(Config) (core.GasMeterFactory, error) { return &impl.Factory{}, nil }
func init() {
	pluginkit.Register("gas/injector", NewInjector)
	pluginkit.Register("gas/profiler", NewProfiler)
	pluginkit.Register("gas/meters", NewMeters)
}
