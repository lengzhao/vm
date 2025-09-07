# 默认库接口规范

## 1. 概述

本文档定义了智能合约虚拟机项目提供的默认库接口规范，这些接口是合约与区块链环境交互的基础。

## 2. 接口定义

### 2.1 区块链信息相关接口

```go
// BlockHeight 获取当前区块高度
BlockHeight() uint64

// BlockTime 获取当前区块时间戳
BlockTime() int64

// ContractAddress 获取当前合约地址
ContractAddress() Address
```

### 2.2 账户操作相关接口

```go
// Sender 获取交易发送方或合约调用方
Sender() Address

// Balance 获取账户余额
Balance(addr Address) uint64

// Transfer 执行转账操作
Transfer(from, to Address, amount uint64) error
```

### 2.3 对象存储相关接口

```go
// CreateObject 创建新对象，失败时panic
CreateObject() Object

// GetObject 获取指定对象，可能返回error
GetObject(id ObjectID) (Object, error)

// GetObjectWithOwner 根据所有者获取对象，可能返回error
GetObjectWithOwner(owner Address) (Object, error)

// DeleteObject 删除对象，失败时panic
DeleteObject(id ObjectID)
```

### 2.4 跨合约调用接口

```go
// Call 跨合约调用
Call(contract Address, function string, args ...any) ([]byte, error)
```

### 2.5 日志和事件接口

```go
// Log 记录事件
Log(eventName string, keyValues ...any)
```

## 3. Object接口

```go
// Object 接口用于管理区块链状态对象
type Object interface {
  // ID 获取对象ID
  ID() ObjectID
  
  // Owner 获取对象所有者
  Owner() Address
  
  // Contract 获取对象所属合约
  Contract() Address
  
  // SetOwner 设置对象所有者，失败时panic
  SetOwner(addr Address)

  // Get 获取字段值
  Get(field string, value any) error
  
  // Set 设置字段值
  Set(field string, value any) error
}
```

## 4. 智能合约对象存储机制

### 4.1 默认对象创建
每个智能合约创建的时候，都会自动创建一个Object，用于存储智能合约的基本信息。

### 4.2 统一账户系统
智能合约可以将所有信息都保存到默认的Object里，从而实现统一的账户系统，类似EVM。这样会导致不同交易可能状态冲突，所以必须串行执行。

### 4.3 并行执行支持
也可以为不同用户创建不同的Object，不同用户的交易因为使用了不同的Object，不会存在状态冲突，所以可以并行执行。

### 4.4 默认对象访问
智能合约可以通过空的ObjectID获取默认的Object。

## 5. 设计原则

1. **安全性**：所有接口都经过严格设计，防止合约访问系统敏感资源
2. **简洁性**：接口设计简洁明了，易于理解和使用
3. **完整性**：提供合约与区块链环境交互所需的基础功能
4. **错误处理**：合理使用panic和error，确保合约执行的稳定性
5. **并发支持**：通过对象隔离机制支持交易的并行执行