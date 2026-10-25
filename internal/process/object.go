package process

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
)

// lockedWriter serializes the initial request with subsequent host replies.
type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

type objectOutput struct {
	boundedOutput
	ctx          context.Context
	reply        io.Writer
	store        core.ObjectStore
	pending      []byte
	final        bytes.Buffer
	finalStarted bool
	calls        int
	replyBytes   int
	gasFloor     uint64
	gasLimit     uint64
	err          error
}

func (o *objectOutput) fail(code core.ErrorCode, err error) {
	if o.err == nil {
		o.err = executionError(code, err.Error(), err)
		o.cancel()
	}
}
func (o *objectOutput) Write(p []byte) (int, error) {
	n, err := o.boundedOutput.Write(p)
	if err != nil || o.exceeded || o.err != nil {
		return n, err
	}
	o.pending = append(o.pending, p...)
	for {
		i := bytes.IndexByte(o.pending, '\n')
		if i < 0 {
			break
		}
		line := o.pending[:i+1]
		o.pending = o.pending[i+1:]
		var kind struct {
			Type string `json:"type"`
		}
		if !o.finalStarted && json.Unmarshal(line, &kind) == nil && (kind.Type == "object" || kind.Type == "call") {
			if kind.Type == "call" {
				if err := o.handleCall(line); err != nil {
					return n, nil
				}
				continue
			}
			if err := o.handle(line); err != nil {
				return n, nil
			}
		} else {
			o.finalStarted = true
			o.final.Write(line)
		}
	}
	return n, nil
}
func (o *objectOutput) resultBytes() []byte { return append(o.final.Bytes(), o.pending...) }
func (o *objectOutput) handle(line []byte) error {
	reject := func(code core.ErrorCode, err error) error { o.fail(code, err); return err }
	if err := interruption(o.ctx); err != nil {
		o.err = err
		o.cancel()
		return err
	}
	var req contractapi.ObjectRequest
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return reject(core.CodeInvalidResponse, err)
	}
	o.calls++
	if req.Version != core.ProtocolVersion || req.Gas == nil || o.calls > core.MaxObjectCalls || !utf8.ValidString(req.Key) || len(req.Key) > core.MaxObjectKeyBytes || len(req.Value) > core.MaxObjectValueBytes {
		return reject(core.CodeInvalidResponse, fmt.Errorf("invalid object request or resource limit"))
	}
	var cost uint64
	switch req.Op {
	case "get":
		cost = contractapi.GasObjectGet
	case "set":
		cost = contractapi.GasObjectSet
	case "delete":
		cost = contractapi.GasObjectDelete
	default:
		return reject(core.CodeInvalidResponse, fmt.Errorf("unknown object operation"))
	}
	if req.Op != "set" && len(req.Value) > 0 {
		return reject(core.CodeInvalidResponse, fmt.Errorf("unexpected object value"))
	}
	if *req.Gas < o.gasFloor || *req.Gas-o.gasFloor < cost {
		return reject(core.CodeInvalidResponse, fmt.Errorf("undercharged object operation"))
	}
	if o.gasLimit > 0 && *req.Gas > o.gasLimit {
		return reject(core.CodeGasLimitExceeded, fmt.Errorf("object call exceeds gas limit"))
	}
	o.gasFloor = *req.Gas
	if o.store == nil {
		return reject(core.CodeStateFailure, fmt.Errorf("object store unavailable"))
	}
	if err := interruption(o.ctx); err != nil {
		o.err = err
		o.cancel()
		return err
	}
	exists := false
	var value []byte
	var err error
	switch req.Op {
	case "get":
		value, exists, err = o.store.Get(o.ctx, req.Key)
	case "set":
		err = o.store.Set(o.ctx, req.Key, req.Value)
	case "delete":
		err = o.store.Delete(o.ctx, req.Key)
	}
	if err != nil {
		return reject(core.CodeStateFailure, err)
	}
	if len(value) > core.MaxObjectValueBytes || (!exists && len(value) > 0) {
		return reject(core.CodeStateFailure, fmt.Errorf("invalid host object value"))
	}
	if err := interruption(o.ctx); err != nil {
		o.err = err
		o.cancel()
		return err
	}
	ok := true
	payload, err := json.Marshal(contractapi.ObjectResponse{Version: core.ProtocolVersion, OK: &ok, Exists: &exists, Value: value})
	if err != nil {
		return reject(core.CodeStateFailure, err)
	}
	return o.sendReply(payload)
}
func (o *objectOutput) sendReply(payload []byte) error {
	reject := func(code core.ErrorCode, err error) error { o.fail(code, err); return err }
	if err := interruption(o.ctx); err != nil {
		o.err = err
		o.cancel()
		return err
	}
	payload = append(payload, '\n')
	o.replyBytes += len(payload)
	if o.replyBytes > contractapi.MaxObjectReplyBytes {
		return reject(core.CodeOutputLimit, fmt.Errorf("object replies exceed 1 MiB"))
	}
	if _, err := o.reply.Write(payload); err != nil {
		return reject(core.CodeProcessFailure, err)
	}
	return nil
}
