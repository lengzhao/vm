# Gas 计费详细设计

## 1. 概述

Gas 分两层：

1. **宿主侧** `GasMetering`：记录最近一次执行消耗
2. **合约侧** 编译期注入的 `__vmConsumeGas`：执行中硬限制

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

`VMEngine.Execute` 在执行后把子进程回报的 `gas` 写入宿主计量器。

## 3. MVP 计费点（已实现）

| 计费点 | 消耗 |
|--------|------|
| 入口基础 Gas | 10 |
| 函数入口 | 1 |
| `for` / `range` 每次迭代 | 1 |

超限时合约进程 panic，`ok=false`，Runner 返回错误。

## 4. 下一阶段模型

### 4.1 默认库接口定价（Host 接入后生效）

保留 [`../gas_metering.md`](../gas_metering.md) 中的接口表作为目标态，例如：

- `BlockHeight` / `Sender`：低消耗
- `Log`：中低消耗
- `CreateObject` / `Transfer` / `Call`：高消耗

### 4.2 EstimateGas（计划）

保守估算：

```text
EstimateGas ≈ 入口基础Gas
            + 静态函数入口数 * 1
            + 静态循环控制点数 * 估算迭代上界
            + 默认库调用表累加
```

初版可不做精确路径分析，只给出上界或配置化粗估。

### 4.3 基准测试方向

- 短函数：`Add`
- 循环函数：固定 N 次循环
- 多返回值函数
- （后续）含默认库调用的合约

## 5. 明确未实现

- 按代码行计费
- 状态回滚数据库语义
- 精确路径敏感的 EstimateGas
