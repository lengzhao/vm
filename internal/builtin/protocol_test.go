package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var e *ExecutionError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestCallableABIAndCompilerRejectSameTypes(t *testing.T) {
	e := NewVMEngine(VMConfig{EnableSecurityChecks: false, ContractStorageDir: t.TempDir()})
	for _, src := range []string{`func F(v []int)int{return 1}`, `func F() []int{return nil}`, `func F()(error,int){return nil,1}`, `func F[T any]()int{return 1}`, `type int string;func F(a int)int{return a}`} {
		code := "package main;" + src
		_, err := e.GenerateABI(code)
		requireCode(t, err, CodeUnsupportedABI)
		_, err = e.Compile(code)
		requireCode(t, err, CodeUnsupportedABI)
	}
}

func TestProtocolScalarBoundariesAndNames(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;func F(req int8,err uint64,result bool,_ string)(int8,uint64,bool){return req,err,result}`)
	r, err := NewProcessRunner(5*time.Second).Run(context.Background(), c, CallRequest{Function: "F", Args: []any{-128, uint64(18446744073709551615), true, "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Data) != `[-128,18446744073709551615,true]` {
		t.Fatal(string(r.Data))
	}
	for _, args := range [][]any{{128, uint64(1), true, "x"}, {nil, uint64(1), true, "x"}, {"1", uint64(1), true, "x"}, {1}, {1, 1, true, "x", 5}} {
		_, err := NewProcessRunner(5*time.Second).Run(context.Background(), c, CallRequest{Function: "F", Args: args})
		requireCode(t, err, CodeInvalidArguments)
	}
}

func TestProtocolReturnedErrorAndPanicCodes(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;import "errors";func Fail()(int,error){return 9,errors.New("rejected")};func Good()(int,error){return 7,nil};func Boom(){panic("gas limit exceeded")};func Loop(){for{}}`)
	runner := NewProcessRunner(5 * time.Second)
	_, err := runner.Run(context.Background(), c, CallRequest{Function: "Fail"})
	requireCode(t, err, CodeContractError)
	r, err := runner.Run(context.Background(), c, CallRequest{Function: "Good"})
	if err != nil || string(r.Data) != "7" {
		t.Fatalf("%+v %v", r, err)
	}
	_, err = runner.Run(context.Background(), c, CallRequest{Function: "Boom"})
	requireCode(t, err, CodeContractPanic)
	_, err = runner.Run(WithGasLimit(context.Background(), 15), c, CallRequest{Function: "Loop"})
	requireCode(t, err, CodeGasLimitExceeded)
	_, err = runner.Run(context.Background(), c, CallRequest{Function: "Missing"})
	requireCode(t, err, CodeUnknownFunction)
}

func TestProtocolRejectsUnknownRequestVersion(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;func F()int{return 1}`)
	cmd := exec.Command(c.ExecutablePath)
	cmd.Stdin = strings.NewReader(`{"version":99,"function":"F","args":[]}`)
	out, _ := cmd.Output()
	var resp struct {
		Version int
		OK      bool
		Code    ErrorCode
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Version != ProtocolVersion || resp.OK || resp.Code != CodeUnsupportedVersion {
		t.Fatalf("%s", out)
	}
}

func TestProtocolRejectsUnversionedResponse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "contract")
	os.WriteFile(p, []byte("#!/bin/sh\necho '{\"ok\":true,\"result\":7}'\n"), 0755)
	_, err := NewProcessRunner(time.Second).Run(context.Background(), sealedTestArtifact(t, &CompiledContract{ExecutablePath: p, ProtocolVersion: ProtocolVersion}), CallRequest{Function: "F"})
	requireCode(t, err, CodeUnsupportedVersion)
}

func TestProtocolEmptyErrorAndInvalidJSONResult(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;import("errors";"math");func Empty()error{return errors.New("")};func Bad()float64{return math.NaN()}`)
	r := NewProcessRunner(5 * time.Second)
	_, err := r.Run(context.Background(), c, CallRequest{Function: "Empty"})
	requireCode(t, err, CodeContractError)
	_, err = r.Run(context.Background(), c, CallRequest{Function: "Bad"})
	requireCode(t, err, CodeInvalidResult)
}

