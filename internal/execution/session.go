package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/callcontext"
	"io"
	"strconv"
	"strings"
)

type processResponse struct {
	Version int            `json:"version"`
	Code    core.ErrorCode `json:"code"`
	OK      *bool          `json:"ok"`
	Result  interface{}    `json:"result"`
	Error   string         `json:"error"`
	Gas     *uint64        `json:"gas"`
	Events  []core.Event   `json:"events"`
}

// Session owns one call's bounded protocol and host accounting. Its IO methods
// are used serially by the guest transport; independent sessions can run concurrently.
type Session struct {
	ctx    context.Context
	stdout objectOutput
	stderr boundedOutput
}

func NewSession(ctx context.Context, env []string, reply io.Writer, cancel context.CancelFunc) *Session {
	store, _ := core.ObjectStoreFromContext(ctx)
	s := &Session{ctx: ctx, stdout: objectOutput{boundedOutput: boundedOutput{limit: maxContractOutput, cancel: cancel}, ctx: ctx, reply: reply, store: store, gasFloor: contractapi.GasEntryBase}, stderr: boundedOutput{limit: maxContractOutput, cancel: cancel}}
	for _, entry := range env {
		if strings.HasPrefix(entry, "VM_GAS_LIMIT=") {
			s.stdout.gasLimit, _ = strconv.ParseUint(strings.TrimPrefix(entry, "VM_GAS_LIMIT="), 10, 64)
		}
	}
	return s
}
func (s *Session) Stdout() io.Writer { return &s.stdout }
func (s *Session) Stderr() io.Writer { return &s.stderr }
func (s *Session) Result(runErr error) (*core.CallResult, error) {
	result := &core.CallResult{Stdout: s.stdout.String(), Stderr: s.stderr.String()}
	// Retain the host-validated lower bound even when no valid final response
	// arrives. The initial entry charge alone is not evidence of execution.
	if s.stdout.gasFloor > contractapi.GasEntryBase {
		result.GasConsumed = s.stdout.gasFloor
	}
	if s.stdout.exceeded || s.stderr.exceeded {
		return result, executionError(core.CodeOutputLimit, "contract output limit exceeded", nil)
	}
	if err := s.ctx.Err(); err != nil {
		code := core.CodeCanceled
		if err == context.DeadlineExceeded {
			code = core.CodeTimeout
		}
		return result, executionError(code, "contract execution interrupted: "+err.Error(), err)
	}
	if s.stdout.err != nil {
		return result, s.stdout.err
	}
	var resp processResponse
	decoder := json.NewDecoder(bytes.NewReader(s.stdout.resultBytes()))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resp); err != nil {
		if runErr != nil {
			return result, executionError(core.CodeProcessFailure, "contract execution failed: "+runErr.Error()+": "+s.stderr.String(), runErr)
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
	if s.stdout.calls > 0 && *resp.Gas < s.stdout.gasFloor {
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
	data, err := json.Marshal(resp.Result)
	result.Data = data
	if err != nil {
		return result, executionError(core.CodeInvalidResponse, "cannot encode response result", err)
	}
	return result, nil
}
func Environment(ctx context.Context) []string {
	// 不继承宿主完整环境，降低信息泄漏面
	env := make([]string, 0, 8)

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

func EncodeRequest(ctx context.Context, req core.CallRequest) ([]byte, error) {
	if err := interruption(ctx); err != nil {
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
	if err := interruption(ctx); err != nil {
		return nil, err
	}
	return payload, nil
}

func Interruption(ctx context.Context) error { return interruption(ctx) }
