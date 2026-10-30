# Gas计费与资源控制设计文档

## 1. 概述

本文档描述智能合约虚拟机中的 Gas 计费与资源控制。目标是防止合约执行消耗过多系统资源，保证网络稳定与公平。

## 2. Gas计费原理

### 2.1 编译期注入（已实现）

编译阶段在以下位置插入 `contractapi.ConsumeGas`：

- 每个导出/内部函数及匿名函数入口（1 Gas）
- `for` / `range` 循环体开头（每次迭代 1 Gas）
- 执行入口固定基础 Gas（`contractapi.InitGas` + `ConsumeGas(10)`）

合约进程通过环境变量 `VM_GAS_LIMIT` 接收上限，JSON 响应中回报 `gas`（即 `contractapi.GasUsed()`）。

Deploy 同时落盘 `gas_profile.json`，供静态 `EstimateGas` 使用。

### 2.2 默认库接口计费（已实现）

只读上下文、Log、默认 Object、多对象创建/查询 owner/转移/销毁、Call 均有运行时扣费。宿主校验对象/跨合约的最低费用。余额和资产转账接口尚未提供。

### 2.3 历史“按行计费”说明

早期方案按代码行计费。当前为控制点计费，语义更稳定、更易测试。行级模型视为可选精细化方案，不作为当前实现约束。

## 3. Gas消耗模型

### 3.1 控制点（已实现）

| 操作类型 | Gas消耗 |
|---------|--------|
| 执行入口基础消耗 | 10 |
| 函数/匿名函数入口 | 1 |
| for / range 每次迭代 | 1 |

### 3.2 目标态基础操作（后置精细化，非当前实现）

| 操作类型 | Gas消耗 |
|---------|--------|
| 代码行执行 | 1 |
| 变量赋值 | 1 |
| 基本算术运算 | 1 |
| 条件判断 | 1 |
| 函数调用开销 | 5 |

### 3.3 当前默认库价格

- BlockHeight / BlockTime / ContractAddress / Sender：各 1。
- Log：2。
- Object.Get / Owner：5；Set：10；Delete：5。
- CreateObject：20；Object.Transfer / Destroy：各 10。
- Call：10 加实际子树消耗；根调用只计一次子树 Gas。

静态画像覆盖以上直接调用；对象方法在没有完整类型信息时按同名方法识别，可能误计无关方法。函数值间接调用、helper 内部路径、循环放大和动态子调用不保证完整计入。OpenObject/DefaultObject 只构造句柄，没有额外价格。

## 4. Gas限制与超限处理

### 4.1 Gas限制机制

- 每个合约执行都有最大 Gas 限制（`VM_GAS_LIMIT` / `CallContext.GasLimit`）
- 超限时 `contractapi.ConsumeGas` panic，合约进程以 `ok=false` 结束
- Runner / Engine 将该错误向上返回

### 4.2 超限处理流程

1. 合约侧累计消耗并检查上限
2. 超限立即停止执行
3. 引擎回滚整条调用链的状态事务，清除业务数据及事件
4. 返回 Gas 不足错误

## 5. 实现细节

### 5.1 合约侧计数器（`contractapi`）

```go
func InitGas(limit uint64)
func ConsumeGas(amount uint64) // 超限 panic("gas limit exceeded")
func GasUsed() uint64
func ResetGas()
```

### 5.2 宿主侧计量

宿主 `GasMetering` 记录最近一次执行回报的 `gas`，供 `GetGasConsumed()` 兼容查询。推荐使用 `ExecuteResult.GasConsumed`。

### 5.3 GasProfile

路径：`contracts/{address}/gas_profile.json`

Compile 对原始源码 AST 扫描生成 Profile；Deploy 落盘。扫描规则：

- `func_entries`：导出函数自身入口 + 体内对同包其他函数的直接调用（按名去重）
- `loop_sites`：体内 `for` / `range` 节点数
- `api_calls`：体内对 `contractapi` 选择器的直接调用次数

## 6. EstimateGas（已实现）

```go
type EstimateOptions struct {
    DryRun   bool
    LoopBound uint64 // 0 → 默认 1000
    Args     []any
    CallCtx  *CallContext
}

func (vm *VMEngine) EstimateGas(address, function string, opts *EstimateOptions) (*GasEstimate, error)
```

### 6.1 静态模式（默认）

```text
LoopBound = opts.LoopBound; if 0 then 1000
Estimated = entry_gas
          + func_entries * 1
          + loop_sites * LoopBound
          + Σ(api_calls[name] * GasTable[name])
```

按约定公式的启发式估算（非严格 gas 上界）：同名 helper 多次调用按名去重、循环内 API 不按迭代放大，这些场景可能出现 static < dry_run。DryRun 只测量给定输入与上下文的一次执行，不保证其他输入或状态下的上界；硬限制由运行时有效 Gas 上限执行。不做路径敏感分析。无 `gas_profile.json`（旧合约）时返回 error，需重新 Compile/Deploy。

### 6.2 DryRun 模式

使用独立状态事务执行并回滚，不发布事件，成功时 `Estimated = GasConsumed`，`Mode = "dry_run"`；失败（含 OOG）返回 error。

## 7. 基准测试

`gas_bench_test.go` 提供：

1. `BenchmarkExecuteAdd` — 短函数
2. `BenchmarkExecuteLoop` — 固定迭代循环
3. `BenchmarkEstimateGasStatic` — 静态估算（含 Log）

```bash
go test . -bench=BenchmarkExecute -benchtime=1x -count=1
```

详细设计见 [`detailed_design/gas_metering_detailed_design.md`](detailed_design/gas_metering_detailed_design.md)。


## 运行时完整性与并发

Gas 初始化/重置等管理符号仅供编译器入口使用。合约源码禁止引用这些符号、保留的 __vm 标识、init 和 recover；此检查不受可选安全开关影响。
宿主按调用创建独立计量器，以引擎和调用上下文的较小非零上限校验，避免并发重置/累加干扰。零调用上限不能解除已配置的引擎上限。
宿主与合约计数器均在加法前检查 uint64 溢出。失败扣费不修改计数器。
这些措施只加固当前计量协议，未覆盖完整内存、标准库复杂度及操作系统资源成本。

## 默认 Object Gas（C2）

Get / Set / Delete 固定为 5 / 10 / 5，字段与消息总量受独立资源上限约束。AST 画像在导入 contractapi 时统计同名 Get/Set/Delete 方法为 Object 调用，因此可能包含无关方法；函数值间接调用仍可能漏计。画像保持 schema v1，Gas 语义版本升级为 go-gas-v3-call。静态值仍是启发式估算，硬限制由运行时与宿主校验执行。详见 [Object 与执行事务](object_runtime.md)。

## 跨合约 Gas（C3）

Call 固定 10 Gas 加子树实际消耗，根计量一次。静态画像只估算固定成本，不估算动态目标子树；DryRun 执行整链后丢弃临时 store，返回整链 Gas。规则及预算见 [跨合约调用](cross_contract.md)。

## 多对象静态画像补齐

CreateObject、Owner、Transfer、Destroy 与运行时常量对齐。无循环的直接 CreateObject 函数静态值由 11 修正为 31；运行时扣费规则不变。编译流水线身份升级为 pipeline-v14-owned-gas-profile，旧缓存不会被复用。已部署画像不会原地重写，需重新编译部署才能获得新画像；部署摘要和地址可能随画像变化。
