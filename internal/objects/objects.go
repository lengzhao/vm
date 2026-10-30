// Package objects implements transaction-bound, contract-scoped owned objects.
package objects

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/lengzhao/vm/core"
)

const MaxRecordBytes = 1 << 20
const MaxFields = 128

// Reserved keys are unreachable through DefaultObject's UTF-8 field API.
const keyPrefix = "\xffvm/objects/v1/"

type record struct {
	Version int               `json:"version"`
	Owner   core.Address      `json:"owner"`
	Deleted bool              `json:"deleted,omitempty"`
	Fields  map[string][]byte `json:"fields,omitempty"`
}

func objectError(message string) error {
	return &core.ExecutionError{Code: core.CodeObjectFailure, Message: message}
}

type OwnerPolicy struct{}

func (OwnerPolicy) Authorize(ctx context.Context, s core.ObjectScope, owner core.Address, op string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.Contract == "" {
		return objectError("object contract required")
	}
	if op == "get" || op == "owner" {
		return nil
	}
	if s.Sender == "" || s.Sender != owner {
		return objectError("object owner authorization denied")
	}
	return nil
}

type Service struct{ policy core.ObjectAuthorizer }

func New(p core.ObjectAuthorizer) *Service { return &Service{policy: p} }
func (s *Service) Bind(tx core.StateTransaction, scope core.ObjectScope) core.ObjectSession {
	return &session{tx: tx, scope: scope, policy: s.policy}
}

type session struct {
	mu     sync.Mutex
	tx     core.StateTransaction
	scope  core.ObjectScope
	policy core.ObjectAuthorizer
	err    error
	closed bool
}

func (s *session) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *session) Close()     { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true }
func validAddress(a core.Address) bool {
	return len(a) > 0 && len(a) <= 256 && utf8.ValidString(string(a))
}
func (s *session) Apply(ctx context.Context, c core.ObjectCommand) (out core.ObjectReply, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if err != nil && s.err == nil {
			s.err = err
		}
	}()
	if s.err != nil {
		return out, s.err
	}
	if s.closed {
		return out, fmt.Errorf("object frame closed")
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if s.tx == nil || s.policy == nil || s.scope.Contract == "" {
		return out, fmt.Errorf("object service dependencies required")
	}
	if len(c.ID) == 0 || len(c.ID) > 128 || !utf8.ValidString(c.ID) || !utf8.ValidString(c.Key) || len(c.Key) > core.MaxObjectKeyBytes || len(c.Value) > core.MaxObjectValueBytes {
		return out, objectError("invalid object input or resource limit")
	}
	switch c.Op {
	case "create", "owner", "transfer", "destroy":
		if c.Key != "" {
			return out, objectError("unexpected object key")
		}
	case "get", "set", "delete":
	default:
		return out, objectError("unknown object operation")
	}
	if c.Op != "set" && len(c.Value) > 0 {
		return out, objectError("unexpected object value")
	}
	if c.Op != "transfer" && c.Owner != "" {
		return out, objectError("unexpected object owner")
	}
	key := core.StateKey{Contract: s.scope.Contract, Key: keyPrefix + c.ID}
	raw, exists, e := s.tx.Get(ctx, key)
	if e != nil {
		return out, e
	}
	var r record
	if exists {
		if len(raw) > MaxRecordBytes {
			return out, fmt.Errorf("object record too large")
		}
		if e = json.Unmarshal(raw, &r); e != nil || r.Version != 1 || !validAddress(r.Owner) || len(r.Fields) > MaxFields {
			return out, fmt.Errorf("invalid object record")
		}
		for k, v := range r.Fields {
			if !utf8.ValidString(k) || len(k) > core.MaxObjectKeyBytes || len(v) > core.MaxObjectValueBytes {
				return out, fmt.Errorf("invalid object fields")
			}
		}
	}
	if c.Op == "create" {
		if exists {
			return out, objectError("object ID already used")
		}
		if !validAddress(s.scope.Sender) {
			return out, objectError("object creator required")
		}
		r = record{Version: 1, Owner: s.scope.Sender, Fields: map[string][]byte{}}
	} else if !exists || r.Deleted {
		return out, objectError("object does not exist")
	}
	if e = s.policy.Authorize(ctx, s.scope, r.Owner, c.Op); e != nil {
		return out, e
	}
	out.Owner = r.Owner
	switch c.Op {
	case "get":
		v, ok := r.Fields[c.Key]
		out.Value = append([]byte(nil), v...)
		out.Exists = ok
		return out, nil
	case "owner":
		return out, nil
	case "set":
		if r.Fields == nil {
			r.Fields = map[string][]byte{}
		}
		r.Fields[c.Key] = append([]byte(nil), c.Value...)
	case "delete":
		delete(r.Fields, c.Key)
	case "transfer":
		if !validAddress(c.Owner) {
			return out, objectError("new object owner required")
		}
		r.Owner = c.Owner
		out.Owner = c.Owner
	case "destroy":
		r.Deleted = true
		r.Fields = nil
	}
	if len(r.Fields) > MaxFields {
		return out, objectError("object field limit exceeded")
	}
	raw, e = json.Marshal(r)
	if e != nil {
		return out, e
	}
	if len(raw) > MaxRecordBytes {
		return out, objectError("object record limit exceeded")
	}
	if e = s.tx.Set(ctx, key, raw); e != nil {
		return out, e
	}
	return out, nil
}

var _ core.ObjectService = (*Service)(nil)
var _ core.ObjectAuthorizer = OwnerPolicy{}
