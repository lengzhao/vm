// Package wasm implements a WASI execution backend without ambient host access.
package wasm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/execution"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type Options struct {
	Timeout     time.Duration
	MemoryMB    int
	MaxModuleMB int
	Engine      string
}
type Runner struct {
	options Options
	code    compilationCaches
}

func New(o Options) (*Runner, error) {
	if o.Timeout < 0 || o.MemoryMB < 0 || o.MemoryMB > 4096 || o.MaxModuleMB < 0 || o.MaxModuleMB > 256 {
		return nil, fmt.Errorf("invalid WASI resource limits")
	}
	if o.Engine != "" && o.Engine != "compiler" && o.Engine != "interpreter" {
		return nil, fmt.Errorf("unknown wazero engine %q", o.Engine)
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MemoryMB == 0 {
		o.MemoryMB = 256
	}
	if o.MaxModuleMB == 0 {
		o.MaxModuleMB = 64
	}
	return &Runner{options: o}, nil
}
func (r *Runner) Run(ctx context.Context, c *core.CompiledContract, req core.CallRequest) (result *core.CallResult, returnErr error) {
	runCtx, cancel := context.WithTimeout(ctx, r.options.Timeout)
	defer cancel()
	if err := execution.Interruption(runCtx); err != nil {
		return nil, err
	}
	if err := validateArtifact(runCtx, c); err != nil {
		return nil, err
	}
	if err := artifact.ValidateTargetRules(c, "wasip1", "wasm"); err != nil {
		return nil, err
	}
	payload, err := execution.EncodeRequest(runCtx, req)
	if err != nil {
		return nil, err
	}
	if err := execution.Interruption(runCtx); err != nil {
		return nil, err
	}
	f, err := os.Open(c.ExecutablePath)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, int64(r.options.MaxModuleMB)*(1<<20)+1))
	closeErr := f.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if len(data) > r.options.MaxModuleMB*(1<<20) {
		return nil, failure("Wasm module exceeds byte limit", nil)
	}
	if artifact.Digest(data) != c.Manifest.BinarySHA256 {
		return nil, &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "Wasm changed during read"}
	}
	if err := execution.Interruption(runCtx); err != nil {
		return nil, err
	}
	env := execution.Environment(ctx)
	if err := execution.Interruption(runCtx); err != nil {
		return nil, err
	}
	guestCtx, stopGuest := context.WithCancel(runCtx)
	defer stopGuest()
	// A guest writes Host frames synchronously. Replies append to its input queue
	// without a pipe/goroutine that could deadlock before the guest's next fd_read.
	input := bytes.NewBuffer(append(payload, '\n'))
	session := execution.NewSession(runCtx, env, input, stopGuest)
	config := wazero.NewRuntimeConfig()
	switch r.options.Engine {
	case "compiler":
		config = wazero.NewRuntimeConfigCompiler()
	case "interpreter":
		config = wazero.NewRuntimeConfigInterpreter()
	}
	code, release, err := r.acquireCache(guestCtx, c.Manifest.BinarySHA256)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := release(); err != nil {
			returnErr = errors.Join(returnErr, failure("close WASI compilation cache", err))
		}
	}()
	config = config.WithMemoryLimitPages(uint32(r.options.MemoryMB) * 16).WithCloseOnContextDone(true).WithCompilationCache(code.cache)
	runtime := wazero.NewRuntimeWithConfig(guestCtx, config)
	defer func() {
		if err := runtime.Close(context.Background()); err != nil {
			returnErr = errors.Join(returnErr, failure("close WASI runtime", err))
		}
	}()
	if _, err := wasi_snapshot_preview1.Instantiate(guestCtx, runtime); err != nil {
		return nil, failure("instantiate WASI host", err)
	}
	module := wazero.NewModuleConfig().WithName("").WithArgs("contract").WithStdin(input).WithStdout(session.Stdout()).WithStderr(session.Stderr())
	// No filesystem, sockets, host clocks, host random source or ambient env.
	for _, value := range env {
		key, value, ok := strings.Cut(value, "=")
		if ok {
			module = module.WithEnv(key, value)
		}
	}
	compiled, runErr := code.compile(guestCtx, runtime, data)
	if runErr == nil {
		// InstantiateWithConfig marks compiled code for deletion at guest close.
		// This explicit path leaves its lifetime with the leased cache instead.
		_, runErr = runtime.InstantiateModule(guestCtx, compiled, module)
	}
	var exit *sys.ExitError
	if errors.As(runErr, &exit) && exit.ExitCode() == 0 {
		runErr = nil
	}
	return session.Result(runErr)
}
func failure(message string, cause error) error {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &core.ExecutionError{Code: core.CodeProcessFailure, Message: message, Cause: cause}
}

// validateArtifact preserves the protocol's infrastructure cancellation category.
func validateArtifact(ctx context.Context, c *core.CompiledContract) error {
	if err := artifact.Validate(ctx, c); err != nil {
		if interrupted := execution.Interruption(ctx); interrupted != nil {
			return interrupted
		}
		return err
	}
	return nil
}
