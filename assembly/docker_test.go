package assembly_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	_ "github.com/lengzhao/vm/plugins/all"
)

func TestDockerGraphRejectsMutableImages(t *testing.T) {
	cfg := assembly.DefaultConfig(core.VMConfig{})
	cfg.Plugins["runner"] = map[string]any{"use": "runner/docker", "config": map[string]any{"sandbox": map[string]any{"image": "golang:latest"}}}
	if _, err := assembly.Build(context.Background(), cfg); err == nil {
		t.Fatal("mutable sandbox image accepted")
	}
}

func TestDockerSandboxEndToEnd(t *testing.T) {
	image := os.Getenv("VM_DOCKER_IMAGE")
	if image == "" {
		t.Skip("set VM_DOCKER_IMAGE to an immutable local Go image ID")
	}
	version := os.Getenv("VM_DOCKER_GO_VERSION")
	if version == "" {
		t.Fatal("VM_DOCKER_GO_VERSION required")
	}
	cfg := assembly.DefaultConfig(core.VMConfig{ContractStorageDir: t.TempDir(), EnableSecurityChecks: false, EnableGasMetering: true, MaxGasLimit: 1000000, ExecutionTimeout: 30 * time.Second})
	cfg.Plugins["compiler"].(map[string]any)["config"] = map[string]any{"outputDir": t.TempDir(), "goVersion": version, "buildTimeout": "2m", "sandbox": map[string]any{"image": image}}
	cfg.Plugins["runner"] = map[string]any{"use": "runner/docker", "config": map[string]any{"timeout": "20s", "sandbox": map[string]any{"image": image}}}
	cfg.Plugins["validator"].(map[string]any)["config"] = map[string]any{"targetOS": "linux"}
	ctx := context.Background()
	rt, err := assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(ctx)
	source := `package contract
import (
 "github.com/lengzhao/vm/contractapi"
 "os"
 "net"
 "time"
)
func Add(a,b int) int {return a+b}
func Write(v string) error {contractapi.Log("write",map[string]interface{}{"v":v});return contractapi.DefaultObject().Set("value",[]byte(v))}
func Read() string {v,_,_:=contractapi.DefaultObject().Get("value");return string(v)}
func OOGWrite() int {contractapi.DefaultObject().Set("value",[]byte("oog"));for i:=0;i<100;i++{};return 1}
func FailedWrite() error {contractapi.DefaultObject().Set("value",[]byte("bad"));return os.ErrPermission}
func Child(a string) error {_,err:=contractapi.Call(a,"Write","child");return err}
func ChildFailure(a string) error {contractapi.Call(a,"Write","bad-child");return os.ErrPermission}
func Isolation(hostPath string) bool {
 _,readErr:=os.ReadFile(hostPath)
 writeErr:=os.WriteFile("/forbidden",[]byte("x"),0600)
 sock,netErr:=net.DialTimeout("tcp","1.1.1.1:53",time.Second);if sock!=nil {sock.Close()}
 return readErr!=nil && writeErr!=nil && netErr!=nil && os.Getuid()!=0
}
`
	if _, err := rt.Engine.Compile(ctx, "package main\nfunc Invalid() int {return unknown}"); err == nil {
		t.Fatal("invalid sandbox build succeeded")
	}
	c, err := rt.Engine.Compile(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if c.Manifest.Build.Toolchain.GOOS != "linux" || c.Manifest.Build.Toolchain.ContainerImage != image {
		t.Fatal("sandbox identity missing")
	}
	cached, err := rt.Engine.Compile(ctx, source)
	if err != nil || cached.ExecutablePath != c.ExecutablePath {
		t.Fatalf("cache: %v", err)
	}
	deploy := func(n uint64) string {
		a, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "sandbox", Sender: "test", Nonce: n}})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	a, b := deploy(1), deploy(2)
	call := func(address, fn string, args ...interface{}) *core.ExecuteResult {
		t.Helper()
		r, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: address, Function: fn, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if string(call(a, "Add", 2, 3).Data) != "5" {
		t.Fatal("wrong result")
	}
	sentinel := t.TempDir() + "/secret"
	os.WriteFile(sentinel, []byte("secret"), 0600)
	if string(call(a, "Isolation", sentinel).Data) != "true" {
		t.Fatal("filesystem/network isolation failed")
	}
	if len(call(a, "Write", "good").Events) != 1 {
		t.Fatal("event lost")
	}
	if _, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "FailedWrite"}); err == nil {
		t.Fatal("failure succeeded")
	}
	if string(call(a, "Read").Data) != `"good"` {
		t.Fatal("failed execution committed")
	}
	if _, err := rt.Engine.EstimateGas(ctx, a, "Write", &core.EstimateOptions{DryRun: true, Args: []any{"dry"}}); err != nil {
		t.Fatal(err)
	}
	if string(call(a, "Read").Data) != `"good"` {
		t.Fatal("DryRun committed")
	}
	_, err = rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "OOGWrite", CallContext: &core.CallContext{GasLimit: core.Uint64Ptr(40)}})
	var executionError *core.ExecutionError
	if !errors.As(err, &executionError) || executionError.Code != core.CodeGasLimitExceeded {
		t.Fatalf("expected OOG: %v", err)
	}
	if string(call(a, "Read").Data) != `"good"` {
		t.Fatal("OOG committed")
	}
	call(a, "Child", b)
	if string(call(b, "Read").Data) != `"child"` {
		t.Fatal("child state missing")
	}
	if _, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: a, Function: "ChildFailure", Args: []interface{}{b}}); err == nil {
		t.Fatal("parent failure succeeded")
	}
	if string(call(b, "Read").Data) != `"child"` {
		t.Fatal("child state escaped rollback")
	}
	data, _ := json.Marshal(c.Manifest)
	if !strings.Contains(string(data), image) {
		t.Fatal("manifest image missing")
	}
}

