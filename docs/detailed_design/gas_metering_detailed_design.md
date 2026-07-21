# Gas 计费详细设计

## 1. 概述

Gas 分两层：

1. **宿主侧** `GasMetering`：记录最近一次执行消耗
2. **合约侧** `contractapi`：统一计数器；编译期注入 `ConsumeGas`；默认库按表扣费

## 2. 宿主接口

```go
type GasMetering interface {
    ConsumeGas(amount uint64) error
    GetConsumedGas() uint64
    SetGasLimit(limit uint64)
    GetGasLimit() uint64
    Reset()
}
```

`VMEngine.ExecuteWithContext` 在执行后把子进程回报的 `gas` 写入宿主计量器。

## 3. 计费点（已实现）

| 计费点 | 消耗 | 实现 |
|--------|------|------|
| 入口基础 Gas | 10 | entry：`InitGas` + `ConsumeGas(10)` |
| 函数入口 | 1 | 编译注入 `__vmConsumeGas(1)`，委托 `contractapi.ConsumeGas` |
| `for` / `range` 每次迭代 | 1 | 同上 |
| BlockHeight / BlockTime / Sender / ContractAddress | 1 | `contractapi` 内部 |
| Log | 2 | `contractapi` 内部 |

超限时合约进程 panic，`ok=false`，Runner 返回错误。

Object / Transfer / Call 计费：**后置**。

## 4. GasProfile 与 EstimateGas（已实现）

### 4.1 GasProfile

Compile 扫描原始源码生成；Deploy 写入 `gas_profile.json`。

```json
{
  "version": 1,
  "entry_gas": 10,
  "functions": {
    "Add": { "func_entries": 1, "loop_sites": 0, "api_calls": {} }
  }
}
```

### 4.2 EstimateGas

```go
func (vm *VMEngine) EstimateGas(address, function string, opts *EstimateOptions) (*GasEstimate, error)
```

- **静态（默认）**：读 Profile，按约定公式启发式估算（非严格上界）；`Mode = "static"`
- **DryRun**：真执行取 `GasConsumed`；`Mode = "dry_run"`
- 无 Profile / 未知函数 → error
- `opts == nil` → 静态 + 默认 LoopBound(1000)

静态公式：

```text
Estimated = entry_gas + func_entries*1 + loop_sites*LoopBound + Σ(api_calls * GasTable)
```

### 4.3 基准测试

- `BenchmarkExecuteAdd`
- `BenchmarkExecuteLoop`
- `BenchmarkEstimateGasStatic`

## 5. 明确未实现 / 后置

- 按代码行计费
- 路径敏感 / 精确迭代次数推断的 EstimateGas
- Object / Transfer / Call 接口计费
- 状态回滚数据库语义
