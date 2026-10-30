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
import "github.com/lengzhao/vm/contractapi"
const TransferEvent = "Transfer"
func SetValue(value int64) {
 contractapi.Log("SetValue","value",value)
}
func Transfer(from,to string,amount uint64) {
 contractapi.Log(TransferEvent,"from",from,"to",to,"amount",amount)
}
func DynamicEvent(name string) {
 // 动态事件名不会被静态 ABI 声明；运行时结果才是完整事件来源。
 contractapi.Log(name,"value",1)
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
