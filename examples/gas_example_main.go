// Package main 示例代码展示Gas计费功能
package vm

import (
	"fmt"
	"log"
	"time"

	"github.com/lengzhao/vm"
)

// GasExample 演示Gas计费功能
func ExampleGasExample() {
	// 创建虚拟机配置，启用Gas计费
	config := vm.VMConfig{
		MaxGasLimit:          1000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   "./contracts",
	}

	// 创建虚拟机实例
	vm := vm.NewVMEngine(config)
	fmt.Printf("创建虚拟机: %s\n", vm.String())

	// 示例智能合约源代码
	sourceCode := `
package main

import "fmt"

func main() {
	fmt.Println("Gas计费示例合约")
}

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}

// GetBalance 获取余额
func GetBalance() int {
	return 1000
}
`

	// 生成ABI
	fmt.Println("正在生成ABI...")
	contractABI, err := vm.GenerateABI(sourceCode)
	if err != nil {
		log.Fatalf("ABI生成失败: %v\n", err)
	}

	fmt.Printf("ABI生成成功:\n%s\n", contractABI.String())

	// 编译合约
	fmt.Println("正在编译合约...")
	execPath, err := vm.Compile(sourceCode)
	if err != nil {
		log.Fatalf("编译失败: %v\n", err)
	}
	fmt.Printf("合约编译成功，可执行文件路径: %s\n", execPath)

	// 部署合约
	fmt.Println("正在部署合约...")
	contractAddress, err := vm.Deploy(execPath)
	if err != nil {
		log.Fatalf("部署失败: %v\n", err)
	}
	fmt.Printf("合约部署成功，合约地址: %s\n", contractAddress)

	// 执行合约函数
	fmt.Println("正在执行合约函数...")
	result, err := vm.Execute(contractAddress, "Add", 10, 20)
	if err != nil {
		log.Fatalf("执行失败: %v\n", err)
	}
	fmt.Printf("执行结果: %s\n", string(result))

	// 检查Gas消耗
	fmt.Printf("本次执行消耗的Gas: %d\n", vm.GetGasConsumed())

	// 再次执行另一个函数
	fmt.Println("正在执行另一个合约函数...")
	result, err = vm.Execute(contractAddress, "GetBalance")
	if err != nil {
		log.Fatalf("执行失败: %v\n", err)
	}
	fmt.Printf("执行结果: %s\n", string(result))

	// 检查Gas消耗
	fmt.Printf("本次执行消耗的Gas: %d\n", vm.GetGasConsumed())

	fmt.Println("Gas计费示例执行完成")
}
