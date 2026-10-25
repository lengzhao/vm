package process

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
)

func (o *objectOutput) handleCall(line []byte) error {
	reject := func(code core.ErrorCode, err error) error { o.fail(code, err); return err }
	if err := interruption(o.ctx); err != nil {
		o.err = err
		o.cancel()
		return err
	}
	var req contractapi.CrossCallRequest
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		return reject(core.CodeInvalidResponse, err)
	}
	o.calls++
	if req.Version != core.ProtocolVersion || req.Gas == nil || req.Args == nil || req.Address == "" || req.Function == "" || o.calls > core.MaxObjectCalls {
		return reject(core.CodeInvalidResponse, fmt.Errorf("invalid cross-call request"))
	}
	if *req.Gas < o.gasFloor || *req.Gas-o.gasFloor < contractapi.GasCrossCall {
		return reject(core.CodeInvalidResponse, fmt.Errorf("undercharged cross-call"))
	}
	if o.gasLimit > 0 && *req.Gas >= o.gasLimit {
		return reject(core.CodeGasLimitExceeded, fmt.Errorf("no remaining cross-call gas"))
	}
	o.gasFloor = *req.Gas
	caller, ok := core.ContractCallerFromContext(o.ctx)
	if !ok {
		return reject(core.CodeCallFailure, fmt.Errorf("cross-contract caller unavailable"))
	}
	events := make([]core.Event, len(req.Events))
	for i, event := range req.Events {
		events[i] = core.Event{Name: event.Name, Fields: event.Fields}
	}
	result, err := caller.Call(o.ctx, core.CrossCallRequest{Address: req.Address, Function: req.Function, Args: req.Args, GasConsumed: *req.Gas, Events: events})
	if err != nil {
		code := core.CodeCallFailure
		var ee *core.ExecutionError
		if errors.As(err, &ee) {
			code = ee.Code
		}
		return reject(code, err)
	}
	if result == nil || len(result.Data) > core.MaxCrossResultBytes || (len(result.Data) > 0 && !json.Valid(result.Data)) {
		return reject(core.CodeInvalidResponse, fmt.Errorf("invalid child result"))
	}
	if result.GasConsumed > ^uint64(0)-*req.Gas {
		return reject(core.CodeGasLimitExceeded, fmt.Errorf("cross-call gas overflow"))
	}
	o.gasFloor = *req.Gas + result.GasConsumed
	if o.gasLimit > 0 && o.gasFloor > o.gasLimit {
		return reject(core.CodeGasLimitExceeded, fmt.Errorf("child exceeds shared gas budget"))
	}
	data := result.Data
	if len(data) == 0 {
		data = []byte("null")
	}
	ok = true
	gas := result.GasConsumed
	payload, err := json.Marshal(contractapi.CrossCallResponse{Version: core.ProtocolVersion, OK: &ok, Result: data, Gas: &gas})
	if err != nil {
		return reject(core.CodeInvalidResponse, err)
	}
	return o.sendReply(payload)
}
