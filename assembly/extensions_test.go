package assembly_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	abiplugin "github.com/lengzhao/vm/plugins/abi"
	gasplugin "github.com/lengzhao/vm/plugins/gas"
)

var rejected = errors.New("test capability rejected input")

type rejectingABI struct{}

func (*rejectingABI) Generate(string) (*abi.ABI, error) { return nil, rejected }

type rejectingPolicy struct{}

func (*rejectingPolicy) Review(string) error { return rejected }
func (*rejectingPolicy) ID() string          { return "test-policy-v1" }

type rejectingProfiler struct{}

func (*rejectingProfiler) BuildProfile(string) (*core.GasProfile, error) { return nil, rejected }
func (*rejectingProfiler) Estimate(*core.GasProfile, string, uint64) (uint64, map[string]uint64, error) {
	return 0, nil, rejected
}

type incrementInjector struct{}

func (*incrementInjector) InjectGas(s string) (string, error) {
	p, _ := gasplugin.NewInjector(gasplugin.Config{})
	return p.InjectGas(strings.ReplaceAll(s, "return 1", "return 2"))
}

type trackingMeters struct {
	mu     sync.Mutex
	meters []core.GasMetering
}

func (f *trackingMeters) NewMeter() core.GasMetering {
	factory, _ := gasplugin.NewMeters(gasplugin.Config{})
	m := factory.NewMeter()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.meters = append(f.meters, m)
	return m
}

type cycleDeps struct {
	Runner core.Runner `json:"runner"`
}

func init() {
	pluginkit.Register("test/abi-reject", func(testConfig) (core.ABIGenerator, error) { return &rejectingABI{}, nil })
	pluginkit.Register("test/policy-reject", func(testConfig) (core.SourcePolicy, error) { return &rejectingPolicy{}, nil })
	pluginkit.Register("test/profile-reject", func(testConfig) (core.GasProfiler, error) { return &rejectingProfiler{}, nil })
	pluginkit.Register("test/increment", func(testConfig) (core.GasInjector, error) { return &incrementInjector{}, nil })
	pluginkit.Register("test/tracking-meters", func(testConfig) (*trackingMeters, error) { return &trackingMeters{}, nil })
	pluginkit.Register("test/cycle", func(_ testConfig, d cycleDeps) (core.Runner, error) { return d.Runner, nil })
}

func TestCompilerUsesInjectedCapabilities(t *testing.T) {
	for _, tt := range []struct{ id, use string }{{"abi", "test/abi-reject"}, {"policy", "test/policy-reject"}, {"profiler", "test/profile-reject"}} {
		t.Run(tt.id, func(t *testing.T) {
			cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
			cfg.Plugins[tt.id] = map[string]any{"use": tt.use}
			rt, err := assembly.Build(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Stop(context.Background())
			if _, err := rt.Engine.Compile(context.Background(), "package main\nfunc Answer() int{return 1}"); !errors.Is(err, rejected) {
				t.Fatalf("injected %s not used: %v", tt.id, err)
			}
			if tt.id != "profiler" {
				if _, err := rt.Engine.GenerateABI(context.Background(), "package main\nfunc Answer() int{return 1}"); !errors.Is(err, rejected) {
					t.Fatalf("GenerateABI bypassed %s: %v", tt.id, err)
				}
			}
		})
	}
}

func TestInjectorChangesArtifactAndCache(t *testing.T) {
	source := "package main\nfunc Answer() int {return 1}"
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	original, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Stop(context.Background())
	first, err := original.Engine.Compile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Plugins["injector"] = map[string]any{"use": "test/increment"}
	modified, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer modified.Stop(context.Background())
	second, err := modified.Engine.Compile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceHash == second.SourceHash || first.ExecutablePath == second.ExecutablePath {
		t.Fatal("different instrumentation reused cached artifact")
	}
	address, err := modified.Engine.Deploy(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	data, err := modified.Engine.Execute(context.Background(), core.ExecutionRequest{Address: address, Function: "Answer"})
	if err != nil || string(data.Data) != "2" {
		t.Fatalf("%s %v", data.Data, err)
	}
}

func TestInjectedMetersAreIndependent(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableGasMetering: true, MaxGasLimit: 100})
	cfg.Plugins["meters"] = map[string]any{"use": "test/tracking-meters"}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Answer() int{return 1}")
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Answer"}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	f, ok := build.GetByID[*trackingMeters](rt.Result, "meters")
	if !ok {
		t.Fatal("meter plugin missing")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.meters) != 4 {
		t.Fatalf("expected four independent meters, got %d", len(f.meters))
	}
	seen := map[core.GasMetering]bool{}
	for _, m := range f.meters {
		if seen[m] {
			t.Fatal("meter reused")
		}
		seen[m] = true
	}
}

