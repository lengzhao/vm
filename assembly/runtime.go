package assembly

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lengzhao/pluginkit/build"
	"github.com/lengzhao/vm/core"
	engineplugin "github.com/lengzhao/vm/plugins/engine"
)

// Constructors must not acquire live resources. Start acquires them only after
// the complete graph has been constructed and type checked by PluginKit.
type Starter interface{ Start(context.Context) error }
type Stopper interface{ Stop(context.Context) error }

type Runtime struct {
	Engine   core.Engine
	Result   *build.Result
	mu       sync.Mutex
	stopped  bool
	stopErr  error
	shutdown []build.Instance
}

func Build(ctx context.Context, cfg Config) (*Runtime, error) {
	if cfg.Root == "" || len(cfg.Plugins) == 0 {
		return nil, fmt.Errorf("root and plugins required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	engine, result, err := build.Build[core.Engine](ctx, cfg.Plugins, cfg.Root)
	if err != nil {
		return nil, err
	}
	return start(ctx, engine, result)
}

// BuildDefaultWithRunner preserves programmatic Runner injection without a
// global registry of mutable instances. All other dependencies use PluginKit.
func BuildDefaultWithRunner(ctx context.Context, c core.VMConfig, runner core.Runner) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := DefaultConfig(c)
	// This helper serves the legacy facade, whose development mode accepts
	// artifacts from other development source policies.
	if !c.RequireDeterminism {
		cfg.Plugins["validator"].(map[string]any)["config"].(map[string]any)["allowAnyPolicy"] = true
	}
	if runner == nil {
		return Build(ctx, cfg)
	}
	root := cfg.Plugins["engine"].(map[string]any)
	cfg.Plugins["components"] = map[string]any{"use": "engine/components", "deps": root["deps"]}
	deps, result, err := build.Build[*engineplugin.Deps](ctx, cfg.Plugins, "components")
	if err != nil {
		return nil, err
	}
	deps.Runner = runner
	e, err := engineplugin.NewWithConfig(c, *deps)
	if err != nil {
		return nil, err
	}
	// The legacy runner replaces the process backend in lifecycle management too.
	for i := range result.Instances {
		if result.Instances[i].ID == "runner" {
			result.Instances[i].Value = runner
			result.Instances[i].Use = "programmatic/runner"
		}
	}
	result.Instances = append(result.Instances, build.Instance{ID: "engine", Use: "engine/default", Value: e})
	return start(ctx, e, result)
}

func start(ctx context.Context, e core.Engine, result *build.Result) (*Runtime, error) {
	rt := &Runtime{Engine: e, Result: result}
	for _, instance := range result.Instances {
		err := ctx.Err()
		if err == nil {
			if s, ok := instance.Value.(Starter); ok {
				// Include the attempted instance: Start may acquire resources before failing.
				rt.shutdown = append(rt.shutdown, instance)
				err = s.Start(ctx)
			}
		}
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			stopErr := rt.Stop(cleanup)
			cancel()
			return nil, errors.Join(fmt.Errorf("start plugin %q: %w", instance.ID, err), stopErr)
		}
	}
	// After successful startup, Stop-only plugins may lazily acquire resources.
	rt.shutdown = append([]build.Instance(nil), result.Instances...)
	return rt, nil
}

// Stop releases resources in reverse dependency order. Cancellation retains
// pending plugins for retry; successfully stopped plugins are never called twice.
func (r *Runtime) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return r.stopErr
	}
	for len(r.shutdown) > 0 {
		// Keep unattempted plugins queued when this shutdown's budget runs out.
		if err := ctx.Err(); err != nil {
			return errors.Join(r.stopErr, err)
		}
		i := len(r.shutdown) - 1
		instance := r.shutdown[i]
		var err error
		if stopper, ok := instance.Value.(Stopper); ok {
			err = stopper.Stop(ctx)
		}
		if err != nil {
			wrapped := fmt.Errorf("stop plugin %q: %w", instance.ID, err)
			// Cancellation is an incomplete shutdown attempt, not a permanent
			// plugin failure. Retain this plugin and its dependencies for retry.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return errors.Join(r.stopErr, wrapped)
			}
			r.stopErr = errors.Join(r.stopErr, wrapped)
		}
		r.shutdown = r.shutdown[:i]
	}
	r.stopped = true
	return r.stopErr
}
