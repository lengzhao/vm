# Gas计费与资源控制设计文档

## 1. 概述

本文档描述智能合约虚拟机中的 Gas 计费与资源控制。目标是防止合约执行消耗过多系统资源，保证网络稳定与公平。

## 2. Gas计费原理

### 2.1 编译期注入（已实现）

编译阶段在以下位置插入 `contractapi.ConsumeGas`：

- 每个导出/内部函数入口（1 Gas）
- `for` / `range` 循环体开头（每次迭代 1 Gas）
- 执行入口固定基础 Gas（`contractapi.InitGas` + `ConsumeGas(10)`）

合约进程通过环境变量 `VM_GAS_LIMIT` 接收上限，JSON 响应中回报 `gas`（即 `contractapi.GasUsed()`）。

Deploy 同时落盘 `gas_profile.json`，供静态 `EstimateGas` 使用。

### 2.2 默认库接口计费（已实现 / 后置）

| 状态 | 接口 |
|------|------|
| **已实现** | `BlockHeight` / `BlockTime` / `Sender` / `ContractAddress` / `Log` |
| **后置** | Object 存储、`Transfer`、`Call` 等 |

已实现接口在 `contractapi` 内按 Gas 表调用 `ConsumeGas`。

### 2.3 历史“按行计费”说明

早期方案按代码行计费。当前为控制点计费，语义更稳定、更易测试。行级模型视为可选精细化方案，不作为当前实现约束。

## 3. Gas消耗模型

### 3.1 控制点（已实现）

| 操作类型 | Gas消耗 |
|---------|--------|
| 执行入口基础消耗 | 10 |
| 函数入口 | 1 |
| for / range 每次迭代 | 1 |

### 3.2 目标态基础操作（后置精细化，非当前实现）

| 操作类型 | Gas消耗 |
|---------|--------|
| 代码行执行 | 1 |
| 变量赋值 | 1 |
| 基本算术运算 | 1 |
| 条件判断 | 1 |
| 函数调用开销 | 5 |

### 3.3 默认库接口 Gas 表

| 接口函数 | Gas消耗 | 状态 |
|---------|--------|------|
| BlockHeight() | 1 | 已实现 |
| BlockTime() | 1 | 已实现 |
| ContractAddress() | 1 | 已实现 |
| Sender() | 1 | 已实现 |
| Log() | 2 | 已实现 |
| Balance() | 5 | 后置 |
| Transfer() | 20 | 后置 |
| CreateObject() | 50 | 后置 |
| GetObject() | 10 | 后置 |
| GetObjectWithOwner() | 15 | 后置 |
| DeleteObject() | 10 | 后置 |
| Object.Get() | 5 | 后置 |
| Object.Set() | 10 | 后置 |
| Object.SetOwner() | 10 | 后置 |
| Call() | 30 | 后置 |

### 3.4 对象存储 / 跨合约（后置）

Object 与 `Call()` 计费表项保留设计，待对应 Host 能力落地后再接入。

## 4. Gas限制与超限处理

### 4.1 Gas限制机制

- 每个合约执行都有最大 Gas 限制（`VM_GAS_LIMIT` / `CallContext.GasLimit`）
- 超限时 `contractapi.ConsumeGas` panic，合约进程以 `ok=false` 结束
- Runner / Engine 将该错误向上返回

### 4.2 超限处理流程

1. 合约侧累计消耗并检查上限
2. 超限立即停止执行
3. 本阶段无持久状态提交；失败等价于无副作用
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

按约定公式的启发式估算（非严格 gas 上界）：同名 helper 多次调用按名去重、循环内 API 不按迭代放大，这些场景可能出现 static &lt; dry_run。需要硬上限时请用 DryRun。不做路径敏感分析。无 `gas_profile.json`（旧合约）时返回 error，需重新 Compile/Deploy。

### 6.2 DryRun 模式

调用 `ExecuteWithContext`，成功时 `Estimated = GasConsumed`，`Mode = "dry_run"`；失败（含 OOG）返回 error。

## 7. 基准测试

`gas_bench_test.go` 提供：

1. `BenchmarkExecuteAdd` — 短函数
2. `BenchmarkExecuteLoop` — 固定迭代循环
3. `BenchmarkEstimateGasStatic` — 静态估算（含 Log）

```bash
go test . -bench=BenchmarkExecute -benchtime=1x -count=1
```

详细设计见 [`detailed_design/gas_metering_detailed_design.md`](detailed_design/gas_metering_detailed_design.md)。
