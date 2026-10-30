package process

import (
	"context"
	"errors"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/execution"
	"github.com/lengzhao/vm/internal/sandbox"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ProcessRunner 通过子进程执行合约二进制
type ProcessRunner struct {
	timeout time.Duration
	docker  *sandbox.Docker
}

// New 创建进程 core.Runner
func New(timeout time.Duration) core.Runner {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ProcessRunner{timeout: timeout}
}

// NewDocker reuses the exact protocol and host boundary with Docker isolation.
func NewDocker(timeout time.Duration, d *sandbox.Docker) core.Runner {
	r := New(timeout).(*ProcessRunner)
	r.docker = d
	return r
}

// Run 执行合约函数
func (r *ProcessRunner) Run(ctx context.Context, contract *core.CompiledContract, req core.CallRequest) (returned *core.CallResult, returnErr error) {
	if err := interruption(ctx); err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, executionError(core.CodeInvalidRequest, "contract cannot be nil", nil)
	}
	if contract.ExecutablePath == "" {
		return nil, executionError(core.CodeInvalidRequest, "executable path cannot be empty", nil)
	}
	if contract.ProtocolVersion != core.ProtocolVersion {
		return nil, executionError(core.CodeUnsupportedVersion, "unsupported artifact protocol; recompile and deploy", nil)
	}
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if err := artifact.Validate(runCtx, contract); err != nil {
		if interrupted := interruption(runCtx); interrupted != nil {
			return nil, interrupted
		}
		return nil, err
	}
	targetOS := runtime.GOOS
	if r.docker != nil {
		targetOS = "linux"
	}
	if err := artifact.ValidateTargetRules(contract, targetOS, runtime.GOARCH); err != nil {
		return nil, err
	}

	payload, err := execution.EncodeRequest(runCtx, req)
	if err != nil {
		return nil, err
	}
	outputCtx, cancelOutput := context.WithCancel(runCtx)
	defer cancelOutput()
	if err := interruption(runCtx); err != nil {
		return nil, err
	}
	env := buildContractEnv(ctx)
	cmd := exec.CommandContext(outputCtx, contract.ExecutablePath)
	if r.docker != nil {
		// Snapshot first, then revalidate: mutable deployment files are never mounted.
		dir, err := os.MkdirTemp("", "vm-sandbox-exec-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "artifact")
		if err := snapshotExecutable(runCtx, contract.ExecutablePath, path); err != nil {
			return nil, err
		}
		snapshot := *contract
		snapshot.ExecutablePath = path
		if err := artifact.Validate(runCtx, &snapshot); err != nil {
			return nil, err
		}
		var clean func() error
		var containerEnv []string
		for _, v := range env {
			if strings.HasPrefix(v, "VM_") {
				containerEnv = append(containerEnv, v)
			}
		}
		cmd, clean, err = r.docker.Prepare(outputCtx, "/artifact", containerEnv, []sandbox.Mount{{Source: path, Target: "/artifact", ReadOnly: true}})
		if err != nil {
			if interrupted := interruption(runCtx); interrupted != nil {
				return nil, errors.Join(interrupted, err)
			}
			return nil, executionError(core.CodeProcessFailure, "prepare execution sandbox: "+err.Error(), err)
		}
		defer func() {
			if err := clean(); err != nil {
				returnErr = errors.Join(returnErr, executionError(core.CodeProcessFailure, "cleanup execution sandbox: "+err.Error(), err))
			}
		}()
	} else {
		cmd.Env = env
	}
	cmd.WaitDelay = 100 * time.Millisecond
	stdinRead, stdinWrite, pipeErr := os.Pipe()
	if pipeErr != nil {
		return nil, executionError(core.CodeProcessFailure, "create stdin pipe", pipeErr)
	}
	defer stdinRead.Close()
	defer stdinWrite.Close()
	stopInput := context.AfterFunc(outputCtx, func() { _ = stdinWrite.Close() })
	defer stopInput()
	cmd.Stdin = stdinRead
	input := &lockedWriter{writer: stdinWrite}
	session := execution.NewSession(runCtx, env, input, cancelOutput)
	cmd.Stdout = session.Stdout()
	cmd.Stderr = session.Stderr()
	if err := interruption(runCtx); err != nil {
		return nil, err
	}
	if err := interruption(runCtx); err != nil {
		return nil, err
	}
	if startErr := cmd.Start(); startErr != nil {
		return nil, executionError(core.CodeProcessFailure, "start contract process", startErr)
	}
	stdinRead.Close()
	_, _ = input.Write(append(payload, '\n'))
	runErr := cmd.Wait()

	return session.Result(runErr)
}

func buildContractEnv(ctx context.Context) []string {
	env := execution.Environment(ctx)
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	return env
}
func executionError(code core.ErrorCode, message string, cause error) error {
	return &core.ExecutionError{Code: code, Message: message, Cause: cause}
}
func interruption(ctx context.Context) error { return execution.Interruption(ctx) }

// snapshotExecutable bounds host memory while taking a private execution snapshot.
func snapshotExecutable(ctx context.Context, source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0555)
	if err != nil {
		return err
	}
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, out.Close())
		}
		n, readErr := in.Read(buffer)
		if n > 0 {
			if _, err := out.Write(buffer[:n]); err != nil {
				return errors.Join(err, out.Close())
			}
		}
		if readErr == io.EOF {
			return out.Close()
		}
		if readErr != nil {
			return errors.Join(readErr, out.Close())
		}
	}
}