func TestCycleIsRejected(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["runner"] = map[string]any{"use": "test/cycle", "deps": map[string]any{"runner": "runner"}}
	if _, err := assembly.Build(context.Background(), cfg); err == nil || !strings.Contains(strings.ToLower(err.Error()), "cycl") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

type lifecycleABI struct{ events []string }

func (p *lifecycleABI) Generate(s string) (*abi.ABI, error) {
	a, _ := abiplugin.New(abiplugin.Config{})
	return a.Generate(s)
}
func (p *lifecycleABI) Start(context.Context) error {
	p.events = append(p.events, "abi.start")
	return nil
}
func (p *lifecycleABI) Stop(context.Context) error {
	p.events = append(p.events, "abi.stop")
	return nil
}

type lifecycleConfig struct {
	Fail bool `json:"fail"`
}
type lifecycleDeps struct {
	ABI *lifecycleABI `json:"abi"`
}
type lifecycleRunner struct {
	abi  *lifecycleABI
	fail bool
}

func (r *lifecycleRunner) Run(context.Context, *core.CompiledContract, core.CallRequest) (*core.CallResult, error) {
	return &core.CallResult{}, nil
}
func (r *lifecycleRunner) Start(context.Context) error {
	r.abi.events = append(r.abi.events, "runner.start")
	if r.fail {
		return rejected
	}
	return nil
}
func (r *lifecycleRunner) Stop(context.Context) error {
	r.abi.events = append(r.abi.events, "runner.stop")
	return nil
}
func init() {
	pluginkit.Register("test/lifecycle-abi", func(testConfig) (*lifecycleABI, error) { return &lifecycleABI{}, nil })
	pluginkit.Register("test/lifecycle-runner", func(c lifecycleConfig, d lifecycleDeps) (core.Runner, error) {
		return &lifecycleRunner{abi: d.ABI, fail: c.Fail}, nil
	})
}
func TestLifecycleDependencyOrderAndIdempotence(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["abi"] = map[string]any{"use": "test/lifecycle-abi"}
	cfg.Plugins["runner"] = map[string]any{"use": "test/lifecycle-runner", "deps": map[string]any{"abi": "abi"}}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := build.GetByID[*lifecycleABI](rt.Result, "abi")
	if got := strings.Join(p.events, ","); got != "abi.start,runner.start" {
		t.Fatal(got)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.events, ","); got != "abi.start,runner.start,runner.stop,abi.stop" {
		t.Fatal(got)
	}
}

// A failed Start must roll back using a live cleanup context.
func TestFailedStartIsRolledBack(t *testing.T) {
	events := []string{}
	rt := newFailureRuntime(t, &events)
	if rt != nil {
		t.Fatal("failed startup returned runtime")
	}
	if got := strings.Join(events, ","); got != "start,stop" {
		t.Fatal(got)
	}
}

var failureProbeMu sync.Mutex
var failureProbe *[]string

// This constructor only links a test recorder; production plugins acquire resources in Start.
type failureRunner struct{ events *[]string }

func (r *failureRunner) Run(context.Context, *core.CompiledContract, core.CallRequest) (*core.CallResult, error) {
	return nil, nil
}
func (r *failureRunner) Start(context.Context) error {
	*r.events = append(*r.events, "start")
	return rejected
}
func (r *failureRunner) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	*r.events = append(*r.events, "stop")
	return nil
}
func init() {
	pluginkit.Register("test/fail-start", func(testConfig) (core.Runner, error) { return &failureRunner{events: failureProbe}, nil })
}
func newFailureRuntime(t *testing.T, events *[]string) *assembly.Runtime {
	t.Helper()
	failureProbeMu.Lock()
	defer failureProbeMu.Unlock()
	failureProbe = events
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["runner"] = map[string]any{"use": "test/fail-start"}
	rt, err := assembly.Build(context.Background(), cfg)
	if !errors.Is(err, rejected) {
		t.Fatalf("%v", err)
	}
	return rt
}

