// Package main 事件提取功能演示
package vm

import (
	"fmt"
	"log"

	"github.com/lengzhao/vm"
)

func main() {
	fmt.Println("=== 事件提取功能演示 ===")

	// 创建虚拟机配置
	config := vm.VMConfig{
		MaxGasLimit:          1000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ContractStorageDir:   "./contracts",
	}

	// 创建虚拟机实例
	vm := vm.NewVMEngine(config)

	// 包含事件的智能合约源代码
	sourceCode := `
package main

// Context 模拟上下文
type Context struct{}

// Log 模拟日志函数
func (c *Context) Log(name string, args ...interface{}) {}
func (c *Context) Emit(name string, args ...interface{}) {}
func (c *Context) Event(name string, args ...interface{}) {}

// Global event functions
func emit(name string, args ...interface{}) {}
func log(name string, args ...interface{}) {}
func event(name string, args ...interface{}) {}

func main() {
}

// SetValue 设置值并触发事件
func SetValue(ctx *Context, value int) {
	// 测试各种事件调用方式
	ctx.Log("SetValue", "value", value)
	ctx.Emit("SetValue", "value", value)
	ctx.Event("SetValue", "value", value)
	
	// 测试全局事件函数
	emit("GlobalSetValue", "value", value)
	log("GlobalSetValue", "value", value)
	event("GlobalSetValue", "value", value)
}

// Transfer 转账并触发事件
func Transfer(ctx *Context, from, to string, amount int) {
	ctx.Log("Transfer", "from", from, "to", to, "amount", amount)
}

// ComplexFunction 复杂函数演示多种事件
func ComplexFunction(ctx *Context) {
	eventName := "DynamicEvent"
	emit(eventName, "param1", "value1", "param2", 42)
	
	// 嵌套事件调用
	for i := 0; i < 3; i++ {
		ctx.Emit("LoopEvent", "iteration", i)
	}
}
`

	fmt.Println("\n--- 生成包含事件的ABI ---")
	contractABI, err := vm.GenerateABI(sourceCode)
	if err != nil {
		log.Fatalf("ABI生成失败: %v\n", err)
	}

	fmt.Printf("ABI生成成功:\n%s\n", contractABI.String())
	fmt.Printf("提取到的事件数量: %d\n", len(contractABI.Events))

	// 显示提取到的事件
	for i, event := range contractABI.Events {
		fmt.Printf("事件 %d: %s\n", i+1, event.Name)
		for _, param := range event.Parameters {
			fmt.Printf("  参数: %s (%s)\n", param.Name, param.Type)
		}
	}
}
