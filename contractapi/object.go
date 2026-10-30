package contractapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"
)

const (
	MaxObjectKeyBytes          = 1024
	MaxObjectValueBytes        = 64 << 10
	MaxObjectCalls             = 1024
	MaxObjectReplyBytes        = 1 << 20
	GasObjectCreate     uint64 = 20
	GasObjectTransfer   uint64 = 10
	GasObjectDestroy    uint64 = 10
	GasObjectGet        uint64 = 5
	GasObjectSet        uint64 = 10
	GasObjectDelete     uint64 = 5
)

// Object is a handle within the current contract. The host binds its namespace
// and checks authorization for each owned-object operation.
type Object struct {
	id    string
	owned bool
}

func DefaultObject() Object { return Object{} }

type ObjectRequest struct {
	ID      string  `json:"id,omitempty"`
	Owner   string  `json:"owner,omitempty"`
	Version int     `json:"version"`
	Type    string  `json:"type"`
	Op      string  `json:"op"`
	Key     string  `json:"key"`
	Value   []byte  `json:"value,omitempty"`
	Gas     *uint64 `json:"gas"`
}
type ObjectResponse struct {
	Owner   string `json:"owner,omitempty"`
	Version int    `json:"version"`
	OK      *bool  `json:"ok"`
	Exists  *bool  `json:"exists"`
	Value   []byte `json:"value"`
	Error   string `json:"error,omitempty"`
}

var objectIO struct {
	sync.Mutex
	reader      *bufio.Reader
	writer      io.Writer
	calls       int
	callFailure error
}

// InitObjectIO is reserved for generated entry code, never a contract API.
func InitObjectIO(reader io.Reader, writer io.Writer) {
	objectIO.Lock()
	defer objectIO.Unlock()
	objectIO.reader = bufio.NewReader(io.LimitReader(reader, MaxObjectReplyBytes+1))
	objectIO.writer = writer
	objectIO.calls = 0
	objectIO.callFailure = nil
}
func (o Object) Get(key string) ([]byte, bool, error) {
	r, err := o.call("get", key, nil, "", GasObjectGet)
	if err != nil {
		return nil, false, err
	}
	return r.Value, *r.Exists, nil
}
func (o Object) Set(key string, value []byte) error {
	_, err := o.call("set", key, value, "", GasObjectSet)
	return err
}
func (o Object) Delete(key string) error {
	_, err := o.call("delete", key, nil, "", GasObjectDelete)
	return err
}

// OpenObject selects an ID within the current contract; the host checks existence
// and permissions on every operation. An empty ID is invalid, never DefaultObject.
func OpenObject(id string) Object {
	return Object{id: id, owned: true}
}
func CreateObject(id string) (Object, error) {
	o := OpenObject(id)
	_, e := o.call("create", "", nil, "", GasObjectCreate)
	return o, e
}
func (o Object) Owner() (string, error) {
	r, e := o.call("owner", "", nil, "", GasObjectGet)
	return r.Owner, e
}
func (o Object) Transfer(owner string) error {
	_, e := o.call("transfer", "", nil, owner, GasObjectTransfer)
	return e
}
func (o Object) Destroy() error { _, e := o.call("destroy", "", nil, "", GasObjectDestroy); return e }
func objectCall(op, key string, value []byte, cost uint64) (ObjectResponse, error) {
	return (Object{}).call(op, key, value, "", cost)
}
func (o Object) call(op, key string, value []byte, owner string, cost uint64) (ObjectResponse, error) {
	objectIO.Lock()
	defer objectIO.Unlock()
	var response ObjectResponse
	if o.owned && (len(o.id) == 0 || len(o.id) > 128 || !utf8.ValidString(o.id)) {
		return response, fmt.Errorf("invalid object ID")
	}
	if (op == "owner" || op == "transfer" || op == "destroy") && o.id == "" {
		return response, fmt.Errorf("owned object required")
	}
	if op == "transfer" && (owner == "" || len(owner) > 256 || !utf8.ValidString(string(owner))) {
		return response, fmt.Errorf("invalid owner")
	}
	if !utf8.ValidString(key) || len(key) > MaxObjectKeyBytes {
		return response, fmt.Errorf("invalid object field name")
	}
	if len(value) > MaxObjectValueBytes {
		return response, fmt.Errorf("object value exceeds 64 KiB")
	}
	ConsumeGas(cost)
	if objectIO.reader == nil || objectIO.writer == nil {
		return response, fmt.Errorf("object host unavailable")
	}
	objectIO.calls++
	if objectIO.calls > MaxObjectCalls {
		return response, fmt.Errorf("object call limit exceeded")
	}
	gas := GasUsed()
	if err := json.NewEncoder(objectIO.writer).Encode(ObjectRequest{ID: o.id, Owner: owner, Version: ProtocolVersion, Type: "object", Op: op, Key: key, Value: value, Gas: &gas}); err != nil {
		return response, err
	}
	line, err := objectIO.reader.ReadBytes('\n')
	if err != nil {
		return response, fmt.Errorf("object host response: %w", err)
	}
	if len(line) > MaxObjectReplyBytes {
		return response, fmt.Errorf("object response limit exceeded")
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return response, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return response, fmt.Errorf("multiple object responses")
	}
	if response.Version != ProtocolVersion || response.OK == nil || response.Exists == nil {
		return response, fmt.Errorf("invalid object response")
	}
	if !*response.OK {
		return response, fmt.Errorf("object host failed: %s", response.Error)
	}
	if response.Error != "" || len(response.Value) > MaxObjectValueBytes || (!*response.Exists && len(response.Value) > 0) {
		return response, fmt.Errorf("invalid object response fields")
	}
	return response, nil
}
