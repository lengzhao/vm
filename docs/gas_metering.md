# Gas计费与资源控制设计文档

## 1. 概述

本文档详细描述了智能合约虚拟机中的Gas计费与资源控制系统。该系统旨在防止合约执行过程中消耗过多系统资源，确保网络的稳定性和公平性。

## 2. Gas计费原理

Gas计费系统通过以下方式实现资源控制：

### 2.1 代码行计费

在合约编译阶段，系统会自动在代码中插入Gas消耗点：

- 每个代码块开始处注入Gas消耗代码
- 每行代码执行消耗1点gas
- 支持条件语句、循环语句等复杂控制流结构

### 2.2 接口操作计费

所有的包函数调用都有固定的gas消耗：

- 基础操作（如查询区块信息）消耗较少gas
- 存储操作（如创建对象、修改字段）消耗较多gas
- 合约调用等高级操作有额外的gas预留机制
- 复杂计算操作通过在default library里显式指定消耗更多gas进行限制

## 3. Gas消耗模型

### 3.1 基础操作Gas消耗

| 操作类型 | Gas消耗 |
|---------|--------|
| 代码行执行 | 1 |
| 变量赋值 | 1 |
| 基本算术运算 | 1 |
| 条件判断 | 1 |
| 函数调用开销 | 5 |

### 3.2 区块链接口Gas消耗

| 接口函数 | Gas消耗 |
|---------|--------|
| BlockHeight() | 1 |
| BlockTime() | 1 |
| ContractAddress() | 1 |
| Sender() | 1 |
| Balance() | 5 |
| Transfer() | 20 |
| Log() | 2 |

### 3.3 对象存储接口Gas消耗

| 接口函数 | Gas消耗 |
|---------|--------|
| CreateObject() | 50 |
| GetObject() | 10 |
| GetObjectWithOwner() | 15 |
| DeleteObject() | 10 |
| Object.Get() | 5 |
| Object.Set() | 10 |
| Object.SetOwner() | 10 |

### 3.4 跨合约调用Gas消耗

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
4. 回滚已执行的操作
5. 返回Gas不足错误

## 5. Gas预估与优化

### 5.1 Gas预估机制

- 提供静态分析工具预估合约执行所需Gas
- 在合约部署时进行Gas消耗分析
- 为用户提供Gas消耗参考值

### 5.2 Gas优化建议

- 避免不必要的循环和递归
- 减少存储操作次数
- 合理使用跨合约调用
- 优化数据结构以减少内存使用

## 6. 实现细节

### 6.1 Gas计数器

Gas计数器在合约执行过程中跟踪已消耗的Gas数量：

```go
type GasMeter struct {
    limit    uint64
    consumed uint64
    enabled  bool
}
```

### 6.2 Gas消耗函数

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

### 6.3 编译期Gas注入

在编译阶段，通过AST分析在适当位置插入Gas消耗代码：

```go
// 插入示例
gasMeter.ConsumeGas(1, "code line execution")
```