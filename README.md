# 智能合约虚拟机 (vm)

这是一个基于Golang的智能合约虚拟机。智能合约是Golang源代码，而不是二进制程序，并且对一些关键字和导入库进行了限制以增强安全性。

## 特性

- **源码即合约**：智能合约本身就是Golang代码
- **安全优先**：通过关键字/导入白名单与 AST 审查确保执行安全性
- **开发友好**：降低智能合约开发门槛，让熟悉Go语言的开发者无缝接入
- **可执行流水线**：Compile → Deploy → Execute 已打通（ProcessRunner）
- **Gas计费**：入口/函数/循环控制点 + 默认库（只读/Log）计费；`EstimateGas` 静态上界与 DryRun
- **并行执行（设计方向）**：通过对象隔离支持交易并行，尚未实现

> 注意：当前为 Alpha/MVP。ProcessRunner 仅有进程超时与 Gas 限制，**不应用于执行不可信合约**。见 [docs/sandbox_roadmap.md](docs/sandbox_roadmap.md)。

## 已实现功能

### 核心模块
1. **安全审查模块**：Import 白名单、危险 AST 节点、包级可变状态检查
2. **ABI生成模块**：从合约源代码提取接口信息
3. **编译器模块**：Gas 注入、入口生成、`go build` 产出真实可执行文件
4. **Runner**：进程超时执行合约产物，JSON 请求/响应
5. **Gas计费模块**：宿主计量 + `contractapi` 统一扣费 + `gas_profile.json` / `EstimateGas`
6. **合约管理模块**：部署、存储和查询合约产物（含 GasProfile）
7. **Host Runtime / contractapi**：只读链上下文与事件日志（MVP）

### 安全机制
- 关键字白名单机制：限制危险关键字的使用
- 导入白名单机制：只允许安全的包导入
- AST解析和分析：静态分析合约代码
- Gas计费机制：防止资源滥用

### API接口
- `NewVMEngine(config VMConfig) *VMEngine`：创建虚拟机实例
- `Compile(sourceCode string) (*CompiledContract, error)`：编译合约源代码
- `GenerateABI(sourceCode string) (*abi.ABI, error)`：生成合约ABI
- `Deploy(contract *CompiledContract) (string, error)`：部署合约
- `Execute(contractAddress, function string, args ...interface{}) ([]byte, error)`：执行合约函数
- `ExecuteWithContext(..., callCtx *CallContext, ...) (*ExecuteResult, error)`：带 Host 上下文执行
- `EstimateGas(address, function string, opts *EstimateOptions) (*GasEstimate, error)`：静态保守估算或 DryRun
- `GetLastEvents() []Event`：获取最近一次执行事件
- `GetContract(address string) (*CompiledContract, error)`：获取合约信息
- `GetContractABI(address string) (*abi.ABI, error)`：获取合约ABI

## 快速开始

### 安装
```bash
go get github.com/lengzhao/vm
```

### 基本用法
```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/lengzhao/vm"
)

func main() {
    // 创建虚拟机配置
    config := vm.VMConfig{
        MaxGasLimit:          1000000,
        EnableSecurityChecks: true,
        EnableGasMetering:    true,
        ExecutionTimeout:     time.Second * 30,
        ContractStorageDir:   "./contracts",
    }

    // 创建虚拟机实例
    vm := vm.NewVMEngine(config)

    // 合约源代码（不要写 main，由编译器生成入口）
    sourceCode := `
package main

func Add(a, b int) int {
    return a + b
}
`

    // 生成ABI
    contractABI, err := vm.GenerateABI(sourceCode)
    if err != nil {
        log.Fatalf("ABI生成失败: %v", err)
    }
    fmt.Printf("ABI: %s\n", contractABI.String())

    // 编译合约
    compiledContract, err := vm.Compile(sourceCode)
    if err != nil {
        log.Fatalf("编译失败: %v", err)
    }

    // 部署合约
    contractAddress, err := vm.Deploy(compiledContract)
    if err != nil {
        log.Fatalf("部署失败: %v", err)
    }

    // 执行合约函数
    result, err := vm.Execute(contractAddress, "Add", 10, 20)
    if err != nil {
        log.Fatalf("执行失败: %v", err)
    }
    fmt.Printf("执行结果: %s\n", string(result))
    
    // 检查Gas消耗
    fmt.Printf("Gas消耗: %d\n", vm.GetGasConsumed())
}
```

## 文档

### 架构设计
- [架构设计](docs/architecture.md)
- [模块化架构设计](docs/modular_architecture_design.md)

### 核心模块详细设计
- [详细设计总览](docs/detailed_design/detailed_design_overview.md)
- [虚拟机执行引擎](docs/detailed_design/vm_engine_detailed_design.md)
- [安全审查系统](docs/detailed_design/security_review_detailed_design.md)
- [编译器模块](docs/detailed_design/compiler_detailed_design.md)
- [执行环境](docs/detailed_design/execution_environment_detailed_design.md)
- [Gas计费系统](docs/detailed_design/gas_metering_detailed_design.md)
- [ABI生成器](docs/detailed_design/abi_generator_detailed_design.md)
- [合约管理模块](docs/detailed_design/contract_manager_detailed_design.md)

### 流程文档
- [合约处理流程](docs/detailed_design/contract_processing_flow.md)

### 规范文档
- [安全审查规范](docs/security_review.md)
- [默认库接口规范](docs/default_library.md)
- [Host Runtime 设计](docs/host_runtime.md)
- [沙箱路线图](docs/sandbox_roadmap.md)
- [执行环境设计](docs/execution_environment.md)
- [Gas计费机制](docs/gas_metering.md)
- [ABI生成与关键字处理](docs/abi_generation.md)

## 开发计划

查看 [todo.md](todo.md) 了解详细的开发计划和进度。

### 使用示例

```bash
go run ./examples/basic
go run ./examples/complete
```

## 测试

运行所有测试：
```bash
go test ./...
```

## 构建

```bash
go build .
```

## 许可证

[MIT License](LICENSE)