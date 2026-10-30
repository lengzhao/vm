// Package main 演示虚拟机基本用法
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
	dir := filepath.Join(os.TempDir(), "lengzhao-vm-example-usage")
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

	// 合约源码不要包含 main，由编译器生成入口
	sourceCode := `
package main

func Add(a, b int) int {
	return a + b
}

func GetBalance() int {
	return 1000
}
`

	fmt.Println("正在编译合约...")
	compiled, err := engine.Compile(sourceCode)
	if err != nil {
		log.Fatalf("编译失败: %v", err)
	}
	fmt.Printf("合约编译成功: %s\n", compiled.ExecutablePath)

	fmt.Println("正在部署合约...")
	address, err := engine.Deploy(compiled)
	if err != nil {
		log.Fatalf("部署失败: %v", err)
	}
	fmt.Printf("合约部署成功: %s\n", address)

	fmt.Println("正在执行 Add(10, 20)...")
	result, err := engine.Execute(address, "Add", 10, 20)
	if err != nil {
		log.Fatalf("执行失败: %v", err)
	}
	fmt.Printf("执行结果: %s\n", string(result))
	fmt.Printf("Gas 消耗: %d\n", engine.GetGasConsumed())
}