func TestDockerCleanupFailureRollsBackState(t *testing.T) {
	ctx := context.Background()
	cli := t.TempDir() + "/docker"
	script := `#!/bin/sh
case "$1" in
create) exit 0;;
start)
read request
echo '{"version":3,"type":"object","op":"set","key":"x","value":"eQ==","gas":20}'
read reply
echo '{"version":3,"ok":true,"gas":20,"result":7,"events":[{"name":"changed"}]}'
;;
rm) echo 'cleanup unavailable' >&2; exit 1;;
esac
`
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := assembly.DefaultConfig(core.VMConfig{EnableGasMetering: true, MaxGasLimit: 1000, ContractStorageDir: t.TempDir()})
	cfg.Plugins["runner"] = map[string]any{"use": "runner/docker", "config": map[string]any{"sandbox": map[string]any{"binary": cli, "image": "sha256:" + strings.Repeat("a", 64)}}}
	cfg.Plugins["validator"].(map[string]any)["config"] = map[string]any{"targetOS": "linux"}
	rt, err := assembly.Build(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(ctx)
	c := artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion})
	build := c.Manifest.Build
	build.Toolchain.GOOS = "linux"
	if err := artifact.Seal(ctx, c, build); err != nil {
		t.Fatal(err)
	}
	address, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test", Sender: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := rt.Engine.Execute(ctx, core.ExecutionRequest{Address: address, Function: "Set"})
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("cleanup failure lost: %v", err)
	}
	if r != nil && (len(r.Events) > 0 || len(r.Data) > 0) {
		t.Fatal("failure published data/events")
	}
	// Read the same state plugin that Engine consumed, rather than a separate store.
	var store core.StateStore
	for _, instance := range rt.Result.Instances {
		if s, ok := instance.Value.(core.StateStore); ok {
			store = s
		}
	}
	if store == nil {
		t.Fatal("state plugin not found")
	}
	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, ok, err := tx.Get(ctx, core.StateKey{Contract: core.Address(address), Key: "x"}); err != nil || ok {
		t.Fatalf("cleanup failure committed: exists=%v err=%v", ok, err)
	}
}

func TestDockerPresetBuildsWithoutStartingResources(t *testing.T) {
	data, err := os.ReadFile("../presets/docker.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config := strings.ReplaceAll(string(data), "IMAGE_ID", "sha256:"+strings.Repeat("a", 64))
	config = strings.ReplaceAll(config, "GO_VERSION", "go1.26.9")
	rt, err := assembly.Load(context.Background(), strings.NewReader(config))
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
