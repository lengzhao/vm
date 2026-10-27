# 智能合约虚拟机 (vm)

这是一个基于Golang的智能合约虚拟机。智能合约是Golang源代码，而不是二进制程序，并且对一些关键字和导入库进行了限制以增强安全性。

## 特性

- **源码即合约**：智能合约本身就是Golang代码
- **安全优先**：通过关键字/导入白名单与 AST 审查确保执行安全性
- **开发友好**：降低智能合约开发门槛，让熟悉Go语言的开发者无缝接入
- **可执行流水线**：Compile → Deploy → Execute 已打通（ProcessRunner）
- **Gas计费**：入口/函数/循环控制点 + 默认库（只读/Log）计费；`EstimateGas` 静态启发式估算与 DryRun
- **可选有序并行执行**：MVCC 按键冲突检测、推测执行与按序提交；需满足独占状态写入等确定性前提

> 注意：当前为 Alpha/MVP。ProcessRunner 仅有进程超时与 Gas 限制，**不应用于执行不可信合约**。可选 Docker 编译与执行沙箱已实现，需显式配置；见 [沙箱配置与边界](docs/sandbox_roadmap.md)。

## 已实现功能

### 核心模块
1. **安全审查模块**：Import 白名单、危险 AST 节点、包级可变状态检查
2. **ABI生成模块**：从合约源代码提取接口信息
3. **编译器模块**：Gas 注入、入口生成、`go build` 产出真实可执行文件
4. **Runner**：Process / Docker / WASI-wazero 插件化执行，复用 JSON / Host 协议；Docker 支持编译隔离和 OS 配额，WASI 支持无默认宿主权限的进程内运行
5. **Gas计费模块**：宿主计量 + `contractapi` 统一扣费 + `gas_profile.json` / `EstimateGas`
6. **合约管理模块**：部署、存储和查询合约产物（含 GasProfile）
7. **Host Runtime / contractapi**：只读链上下文与事件日志（MVP）
8. **状态事务基础**：可替换的 `state/memory` 插件，支持快照、读写集合及原子提交/回滚；默认 Object 已接入，失败/OOG/DryRun 无提交，见 [状态事务](docs/state_transactions.md)

