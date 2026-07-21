// Package main 高级用法演示
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
	dir := filepath.Join(os.TempDir(), "lengzhao-vm-advanced-example")
	_ = os.RemoveAll(dir)

	config := vm.VMConfig{
		MaxGasLimit:          1000000,
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

	contractABI, err := engine.GenerateABI(sourceCode)
	if err != nil {
		log.Fatalf("ABI生成失败: %v", err)
	}
	fmt.Printf("ABI:\n%s\n", contractABI.String())

	compiled, err := engine.Compile(sourceCode)
	if err != nil {
		log.Fatalf("编译失败: %v", err)
	}
	fmt.Printf("编译成功: %s\n", compiled.ExecutablePath)

	address, err := engine.Deploy(compiled)
	if err != nil {
		log.Fatalf("部署失败: %v", err)
	}
	fmt.Printf("合约地址: %s\n", address)

	result, err := engine.Execute(address, "Add", 10, 20)
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("Add 结果: %s\n", string(result))

	result, err = engine.Execute(address, "GetUserDetails", 7)
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("GetUserDetails 结果: %s\n", string(result))
}
