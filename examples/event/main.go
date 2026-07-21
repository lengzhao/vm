// Package main 事件提取功能演示
package main

import (
	"fmt"
	"log"

	"github.com/lengzhao/vm"
)

func main() {
	fmt.Println("=== 事件提取功能演示 ===")

	config := vm.VMConfig{
		MaxGasLimit:          1000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ContractStorageDir:   "./contracts",
	}

	engine := vm.NewVMEngine(config)

	sourceCode := `
package main

type Context struct{}

func (c *Context) Log(name string, args ...interface{}) {}
func (c *Context) Emit(name string, args ...interface{}) {}
func (c *Context) Event(name string, args ...interface{}) {}

func emit(name string, args ...interface{}) {}
func logEvent(name string, args ...interface{}) {}
func event(name string, args ...interface{}) {}

func SetValue(ctx *Context, value int) {
	ctx.Log("SetValue", "value", value)
	ctx.Emit("SetValue", "value", value)
	ctx.Event("SetValue", "value", value)
	emit("GlobalSetValue", "value", value)
	logEvent("GlobalSetValue", "value", value)
	event("GlobalSetValue", "value", value)
}

func Transfer(ctx *Context, from, to string, amount int) {
	ctx.Log("Transfer", "from", from, "to", to, "amount", amount)
}

func ComplexFunction(ctx *Context) {
	eventName := "DynamicEvent"
	emit(eventName, "param1", "value1", "param2", 42)
	for i := 0; i < 3; i++ {
		ctx.Emit("LoopEvent", "iteration", i)
	}
}
`

	contractABI, err := engine.GenerateABI(sourceCode)
	if err != nil {
		log.Fatalf("ABI生成失败: %v", err)
	}

	fmt.Printf("ABI生成成功:\n%s\n", contractABI.String())
	fmt.Printf("提取到的事件数量: %d\n", len(contractABI.Events))
	for i, ev := range contractABI.Events {
		fmt.Printf("事件 %d: %s\n", i+1, ev.Name)
		for _, param := range ev.Parameters {
			fmt.Printf("  参数: %s (%s)\n", param.Name, param.Type)
		}
	}
}
