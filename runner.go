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
	Events      []Event
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
	Events []Event     `json:"events"`
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
	cmd.Env = buildContractEnv(ctx)

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
	result.Events = resp.Events

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

func buildContractEnv(ctx context.Context) []string {
	// 不继承宿主完整环境，降低信息泄漏面
	env := make([]string, 0, 8)
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}

	var gasLimit uint64
	var hasGas bool
	if limit, ok := gasLimitFromContext(ctx); ok {
		gasLimit = limit
		hasGas = true
	}
	if callCtx, ok := CallContextFrom(ctx); ok && callCtx != nil {
		resolved := resolveCallContext(callCtx)
		if resolved.blockHeightSet {
			env = append(env, fmt.Sprintf("VM_BLOCK_HEIGHT=%d", resolved.blockHeight))
		}
		if resolved.blockTimeSet {
			env = append(env, fmt.Sprintf("VM_BLOCK_TIME=%d", resolved.blockTime))
		}
		if resolved.senderSet {
			env = append(env, "VM_SENDER="+string(resolved.sender))
		}
		if resolved.contractSet {
			env = append(env, "VM_CONTRACT_ADDRESS="+string(resolved.contract))
		}
		if resolved.gasLimitSet {
			gasLimit = resolved.gasLimit
			hasGas = true
		}
	}
	if hasGas {
		env = append(env, fmt.Sprintf("VM_GAS_LIMIT=%d", gasLimit))
	}
	return env
}

type resolvedCallContext struct {
	blockHeight    uint64
	blockTime      uint64
	sender         Address
	contract       Address
	gasLimit       uint64
	blockHeightSet bool
	blockTimeSet   bool
	senderSet      bool
	contractSet    bool
	gasLimitSet    bool
}

func resolveCallContext(cc *CallContext) resolvedCallContext {
	var out resolvedCallContext
	if cc.BlockHeight != nil {
		out.blockHeight = *cc.BlockHeight
		out.blockHeightSet = true
	} else if cc.Host != nil {
		out.blockHeight = cc.Host.BlockHeight()
		out.blockHeightSet = true
	}
	if cc.BlockTime != nil {
		out.blockTime = *cc.BlockTime
		out.blockTimeSet = true
	} else if cc.Host != nil {
		out.blockTime = cc.Host.BlockTime()
		out.blockTimeSet = true
	}
	if cc.Sender != nil {
		out.sender = *cc.Sender
		out.senderSet = true
	} else if cc.Host != nil {
		out.sender = cc.Host.Sender()
		out.senderSet = true
	}
	if cc.ContractAddress != nil {
		out.contract = *cc.ContractAddress
		out.contractSet = true
	} else if cc.Host != nil {
		out.contract = cc.Host.ContractAddress()
		out.contractSet = true
	}
	if cc.GasLimit != nil {
		out.gasLimit = *cc.GasLimit
		out.gasLimitSet = true
	}
	return out
}

type gasLimitKey struct{}
type callContextKey struct{}

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

// WithCallContext 将调用上下文放入 context
func WithCallContext(ctx context.Context, callCtx *CallContext) context.Context {
	return context.WithValue(ctx, callContextKey{}, callCtx)
}

// CallContextFrom 从 context 取出 CallContext
func CallContextFrom(ctx context.Context) (*CallContext, bool) {
	v := ctx.Value(callContextKey{})
	if v == nil {
		return nil, false
	}
	cc, ok := v.(*CallContext)
	return cc, ok
}
