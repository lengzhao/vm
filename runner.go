package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// CallRequest 合约调用请求
type CallRequest struct {
	Function string        `json:"function"`
	Args     []interface{} `json:"args"`
}

// CallResult 合约调用结果
type CallResult struct {
	Data        []byte
	RawResult   interface{}
	GasConsumed uint64
	Stdout      string
	Stderr      string
}

// Runner 执行已编译合约产物
type Runner interface {
	Run(ctx context.Context, contract *CompiledContract, req CallRequest) (*CallResult, error)
}

// ProcessRunner 通过子进程执行合约二进制
type ProcessRunner struct {
	timeout time.Duration
}

// NewProcessRunner 创建进程 Runner
func NewProcessRunner(timeout time.Duration) Runner {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ProcessRunner{timeout: timeout}
}

type processResponse struct {
	OK     bool        `json:"ok"`
	Result interface{} `json:"result"`
	Error  string      `json:"error"`
	Gas    uint64      `json:"gas"`
}

// Run 执行合约函数
func (r *ProcessRunner) Run(ctx context.Context, contract *CompiledContract, req CallRequest) (*CallResult, error) {
	if contract == nil {
		return nil, fmt.Errorf("contract cannot be nil")
	}
	if contract.ExecutablePath == "" {
		return nil, fmt.Errorf("executable path cannot be empty")
	}
	if _, err := os.Stat(contract.ExecutablePath); err != nil {
		return nil, fmt.Errorf("executable not found: %w", err)
	}
	if req.Function == "" {
		return nil, fmt.Errorf("function name cannot be empty")
	}
	if req.Args == nil {
		req.Args = []interface{}{}
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	runCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		runCtx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, contract.ExecutablePath)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if limit, ok := gasLimitFromContext(ctx); ok {
		cmd.Env = append(os.Environ(), fmt.Sprintf("VM_GAS_LIMIT=%d", limit))
	} else {
		cmd.Env = os.Environ()
	}

	runErr := cmd.Run()
	result := &CallResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	var resp processResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		if runErr != nil {
			return result, fmt.Errorf("contract execution failed: %w: %s", runErr, stderr.String())
		}
		return result, fmt.Errorf("invalid contract response: %w: %s", err, stdout.String())
	}

	result.GasConsumed = resp.Gas
	result.RawResult = resp.Result

	if !resp.OK {
		errMsg := resp.Error
		if errMsg == "" {
			errMsg = "contract execution failed"
		}
		if runErr != nil {
			return result, fmt.Errorf("%s: %w", errMsg, runErr)
		}
		return result, fmt.Errorf("%s", errMsg)
	}

	if resp.Result == nil {
		result.Data = []byte("null")
	} else {
		data, err := json.Marshal(resp.Result)
		if err != nil {
			return result, fmt.Errorf("failed to marshal result: %w", err)
		}
		result.Data = data
	}

	if runErr != nil {
		return result, fmt.Errorf("contract execution failed: %w", runErr)
	}

	return result, nil
}

type gasLimitKey struct{}

// WithGasLimit 将 Gas 限制放入 context，供 Runner 传给子进程
func WithGasLimit(ctx context.Context, limit uint64) context.Context {
	return context.WithValue(ctx, gasLimitKey{}, limit)
}

func gasLimitFromContext(ctx context.Context) (uint64, bool) {
	v := ctx.Value(gasLimitKey{})
	if v == nil {
		return 0, false
	}
	limit, ok := v.(uint64)
	return limit, ok
}
