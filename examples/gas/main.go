// Package main 演示 Gas 计费
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
	dir := filepath.Join(os.TempDir(), "lengzhao-vm-gas-example")
	_ = os.RemoveAll(dir)

	config := vm.VMConfig{
		MaxGasLimit:          1000,
		EnableSecurityChecks: true,
		EnableGasMetering:    true,
		ExecutionTimeout:     time.Second * 30,
		ContractStorageDir:   dir,
	}

	engine := vm.NewVMEngine(config)
	fmt.Printf("创建虚拟机: %s\n", engine.String())

	sourceCode := `
package main

func Add(a, b int) int {
	return a + b
}

func GetBalance() int {
	return 1000
}
`

	compiled, err := engine.Compile(sourceCode)
	if err != nil {
		log.Fatalf("编译失败: %v", err)
	}

	address, err := engine.Deploy(compiled)
	if err != nil {
		log.Fatalf("部署失败: %v", err)
	}

	result, err := engine.Execute(address, "Add", 10, 20)
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("Add 结果: %s, Gas: %d\n", string(result), engine.GetGasConsumed())

	result, err = engine.Execute(address, "GetBalance")
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("GetBalance 结果: %s, Gas: %d\n", string(result), engine.GetGasConsumed())
}
