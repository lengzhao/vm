package process

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/callcontext"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProcessRunner 通过子进程执行合约二进制
type ProcessRunner struct {
	timeout time.Duration
}

// New 创建进程 core.Runner
func New(timeout time.Duration) core.Runner {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ProcessRunner{timeout: timeout}
}

type processResponse struct {
	Version int            `json:"version"`
	Code    core.ErrorCode `json:"code"`
	OK      *bool          `json:"ok"`
	Result  interface{}    `json:"result"`
	Error   string         `json:"error"`
	Gas     *uint64        `json:"gas"`
	Events  []core.Event   `json:"events"`
}

// Run 执行合约函数
func (r *ProcessRunner) Run(ctx context.Context, contract *core.CompiledContract, req core.CallRequest) (*core.CallResult, error) {
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
	if err := artifact.ValidateNativeRules(contract); err != nil {
		return nil, err
	}

	if req.Function == "" {
		return nil, executionError(core.CodeInvalidRequest, "function name cannot be empty", nil)
	}
	if req.Args == nil {
		req.Args = []interface{}{}
	}
	payload, err := json.Marshal(struct {
		Version  int           `json:"version"`
		Function string        `json:"function"`
		Args     []interface{} `json:"args"`
	}{core.ProtocolVersion, req.Function, req.Args})
	if err != nil {
		return nil, executionError(core.CodeInvalidRequest, "failed to marshal request: "+err.Error(), err)
	}
	if len(payload) > 1<<20 {
		return nil, executionError(core.CodeInvalidRequest, "request exceeds 1 MiB", nil)
	}
	outputCtx, cancelOutput := context.WithCancel(runCtx)
	defer cancelOutput()
	cmd := exec.CommandContext(outputCtx, contract.ExecutablePath)
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
	store, _ := core.ObjectStoreFromContext(ctx)
	stdout := objectOutput{boundedOutput: boundedOutput{limit: maxContractOutput, cancel: cancelOutput}, ctx: runCtx, reply: input, store: store, gasFloor: contractapi.GasEntryBase}
	stderr := boundedOutput{limit: maxContractOutput, cancel: cancelOutput}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := interruption(runCtx); err != nil {
		return nil, err
	}
	cmd.Env = buildContractEnv(ctx)
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "VM_GAS_LIMIT=") {
			stdout.gasLimit, _ = strconv.ParseUint(strings.TrimPrefix(entry, "VM_GAS_LIMIT="), 10, 64)
		}
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

	result := &core.CallResult{Stdout: stdout.String(), Stderr: stderr.String()}
	// Retain the host-validated lower bound even when no valid final response
	// arrives. The initial entry charge alone is not evidence of execution.
	if stdout.gasFloor > contractapi.GasEntryBase {
		result.GasConsumed = stdout.gasFloor
	}
	if stdout.exceeded || stderr.exceeded {
		return result, executionError(core.CodeOutputLimit, "contract output limit exceeded", nil)
	}
	if err := runCtx.Err(); err != nil {
		code := core.CodeCanceled
		if err == context.DeadlineExceeded {
			code = core.CodeTimeout
		}
		return result, executionError(code, "contract execution interrupted: "+err.Error(), err)
	}
	if stdout.err != nil {
		return result, stdout.err
	}
	var resp processResponse
	decoder := json.NewDecoder(bytes.NewReader(stdout.resultBytes()))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resp); err != nil {
		if runErr != nil {
			return result, executionError(core.CodeProcessFailure, "contract execution failed: "+runErr.Error()+": "+stderr.String(), runErr)
		}
		return result, executionError(core.CodeInvalidResponse, "invalid contract response: "+err.Error(), err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, executionError(core.CodeInvalidResponse, "expected a single JSON response", err)
	}
	if resp.Version != core.ProtocolVersion {
		return result, executionError(core.CodeUnsupportedVersion, "unsupported response protocol; recompile and deploy", nil)
	}
	if resp.OK == nil || resp.Gas == nil {
		return result, executionError(core.CodeInvalidResponse, "response requires ok and gas fields", nil)
	}
	if *resp.OK && (resp.Code != "" || resp.Error != "") {
		return result, executionError(core.CodeInvalidResponse, "success response contains an error", nil)
	}
	if !*resp.OK && (!remoteErrorCode(resp.Code) || resp.Error == "" || resp.Result != nil) {
		return result, executionError(core.CodeInvalidResponse, "failure response requires a supported code and diagnostic", nil)
	}
	if stdout.calls > 0 && *resp.Gas < stdout.gasFloor {
		return result, executionError(core.CodeInvalidResponse, "final gas is below host calls", nil)
	}
	result.GasConsumed = *resp.Gas
	result.RawResult = resp.Result
	result.Events = resp.Events
	if !*resp.OK {
		return result, executionError(resp.Code, resp.Error, runErr)
	}
	if runErr != nil {
		return result, executionError(core.CodeProcessFailure, "contract process failed: "+runErr.Error(), runErr)
	}
	result.Data, err = json.Marshal(resp.Result)
	if err != nil {
		return result, executionError(core.CodeInvalidResponse, "cannot encode response result", err)
	}
	return result, nil
}

