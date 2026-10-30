package contractapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const MaxCrossResultBytes = 64 << 10

type CrossCallRequest struct {
	Version  int     `json:"version"`
	Type     string  `json:"type"`
	Address  string  `json:"address"`
	Function string  `json:"function"`
	Args     []any   `json:"args"`
	Gas      *uint64 `json:"gas"`
	Events   []Event `json:"events,omitempty"`
}
type CrossCallResponse struct {
	Version int     `json:"version"`
	OK      *bool   `json:"ok"`
	Result  []byte  `json:"result"`
	Gas     *uint64 `json:"gas"`
}

// Call invokes a different, inactive contract within the shared temporary store.
// The returned bytes encode the child's JSON result, not a Go typed value.
func Call(address, function string, args ...any) (data []byte, err error) {
	objectIO.Lock()
	defer objectIO.Unlock()
	defer func() {
		if err != nil && objectIO.callFailure == nil {
			objectIO.callFailure = err
		}
	}()
	if objectIO.callFailure != nil {
		return nil, objectIO.callFailure
	}
	ConsumeGas(GasCrossCall)
	if objectIO.reader == nil || objectIO.writer == nil {
		return nil, fmt.Errorf("call host unavailable")
	}
	objectIO.calls++
	if objectIO.calls > MaxObjectCalls {
		return nil, fmt.Errorf("host call limit exceeded")
	}
	if args == nil {
		args = []any{}
	}
	gas := GasUsed()
	payload, err := json.Marshal(CrossCallRequest{Version: ProtocolVersion, Type: "call", Address: address, Function: function, Args: args, Gas: &gas, Events: DrainEvents()})
	if err != nil {
		return nil, err
	}
	if len(payload) > 1<<20 {
		return nil, fmt.Errorf("call request exceeds 1 MiB")
	}
	if _, err = objectIO.writer.Write(append(payload, '\n')); err != nil {
		return nil, err
	}
	line, err := objectIO.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var response CrossCallResponse
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&response); err != nil {
		return nil, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("multiple call replies")
	}
	if response.Version != ProtocolVersion || response.OK == nil || !*response.OK || response.Gas == nil || len(response.Result) > MaxCrossResultBytes || !json.Valid(response.Result) {
		return nil, fmt.Errorf("invalid cross-call reply")
	}
	ConsumeGas(*response.Gas)
	return response.Result, nil
}

// CallFailure is reserved for entry code so an ignored local transport/encoding
// failure cannot turn into a successful transaction.
func CallFailure() error { objectIO.Lock(); defer objectIO.Unlock(); return objectIO.callFailure }