type cancelEngine struct {
	core.Engine
	cancel context.CancelFunc
}

func (e *cancelEngine) Start(context.Context) error { e.cancel(); return nil }

var startCancel context.CancelFunc

func init() {
	pluginkit.Register("test/cancel-engine", func(_ testConfig, d struct {
		Engine core.Engine `json:"engine"`
	}) (core.Engine, error) {
		return &cancelEngine{Engine: d.Engine, cancel: startCancel}, nil
	})
}
func TestCancellationDuringFinalStartRollsBack(t *testing.T) {
	failureProbeMu.Lock()
	defer failureProbeMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startCancel = cancel
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["cancel"] = map[string]any{"use": "test/cancel-engine", "deps": map[string]any{"engine": "engine"}}
	cfg.Root = "cancel"
	rt, err := assembly.Build(ctx, cfg)
	if !errors.Is(err, context.Canceled) {
		if rt != nil {
			rt.Stop(context.Background())
		}
		t.Fatalf("cancellation ignored: %v", err)
	}
}

type lifecycleEngine struct {
	core.Engine
	probe *lifecycleABI
}

func (e *lifecycleEngine) Start(context.Context) error {
	e.probe.events = append(e.probe.events, "engine.start")
	return nil
}
func (e *lifecycleEngine) Stop(context.Context) error {
	e.probe.events = append(e.probe.events, "engine.stop")
	return nil
}
func init() {
	pluginkit.Register("test/lifecycle-engine", func(_ testConfig, d struct {
		Engine core.Engine   `json:"engine"`
		ABI    *lifecycleABI `json:"abi"`
	}) (core.Engine, error) {
		return &lifecycleEngine{Engine: d.Engine, probe: d.ABI}, nil
	})
}
func TestCustomEngineStopIsCalled(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir()})
	cfg.Plugins["abi"] = map[string]any{"use": "test/lifecycle-abi"}
	cfg.Plugins["custom"] = map[string]any{"use": "test/lifecycle-engine", "deps": map[string]any{"engine": "engine", "abi": "abi"}}
	cfg.Root = "custom"
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := build.GetByID[*lifecycleABI](rt.Result, "abi")
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(probe.events, ","); got != "abi.start,engine.start,engine.stop,abi.stop" {
		t.Fatal(got)
	}
}

type readyMeters struct {
	trackingMeters
	started bool
}

func (f *readyMeters) Start(context.Context) error { f.started = true; return nil }
func (f *readyMeters) NewMeter() core.GasMetering {
	if !f.started {
		return nil
	}
	return f.trackingMeters.NewMeter()
}
func init() {
	pluginkit.Register("test/ready-meters", func(testConfig) (core.GasMeterFactory, error) { return &readyMeters{}, nil })
}
func TestMeterFactoryStartsBeforeUse(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableGasMetering: true, MaxGasLimit: 100})
	cfg.Plugins["meters"] = map[string]any{"use": "test/ready-meters"}
	rt, err := assembly.Build(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Answer()int{return 1}")
	if err != nil {
		t.Fatal(err)
	}
	a, err := rt.Engine.Deploy(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Answer"}); err != nil {
		t.Fatal(err)
	}
}
