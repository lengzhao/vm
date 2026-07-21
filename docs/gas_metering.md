# Gas计费与资源控制设计文档

## 1. 概述

本文档详细描述了智能合约虚拟机中的Gas计费与资源控制系统。该系统旨在防止合约执行过程中消耗过多系统资源，确保网络的稳定性和公平性。

## 2. Gas计费原理

当前实现为 MVP，后续可扩展默认库计费。

### 2.1 编译期注入（已实现）

在合约编译阶段，系统在以下位置插入 `__vmConsumeGas`：

- 每个导出/内部函数入口
- `for` / `range` 循环体开头
- 执行入口固定基础 Gas（当前为 10）

合约进程通过环境变量 `VM_GAS_LIMIT` 接收上限，并在 JSON 响应中回报 `gas`。

### 2.2 接口操作计费（后置）

默认库函数的固定 Gas 表保留设计，待 Host Runtime 落地后接入：

- 基础查询消耗较少 gas
- 存储与跨合约调用消耗较多 gas
- 复杂计算在 default library 中显式定价

### 2.3 历史“按行计费”说明

早期方案按代码行计费。当前改为控制点计费，语义更稳定、更易测试。文档中的行级模型视为后续可选精细化方案，不再作为当前实现约束。

## 3. Gas消耗模型

### 3.1 MVP 已实现

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

### 3.3 区块链接口Gas消耗（Host Runtime 接入后生效）

| 接口函数 | Gas消耗 |
|---------|--------|
| BlockHeight() | 1 |
| BlockTime() | 1 |
| ContractAddress() | 1 |
| Sender() | 1 |
| Balance() | 5 |
| Transfer() | 20 |
| Log() | 2 |
| CreateObject() | 50 |
| GetObject() | 10 |
| GetObjectWithOwner() | 15 |
| DeleteObject() | 10 |
| Object.Get() | 5 |
| Object.Set() | 10 |
| Object.SetOwner() | 10 |
| Call() | 30 |

### 3.4 对象存储接口Gas消耗（后置）

| 接口函数 | Gas消耗 |
|---------|--------|
| CreateObject() | 50 |
| GetObject() | 10 |
| GetObjectWithOwner() | 15 |
| DeleteObject() | 10 |
| Object.Get() | 5 |
| Object.Set() | 10 |
| Object.SetOwner() | 10 |

### 3.5 跨合约调用Gas消耗（后置）

| 操作 | Gas消耗 |
|-----|--------|
| Call() 基础费用 | 30 |
| Call() 预留费用 | 根据被调用合约复杂度动态计算 |

## 4. Gas限制与超限处理

### 4.1 Gas限制机制

- 每个合约执行都有最大Gas限制
- 当Gas消耗超过限制时，合约执行立即终止
- 超限的交易被视为无效交易

### 4.2 超限处理流程

1. 监控Gas消耗
2. 当接近限制时，触发预警
3. 超过限制时，立即停止执行
4. 所有状态操作都是缓存到内存中，全部正确执行后，才会提交到数据库
5. 如果异常，则不提交，相当于回滚了
6. 返回Gas不足错误

## 5. 实现细节

### 5.1 Gas计数器

Gas计数器在合约执行过程中跟踪已消耗的Gas数量：

```go
type GasMeter struct {
    limit    uint64
    consumed uint64
    enabled  bool
}
```

### 5.2 Gas消耗函数

```go
func (g *GasMeter) ConsumeGas(amount uint64, descriptor string) error {
    if !g.enabled {
        return nil
    }
    
    g.consumed += amount
    if g.consumed > g.limit {
        return fmt.Errorf("out of gas: %s", descriptor)
    }
    return nil
}
```

### 5.3 编译期Gas注入

在编译阶段，通过AST分析在适当位置插入Gas消耗代码：

```go
// 插入示例
gasMeter.ConsumeGas(1, "code line execution")
```

## 6. 模块职责划分

### 6.1 Gas计费模块
Gas计费模块([gas_metering_detailed_design.md](./detailed_design/gas_metering_detailed_design.md))负责基础的Gas计量功能：
- 设置Gas限制
- 跟踪Gas消耗
- 超限时处理（panic）

### 6.2 接口模块
接口模块负责具体的Gas消耗数值计算：
- 根据操作类型确定Gas消耗量
- 在接口函数调用时消耗相应Gas
- 提供Gas估算功能

## 7. EstimateGas 与基准测试（下一阶段）

### 7.1 EstimateGas 初版思路

```text
EstimateGas ≈ 入口基础Gas(10)
            + 静态函数入口数 * 1
            + 静态循环控制点数 * 配置迭代上界
            + 默认库调用表累加（Host 接入后）
```

初版只做保守上界，不做精确路径分析。

### 7.2 基准测试方向

1. 短函数：`Add`
2. 固定迭代循环函数
3. 多返回值函数
4. （后续）含 `Log` / Object / Call 的合约

详细设计见 [`detailed_design/gas_metering_detailed_design.md`](detailed_design/gas_metering_detailed_design.md)。
