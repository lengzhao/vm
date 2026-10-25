// A configured VM assembled entirely through PluginKit.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/lengzhao/vm/assembly"
	"github.com/lengzhao/vm/core"
	_ "github.com/lengzhao/vm/plugins/all"
)

func main() {
	path := "presets/default.yaml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	rt, err := assembly.Load(context.Background(), file)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := rt.Stop(context.Background()); err != nil {
			log.Print(err)
		}
	}()
	if err := run(rt); err != nil {
		log.Print(err)
	}
}
func run(rt *assembly.Runtime) error {
	c, err := rt.Engine.Compile(context.Background(), "package main\nfunc Add(a,b int)int{return a+b}")
	if err != nil {
		return err
	}
	a, err := rt.Engine.Deploy(context.Background(), core.DeploymentRequest{Contract: c, Input: core.DeploymentInput{ChainID: "test-chain", Sender: "alice"}})
	if err != nil {
		return err
	}
	result, err := rt.Engine.Execute(context.Background(), core.ExecutionRequest{Address: a, Function: "Add", Args: []any{2, 3}})
	if err != nil {
		return err
	}
	fmt.Println(string(result.Data))
	return nil
}
