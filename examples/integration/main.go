// Run with presets/objects.yaml for trusted development, or with a configured
// presets/objects-docker.yaml for isolated build and execution.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	_ "github.com/lengzhao/vm/plugins/all"
	"gopkg.in/yaml.v3"
)

const source = `package main
import "github.com/lengzhao/vm/contractapi"
func Create(id string) error {
 o,e:=contractapi.CreateObject(id);if e!=nil{return e}
 if e=o.Set("value",[]byte("hello"));e!=nil{return e}
 contractapi.Log("Created","id",id);return nil
}
func Read(id string)(string,error){v,_,e:=contractapi.OpenObject(id).Get("value");return string(v),e}
`

func main() {
	path := "presets/objects.yaml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	data, err := os.ReadFile(path)
	var cfg assembly.Config
	if err == nil {
		err = yaml.Unmarshal(data, &cfg)
	}
	if err == nil {
		err = run(context.Background(), cfg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func stop(rt *assembly.Runtime) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return rt.Stop(ctx)
}
func run(ctx context.Context, cfg assembly.Config) (err error) {
	rt, err := assembly.Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, stop(rt)) }()
	c, err := rt.Engine.Compile(ctx, source)
	if err != nil {
		return err
	}
	address, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "integration-demo", Sender: "alice", Nonce: 0}})
	if err != nil {
		return err
	}
	// This demo declares a trusted identity. A real host verifies signatures and
	// nonce BEFORE constructing CallContext; the VM does not authenticate alice.
	sender := core.Address("alice")
	cc := &core.CallContext{Sender: &sender}
	request := core.RecoverableBatchRequest{ID: "demo-" + address, Calls: []core.ExecutionRequest{
		{Address: address, Function: "Create", Args: []any{"greeting"}, CallContext: cc},
		{Address: address, Function: "Read", Args: []any{"greeting"}, CallContext: cc},
	}}
	batch, ok := rt.Engine.(core.RecoverableBatchExecutor)
	if !ok {
		return fmt.Errorf("preset must provide RecoverableBatchExecutor")
	}
	result, err := batch.ExecuteRecoverableBatch(ctx, request)
	if err != nil {
		return err
	}
	if err = validate(result); err != nil {
		return err
	}
	// Preserve the complete request and artifacts alongside the database in a real
	// application. Receipt events are deduplicated by batch ID + item + event index.
	for i, item := range result {
		for j, event := range item.Result.Events {
			fmt.Printf("event key=%s/%d/%d name=%s\n", request.ID, i, j, event.Name)
		}
	}
	if err = stop(rt); err != nil {
		return err
	}
	reopened, err := assembly.Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, stop(reopened)) }()
	replay, err := reopened.Engine.(core.RecoverableBatchExecutor).ExecuteRecoverableBatch(ctx, request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(result, replay) {
		return fmt.Errorf("replayed receipts differ")
	}
	fmt.Printf("read=%s; reopened and replayed %d receipts\n", result[1].Result.Data, len(replay))
	return nil
}
func validate(items []core.BatchItemResult) error {
	if len(items) != 2 {
		return fmt.Errorf("expected two receipts")
	}
	for i, item := range items {
		if item.Err != nil {
			return fmt.Errorf("item %d: %w", i, item.Err)
		}
		if item.Result == nil {
			return fmt.Errorf("item %d has no result", i)
		}
	}
	if string(items[1].Result.Data) != `"hello"` {
		return fmt.Errorf("unexpected read: %s", items[1].Result.Data)
	}
	return nil
}
