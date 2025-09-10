// Package main 完整示例展示所有已实现的功能
package vm

import (
	"fmt"
	"log"
	"time"

	"github.com/lengzhao/vm"
)

// CompleteExample 演示所有已实现的功能
func CompleteExample() {
	fmt.Println("=== 智能合约虚拟机完整功能演示 ===")

	// 创建虚拟机配置
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

import (
	"fmt"
	"strconv"
)

func main() {
	fmt.Println("完整功能演示合约")
}

// Add 两个数相加
func Add(a, b int) int {
	return a + b
}

// GetBalance 获取余额
func GetBalance() int {
	return 1000
}

// Transfer 转账
func Transfer(from, to string, amount int) (bool, error) {
	fmt.Printf("Transfering %d from %s to %s\n", amount, from, to)
	return true, nil
}

// GetUserDetails 获取用户详情
func GetUserDetails(id int) (string, int, bool) {
	name := "User" + strconv.Itoa(id)
	balance := 100 * id
	active := true
	return name, balance, active
}
`

	fmt.Println("\n--- 1. 生成ABI ---")
	contractABI, err := vm.GenerateABI(sourceCode)
	if err != nil {
		log.Fatalf("ABI生成失败: %v\n", err)
	}

	fmt.Printf("ABI生成成功:\n%s\n", contractABI.String())

	fmt.Println("\n--- 2. 编译合约 ---")
	compiledContract, err := vm.Compile(sourceCode)
	if err != nil {
		log.Fatalf("编译失败: %v\n", err)
	}
	fmt.Printf("合约编译成功\n")

	fmt.Println("\n--- 3. 部署合约 ---")
	contractAddress, err := vm.Deploy(compiledContract)
	if err != nil {
		log.Fatalf("部署失败: %v\n", err)
	}
	fmt.Printf("合约部署成功，合约地址: %s\n", contractAddress)

	fmt.Println("\n--- 4. 查询合约信息 ---")
	// 获取合约
	retrievedContract, err := vm.GetContract(contractAddress)
	if err != nil {
		log.Fatalf("获取合约失败: %v\n", err)
	}
	fmt.Printf("合约获取成功，源码哈希: %s\n", retrievedContract.SourceHash)

	// 获取合约ABI
	retrievedABI, err := vm.GetContractABI(contractAddress)
	if err != nil {
		log.Fatalf("获取合约ABI失败: %v\n", err)
	}
	fmt.Printf("合约ABI获取成功，函数数量: %d\n", len(retrievedABI.Functions))

	fmt.Println("\n--- 5. 执行合约函数 ---")
	// 执行Add函数
	fmt.Println("执行Add函数 (10 + 20)...")
	result, err := vm.Execute(contractAddress, "Add", 10, 20)
	if err != nil {
		log.Fatalf("执行失败: %v\n", err)
	}
	fmt.Printf("执行结果: %s\n", string(result))
	fmt.Printf("本次执行消耗的Gas: %d\n", vm.GetGasConsumed())

	// 执行GetBalance函数
	fmt.Println("\n执行GetBalance函数...")
	result, err = vm.Execute(contractAddress, "GetBalance")
	if err != nil {
		log.Fatalf("执行失败: %v\n", err)
	}
	fmt.Printf("执行结果: %s\n", string(result))
	fmt.Printf("本次执行消耗的Gas: %d\n", vm.GetGasConsumed())

	// 执行Transfer函数
	fmt.Println("\n执行Transfer函数...")
	result, err = vm.Execute(contractAddress, "Transfer", "Alice", "Bob", 100)
	if err != nil {
		log.Fatalf("执行失败: %v\n", err)
	}
	fmt.Printf("执行结果: %s\n", string(result))
	fmt.Printf("本次执行消耗的Gas: %d\n", vm.GetGasConsumed())

	fmt.Println("\n=== 演示完成 ===")
}

func main() {
	CompleteExample()
}
