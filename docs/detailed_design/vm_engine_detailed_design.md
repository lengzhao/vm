# 虚拟机执行引擎详细设计

## 1. 职责与契约

`internal/engine` 编排业务能力，插件工厂位于 `plugins/engine`。`core.Engine` 组合 `ContractCompiler`、`ContractDeployer`、`ContractReader`、`Executor`、`ABIService` 和 `GasEstimationService`，不要求兼容查询、配置诊断或生命周期方法。

```go
type Executor interface {
    Execute(context.Context, ExecutionRequest) (*ExecuteResult, error)
}
type ExecutionRequest struct {
    Address string
    Function string
    Args []any
    CallContext *CallContext
}
```

根包 `VMEngine` 和 `internal/compat` 负责旧签名、最近调用状态和 getter。根包 `Stop() error` 委托 Runtime；`assembly.Runtime.Stop(ctx)` 统一处理图中实例可选的 `Stop(context.Context) error`。

## 2. 显式依赖

```go
type Deps struct {
    Policy core.SourcePolicy
    ABI core.ABIGenerator
    Compiler core.ContractCompiler
    Deployer core.ContractDeployer
    Contracts core.ContractReader
    Runner core.Runner
    Meters core.GasMeterFactory
    Estimator core.GasEstimator
    Validator core.ArtifactValidator
}
```

引擎不构造默认子模块，不承担文件存储和地址生成。画像构建由编译器依赖的 `GasProfileBuilder` 完成；同一个 `GasProfiler` 插件可同时提供画像和估算能力。

## 3. 编排流程

- `Compile(ctx, source)`：向编译器传递调用方上下文。
- `GenerateABI(ctx, source)`：源码策略审查后生成 ABI。
- `Deploy(ctx, artifact)`：委托部署插件。部署插件校验产物策略，分配地址后调用存储的 Put；不修改调用方产物。
- `Get(ctx, address)`：委托只读存储能力，返回完整产物。
- `Execute(ctx, request)`：读取产物，调用 ArtifactValidator，解析 Host 为独立值快照，创建本次 Gas 计量器，向 Runner 传入派生超时上下文，返回 Data/Events/GasConsumed。
- `EstimateGas(ctx, address, function, opts)`：读取并校验产物，委托 GasEstimator；可选 DryRun 使用同一执行路径。

事件不会回放到 `Host.Log`。最大 Gas 是硬上限，调用上下文只能进一步收紧；并发调用使用各自的 ExecuteResult。

## 4. 策略与配置

源码策略身份与保证分开。`PolicyGuarantor` 可声明确定性保证，自定义策略不需要使用内置 ID。`policy/artifact` 单独负责接受的策略 ID 和所需保证，部署和执行共享该校验契约。

内建引擎配置仅包含 MaxGasLimit、EnableGasMetering 和 ExecutionTimeout。安全、编译、存储及策略参数由各自插件配置。完整图与默认预设见 [插件指南](../plugins.md)。

## 5. 当前限制

ProcessRunner 提供进程、超时、协议和输出限制；完整 OS 沙箱、链状态读写、跨合约调用与升级仍为后续能力。静态 Gas 估算为启发式值。