9. **跨合约调用**：共享临时 store 与 Gas、整链成功原子提交、禁止重入与深度限制，见 [跨合约调用](docs/cross_contract.md)

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
- [沙箱配置与边界](docs/sandbox_roadmap.md)
- [后端实测比较（D2）](docs/backend_comparison.md)
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
make check
```

`make check` 顺序执行普通测试、race、vet 和差异检查；`make test` 仅运行普通测试。真实 Docker 隔离测试需按沙箱文档显式配置环境。

## 构建

```bash
make build
```

项目为 Go 库，`make build` 校验全部包；`make run` 运行 basic 示例。跨平台校验可执行 `make build-linux TARGET_ARCH=arm64`。

## 许可证

[MIT License](LICENSE)

## 运行边界与接口约束（架构加固）

- 合约可访问 `contractapi` 的 `BlockHeight`、`BlockTime`、`Sender`、`ContractAddress`、`Log`。Gas 初始化/重置、事件清理和计量属于运行时控制，不允许在合约源码中引用（包括别名、函数值和点导入）。
- 保留 `__vm` 前缀；合约不允许定义 `init` 或使用 `recover`。这些完整性检查即使关闭可选安全审查仍执行。关闭安全审查不提供不可信代码隔离。
- 函数声明、匿名函数和循环体均插入 Gas 控制点；入口超限返回结构化失败。
- 每次调用独立计量和解析 Host 上下文。配置的 `MaxGasLimit` 是上限，调用上下文只能收紧；零表示无限仅在未配置引擎上限时生效。
- `ExecuteResult` 是调用结果的依据。`GetGasConsumed` / `GetLastEvents` 仅表示最近结束的调用，不能用于关联并发请求。
- 返回值与事件中的通用 JSON 数值使用 `json.Number`，保留 int64/uint64 精度；使用者应通过 `Int64` / `strconv.ParseUint` 等精确解析。响应只允许一个 JSON 值。
- ProcessRunner 的 stdout、stderr 各限制为 1 MiB，超限取消直接进程；管道收尾最多额外等待 100ms。此限制不等同于进程树、内存、文件或网络隔离。
- 构建使用独立临时目录，二进制与版本化清单一起原子发布；实际工具链探测与构建共用默认 30s 超时。缓存指纹覆盖实际工具链、平台、生成代码与元数据，缓存、部署、加载和执行前校验内容摘要。详见 [产物清单](docs/artifact_manifest.md)。
- `NewVMEngineWithRunner(config, runner)` 可注入执行后端，传入 nil 等价于现有 `NewVMEngine`。自定义后端需要遵守同样的数值、Gas、事件与资源契约。


## 可选确定性源码策略

设置 `VMConfig.RequireDeterminism=true` 启用 `go-deterministic-v3-call`。它强制安全审查，禁止机器时间、浮点/指针观测和直接输出，仅开放受限值计算及链上下文 API；严格模式拒绝没有匹配策略标签的旧产物。默认使用开发模式，确定性策略需显式启用。

详见 [确定性源码策略](docs/deterministic_execution.md)。该策略不等同于完整沙箱或完整共识执行规范；工具链固定、产物摘要及[确定性部署地址](docs/deployment.md)已实现；状态事务、持久化与恢复批次已实现；来源认证仍由集成宿主负责。


## ABI 与调用协议

当前仅支持 ABI schema v1 / 调用协议 v3，不兼容无版本旧产物；升级后重新 Compile / Deploy。统一支持整数、布尔、字符串与开发模式浮点标量，末位 error 表示调用失败。参数严格匹配 JSON 类型及数量；错误通过 ExecutionError.Code 分类。

详见 [ABI 与调用协议 v3](docs/call_protocol.md)。

## PluginKit 插件化

VM 的编译、安全策略、产物接受策略、ABI、Gas、部署、存储、执行后端及引擎编排已通过 PluginKit 装配，宿主项目使用 Go 1.26.1。
现有 `vm.NewVMEngine` API 和以上示例保留，由兼容层适配。新项目可使用 `assembly.Load` / `assembly.Build` 配置依赖图，通过 `rt.Engine.Compile(ctx, source)` 和 `rt.Engine.Execute(ctx, core.ExecutionRequest{...})` 调用业务能力，执行返回包含事件和 Gas 的 `ExecuteResult`。

核心接口按职责收敛：编译器仅提供 Compile，安全审查仅提供 Review，存储 Put/Get 与部署分离。源码策略通过可选 Guarantees 声明保证，自定义确定性策略不绑定内置 ID；`policy/artifact` 校验产物身份与保证。Gas 画像构建和估算提供窄视图，由同一个 Profiler 实现配对。

内建实现按能力拆分到独立 internal 包，internal/compat 适配旧接口，internal/builtin 仅保留历史回归测试。core.Engine 只组合业务接口，不包含 Stop 或最近结果 getter；通过 `rt.Stop(ctx)` 管理实例可选的 `Stop(context.Context) error`，根包旧 `Stop() error` 保留。

```bash
go run ./examples/plugins presets/default.yaml
```

参见 [插件架构与扩展指南](docs/plugins.md) 和 [默认预设](presets/default.yaml)。


可选批量执行：`state/mvcc` + `batch/ordered` 支持有界推测执行、按输入顺序提交与冲突重执行；现有默认配置不变。配置见 [`presets/parallel.yaml`](presets/parallel.yaml)，确定性前提、错误及已提交前缀语义见 [有序并行执行](docs/parallel_execution.md)。


可选持久化状态：`state/bbolt` 支持重启恢复、同步原子提交及 D3 批量执行；默认内存后端不变。配置见 [`presets/persistent.yaml`](presets/persistent.yaml)，生命周期及提交不确定结果的处理见 [持久化状态](docs/persistent_state.md)。


可选可恢复批次：`batch/recoverable` + `state/bbolt` 提供共享独占会话、状态与回执原子提交、同 ID 续跑和重放；默认及旧批量接口不变。配置见 [`presets/recoverable.yaml`](presets/recoverable.yaml)，身份、未完成批次与事件去重要求见 [可恢复批次](docs/recoverable_batches.md)。

可选多对象与 owner 授权：见 [对象模型](docs/objects.md) 和 [配置](presets/objects.yaml)；根 Sender 认证由宿主负责。
