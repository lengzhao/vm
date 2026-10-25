package gas

import "github.com/lengzhao/vm/core"

type Factory struct{}

func (*Factory) NewMeter() core.GasMetering { return NewMeter() }

type Profiler struct{}

func (*Profiler) BuildProfile(s string) (*core.GasProfile, error) { return BuildGasProfile(s) }
func (*Profiler) Estimate(p *core.GasProfile, f string, b uint64) (uint64, map[string]uint64, error) {
	return EstimateFromProfile(p, f, b)
}
