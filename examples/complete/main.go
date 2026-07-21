// Package main 完整功能演示
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lengzhao/vm"
)

func main() {
	dir := filepath.Join(os.TempDir(), "lengzhao-vm-complete-example")
	_ = os.RemoveAll(dir)

	config := vm.VMConfig{
		MaxGasLimit:          100000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := vm.NewVMEngine(config)
	fmt.Printf("创建虚拟机: %s\n", engine.String())

	sourceCode := `
package main

import (
	"fmt"
	"strconv"
)

func Add(a, b int) int {
	return a + b
}

func GetBalance() int {
	return 1000
}

func Transfer(from, to string, amount int) bool {
	fmt.Printf("Transfering %d from %s to %s\n", amount, from, to)
	return true
}

func GetUserDetails(id int) (string, int, bool) {
	name := "User" + strconv.Itoa(id)
	balance := 100 * id
	active := true
	return name, balance, active
}
`

	fmt.Println("\n--- 1. 生成ABI ---")
	contractABI, err := engine.GenerateABI(sourceCode)
	if err != nil {
		log.Fatalf("ABI生成失败: %v", err)
	}
	fmt.Printf("%s\n", contractABI.String())

	fmt.Println("\n--- 2. 编译合约 ---")
	compiled, err := engine.Compile(sourceCode)
	if err != nil {
		log.Fatalf("编译失败: %v", err)
	}
	fmt.Printf("编译成功: %s\n", compiled.ExecutablePath)

	fmt.Println("\n--- 3. 部署合约 ---")
	address, err := engine.Deploy(compiled)
	if err != nil {
		log.Fatalf("部署失败: %v", err)
	}
	fmt.Printf("合约地址: %s\n", address)

	fmt.Println("\n--- 4. 查询合约 ---")
	retrieved, err := engine.GetContract(address)
	if err != nil {
		log.Fatalf("获取合约失败: %v", err)
	}
	fmt.Printf("源码哈希: %s\n", retrieved.SourceHash)

	fmt.Println("\n--- 5. 执行合约 ---")
	result, err := engine.Execute(address, "Add", 10, 20)
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("Add => %s, Gas=%d\n", string(result), engine.GetGasConsumed())

	result, err = engine.Execute(address, "GetBalance")
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("GetBalance => %s, Gas=%d\n", string(result), engine.GetGasConsumed())

	result, err = engine.Execute(address, "Transfer", "Alice", "Bob", 100)
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("Transfer => %s, Gas=%d\n", string(result), engine.GetGasConsumed())

	fmt.Println("\n=== 演示完成 ===")
}