func TestProtocolInfrastructureCodes(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;func F(){for{}}`)
	r := NewProcessRunner(50 * time.Millisecond)
	_, err := r.Run(context.Background(), c, CallRequest{Function: "F"})
	requireCode(t, err, CodeTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline cause lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.Run(ctx, c, CallRequest{Function: "F"})
	requireCode(t, err, CodeCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation cause lost")
	}
}

func TestProtocolMetadataVersionsAreMandatory(t *testing.T) {
	e := NewVMEngine(VMConfig{ContractStorageDir: t.TempDir()})
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	saved := c.ProtocolVersion
	c.ProtocolVersion = 0
	_, err := e.Deploy(c)
	requireCode(t, err, CodeUnsupportedVersion)
	c.ProtocolVersion = saved
	c.ABI.Version = 0
	_, err = e.Deploy(c)
	requireCode(t, err, CodeUnsupportedABI)
}

func TestABIPredeclaredNamesCannotShadowEntry(t *testing.T) {
	e := NewVMEngine(VMConfig{EnableSecurityChecks: false, ContractStorageDir: t.TempDir()})
	for _, src := range []string{`const true=false;func F()int{return 1}`, `type any=int;func F(v bool)bool{return v}`, `func len(v []int)int{return 0};func F(){}`, `var nil=1;func F(){}`} {
		_, err := e.GenerateABI("package main;" + src)
		requireCode(t, err, CodeUnsupportedABI)
		_, err = e.Compile("package main;" + src)
		requireCode(t, err, CodeUnsupportedABI)
	}
}

func TestProtocolRequiresResponseFields(t *testing.T) {
	// Validate response shape independently of process-startup scheduling.
	// Dedicated timeout/cancellation tests retain their short latency budgets.
	runner := NewProcessRunner(5 * time.Second)
	for _, payload := range []string{`{"version":3,"ok":true,"result":7}`, `{"version":3,"gas":11,"code":"CONTRACT_ERROR","error":"failed"}`, `{"version":3,"ok":false,"gas":11,"code":"CONTRACT_ERROR","error":"failed","result":7}`} {
		p := filepath.Join(t.TempDir(), "contract")
		os.WriteFile(p, []byte("#!/bin/sh\necho '"+payload+"'\n"), 0755)
		_, err := runner.Run(context.Background(), sealedTestArtifact(t, &CompiledContract{ProtocolVersion: ProtocolVersion, ExecutablePath: p}), CallRequest{Function: "F"})
		requireCode(t, err, CodeInvalidResponse)
	}
}

func TestEntryImportsDoNotReserveContractNames(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;type json int;type contractapi int;func F()int{return 7}`)
	r, err := NewProcessRunner(5*time.Second).Run(context.Background(), c, CallRequest{Function: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Data) != "7" {
		t.Fatal(string(r.Data))
	}
}

func TestProtocolAllScalarTypesRoundTrip(t *testing.T) {
	types := []string{"int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "byte", "rune", "float32", "float64", "string", "bool"}
	source := "package main\n"
	for i, typ := range types {
		source += fmt.Sprintf("func F%d(v %s)%s{return v}\n", i, typ, typ)
	}
	c := compileTestContract(t, t.TempDir(), source)
	runner := NewProcessRunner(5 * time.Second)
	for i, typ := range types {
		var arg any = 7
		want := "7"
		switch typ {
		case "float32", "float64":
			arg = 0.25
			want = "0.25"
		case "string":
			arg = "x"
			want = `"x"`
		case "bool":
			arg = true
			want = "true"
		}
		r, err := runner.Run(context.Background(), c, CallRequest{Function: fmt.Sprintf("F%d", i), Args: []any{arg}})
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if string(r.Data) != want {
			t.Fatalf("%s: got %s", typ, r.Data)
		}
	}
}

func TestProtocolRejectsMalformedRequestEnvelopes(t *testing.T) {
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	for _, payload := range []string{`{"version":3,"function":"F","args":null}`, `{"version":3,"function":"F","args":[],"extra":1}`, `{"version":3,"function":"F","args":[]} {}`, strings.Repeat(" ", (1<<20)+1)} {
		cmd := exec.Command(c.ExecutablePath)
		cmd.Stdin = strings.NewReader(payload)
		out, _ := cmd.Output()
		var response struct{ Code ErrorCode }
		if err := json.Unmarshal(out, &response); err != nil {
			t.Fatalf("%v", err)
		}
		if response.Code != CodeInvalidRequest {
			t.Fatalf("%s", out)
		}
	}
	_, err := NewProcessRunner(5*time.Second).Run(context.Background(), c, CallRequest{Function: "F", Args: []any{strings.Repeat("x", 1<<20)}})
	requireCode(t, err, CodeInvalidRequest)
}

func TestProtocolStoredVersionsAreValidatedOnLoad(t *testing.T) {
	dir := t.TempDir()
	e := NewVMEngine(VMConfig{ContractStorageDir: dir})
	c := compileTestContract(t, t.TempDir(), `package main;func F(){}`)
	a, err := e.Deploy(c)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, a, "metadata.json")
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var meta ContractMetadata
	json.Unmarshal(saved, &meta)
	meta.ProtocolVersion = 0
	data, _ := json.Marshal(meta)
	os.WriteFile(path, data, 0644)
	_, err = e.Execute(a, "F")
	requireCode(t, err, CodeUnsupportedVersion)
	os.WriteFile(path, saved, 0644)
	path = filepath.Join(dir, a, "abi.json")
	data, _ = os.ReadFile(path)
	var schema map[string]any
	json.Unmarshal(data, &schema)
	schema["version"] = 99
	data, _ = json.Marshal(schema)
	os.WriteFile(path, data, 0644)
	_, err = e.Execute(a, "F")
	requireCode(t, err, CodeUnsupportedABI)
}
