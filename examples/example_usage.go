// Package vm 示例代码展示如何使用虚拟机
package vm

import (
	"fmt"
	"time"

	"github.com/lengzhao/vm"
)

// ExampleUsage 演示虚拟机的基本用法
func ExampleUsage() {
	// 创建虚拟机配置
	config := vm.VMConfig{
		MaxGasLimit:          1000000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   "./contracts", // 合约存储目录
	}

	// 创建虚拟机实例
	vm := vm.NewVMEngine(config)
	fmt.Printf("创建虚拟机: %s\n", vm.String())

	// 示例智能合约源代码
	sourceCode := `
package main

import "fmt"

func main() {
	fmt.Println("Hello, Smart Contract World!")
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

	// 编译合约
	fmt.Println("正在编译合约...")
	execPath, err := vm.Compile(sourceCode)
	if err != nil {
		fmt.Printf("编译失败: %v\n", err)
		return
	}
	fmt.Printf("合约编译成功，可执行文件路径: %s\n", execPath)

	// 部署合约
	fmt.Println("正在部署合约...")
	contractAddress, err := vm.Deploy(execPath)
	if err != nil {
		fmt.Printf("部署失败: %v\n", err)
		return
	}
	fmt.Printf("合约部署成功，合约地址: %s\n", contractAddress)

	// 执行合约函数
	fmt.Println("正在执行合约函数...")
	result, err := vm.Execute(contractAddress, "Add", 10, 20)
	if err != nil {
		fmt.Printf("执行失败: %v\n", err)
		return
	}
	fmt.Printf("执行结果: %s\n", string(result))

	// 清理生成的文件
	// 在实际使用中，你可能想要保留这些文件以供后续使用
	fmt.Println("示例执行完成")
}
