package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
)

type RecoverableOptions struct {
	Workers, MaxBatch int
	ExecutionID       string
}
type RecoverableDeps struct {
	State     core.RecoverableBatchStore
	Executor  core.TransactionExecutor
	Contracts core.ContractReader
}
type Recoverable struct {
	options RecoverableOptions
	deps    RecoverableDeps
	ordered *Ordered
}

var _ core.RecoverableBatchExecutor = (*Recoverable)(nil)

func NewRecoverable(o RecoverableOptions, d RecoverableDeps) (*Recoverable, error) {
	if d.State == nil || d.Executor == nil || d.Contracts == nil || !utf8.ValidString(o.ExecutionID) || len(o.ExecutionID) == 0 || len(o.ExecutionID) > 256 {
		return nil, fmt.Errorf("recoverable state, executor, contracts and executionID required")
	}
	opts, err := normalizeOptions(Options{Workers: o.Workers, MaxBatch: o.MaxBatch})
	if err != nil {
		return nil, err
	}
	o.Workers, o.MaxBatch = opts.Workers, opts.MaxBatch
	return &Recoverable{options: o, deps: d, ordered: &Ordered{options: opts, deps: Deps{Executor: d.Executor}}}, nil
}

type frozenCall struct {
	Address, Function string
	Args              []any
	CallContext       *core.CallContext
	Manifest          string
}

func (b *Recoverable) freeze(ctx context.Context, request core.RecoverableBatchRequest) ([]core.ExecutionRequest, core.BatchIdentity, error) {
	var empty core.BatchIdentity
	if err := ctx.Err(); err != nil {
		return nil, empty, err
	}
	if !utf8.ValidString(request.ID) || len(request.ID) == 0 || len(request.ID) > 256 || len(request.Calls) > b.options.MaxBatch {
		return nil, empty, fmt.Errorf("invalid batch ID or request count")
	}
	views := make([]frozenCall, len(request.Calls))
	manifests := make(map[string]string)
	for i, call := range request.Calls {
		if err := ctx.Err(); err != nil {
			return nil, empty, err
		}
		if call.CallContext != nil && call.CallContext.Host != nil {
			return nil, empty, fmt.Errorf("recoverable batches require explicit context without Host")
		}
		for _, arg := range call.Args {
			switch arg.(type) {
			case nil, bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			default:
				return nil, empty, fmt.Errorf("recoverable arguments must be ABI scalars")
			}
		}
		manifest, ok := manifests[call.Address]
		if !ok {
			c, err := b.deps.Contracts.Get(ctx, call.Address)
			if err != nil {
				return nil, empty, err
			}
			if err := artifact.Validate(ctx, c); err != nil {
				return nil, empty, err
			}
			if c.Address != call.Address || c.Deployment == nil || !c.Guarantees.Deterministic {
				return nil, empty, fmt.Errorf("recoverable calls require deterministic deployed artifacts")
			}
			manifest = c.ManifestSHA256
			manifests[call.Address] = manifest
		}
		cc := call.CallContext
		if cc == nil {
			cc = &core.CallContext{}
		}
		// JSON round-trip copies pointer fields and normalizes integer arguments without
		// invoking arbitrary MarshalJSON implementations or retaining caller storage.
		args := call.Args
		if args == nil {
			args = []any{}
		}
		views[i] = frozenCall{Address: call.Address, Function: call.Function, Args: args, CallContext: cc, Manifest: manifest}
	}
	data, err := json.Marshal(struct {
		Version     int
		ExecutionID string
		Calls       []frozenCall
	}{1, b.options.ExecutionID, views})
	if err != nil {
		return nil, empty, err
	}
	if len(data) > 16<<20 {
		return nil, empty, fmt.Errorf("recoverable input exceeds 16 MiB")
	}
	decoded := struct {
		Version     int
		ExecutionID string
		Calls       []frozenCall
	}{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, empty, err
	}
	calls := make([]core.ExecutionRequest, len(views))
	for i, v := range decoded.Calls {
		calls[i] = core.ExecutionRequest{Address: v.Address, Function: v.Function, Args: v.Args, CallContext: v.CallContext}
	}
	digest := sha256.Sum256(data)
	return calls, core.BatchIdentity{ID: request.ID, Fingerprint: hex.EncodeToString(digest[:]), Count: len(calls)}, nil
}
func (b *Recoverable) ExecuteRecoverableBatch(ctx context.Context, request core.RecoverableBatchRequest) (out []core.BatchItemResult, returnErr error) {
	calls, identity, err := b.freeze(ctx, request)
	if err != nil {
		return nil, err
	}
	session, err := b.deps.State.AcquireBatch(ctx, identity)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("batch store returned nil session")
	}
	defer func() { returnErr = errors.Join(returnErr, session.Close()) }()
	prefix, err := session.Prefix(ctx)
	if err != nil {
		return nil, err
	}
	if len(prefix) > len(calls) {
		return nil, fmt.Errorf("invalid batch prefix length")
	}
	for i, receipt := range prefix {
		if receipt.Index != i {
			return nil, fmt.Errorf("invalid batch prefix order")
		}
		out = append(out, receiptItem(receipt))
	}
	for start := len(prefix); start < len(calls); start += b.options.Workers {
		end := min(start+b.options.Workers, len(calls))
		base := start
		window, err := b.ordered.windowWith(ctx, calls[start:end], session, func(ctx context.Context, i int, a *attempt) error {
			receipt := core.BatchReceipt{Index: base + i, Result: a.result}
			if a.err != nil {
				var ee *core.ExecutionError
				if !errors.As(a.err, &ee) {
					return a.err
				}
				receipt.Failure = &core.BatchFailure{Code: ee.Code, Message: ee.Message}
				if a.result != nil {
					receipt.Result = &core.ExecuteResult{GasConsumed: a.result.GasConsumed}
				}
			}
			data, err := json.Marshal(receipt)
			if err != nil {
				return err
			}
			var canonical core.BatchReceipt
			dec := json.NewDecoder(bytes.NewReader(data))
			dec.UseNumber()
			if err := dec.Decode(&canonical); err != nil {
				return err
			}
			item := receiptItem(canonical)
			if err := session.Finalize(ctx, a.tx, canonical); err != nil {
				return err
			}
			a.result, a.err = item.Result, item.Err
			return nil
		})
		out = append(out, window...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func receiptItem(r core.BatchReceipt) core.BatchItemResult {
	item := core.BatchItemResult{Result: r.Result}
	if r.Failure != nil {
		item.Err = &core.ExecutionError{Code: r.Failure.Code, Message: r.Failure.Message}
	}
	return item
}