func buildContractEnv(ctx context.Context) []string {
	// 不继承宿主完整环境，降低信息泄漏面
	env := make([]string, 0, 8)
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}

	var gasLimit uint64
	var hasGas bool
	if limit, ok := callcontext.GasLimitFromContext(ctx); ok {
		gasLimit = limit
		hasGas = true
	}
	if callCtx, ok := callcontext.CallContextFrom(ctx); ok && callCtx != nil {
		resolved := callcontext.Resolve(callCtx)
		if resolved.BlockHeightSet {
			env = append(env, fmt.Sprintf("VM_BLOCK_HEIGHT=%d", resolved.BlockHeight))
		}
		if resolved.BlockTimeSet {
			env = append(env, fmt.Sprintf("VM_BLOCK_TIME=%d", resolved.BlockTime))
		}
		if resolved.SenderSet {
			env = append(env, "VM_SENDER="+string(resolved.Sender))
		}
		if resolved.ContractSet {
			env = append(env, "VM_CONTRACT_ADDRESS="+string(resolved.Contract))
		}
		if resolved.GasLimitSet {
			gasLimit = resolved.GasLimit
			hasGas = true
		}
	}
	if hasGas {
		env = append(env, fmt.Sprintf("VM_GAS_LIMIT=%d", gasLimit))
	}
	return env
}

// Each stream has its own bound, including error responses and diagnostics.
const maxContractOutput = 1 << 20

type boundedOutput struct {
	buffer   bytes.Buffer
	limit    int
	cancel   context.CancelFunc
	exceeded bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if n > remaining {
		_, _ = b.buffer.Write(p[:remaining])
		b.exceeded = true
		b.cancel()
		return n, nil
	}
	return b.buffer.Write(p)
}

func (b *boundedOutput) Len() int       { return b.buffer.Len() }
func (b *boundedOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedOutput) String() string { return b.buffer.String() }

func executionError(code core.ErrorCode, message string, cause error) error {
	return &core.ExecutionError{Code: code, Message: message, Cause: cause}
}

func remoteErrorCode(code core.ErrorCode) bool {
	switch code {
	case core.CodeCallFailure, core.CodeInvalidRequest, core.CodeInvalidArguments, core.CodeUnknownFunction, core.CodeContractError, core.CodeContractPanic, core.CodeGasLimitExceeded, core.CodeUnsupportedVersion, core.CodeInvalidResult:
		return true
	}
	return false
}

func interruption(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		code := core.CodeCanceled
		if err == context.DeadlineExceeded {
			code = core.CodeTimeout
		}
		return executionError(code, "contract execution interrupted: "+err.Error(), err)
	}
	return nil
}
