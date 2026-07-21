# 默认库接口规范

## 1. 概述

默认库是合约与区块链环境交互的唯一受控入口。

- **当前态**：合约只能 import `github.com/lengzhao/vm/contractapi`；禁止 import 宿主根包
- Host Runtime 实现见 [`host_runtime.md`](host_runtime.md)

## 2. MVP（优先实现）

### 2.1 只读上下文

```go
BlockHeight() uint64
BlockTime() uint64
ContractAddress() Address
Sender() Address
```

### 2.2 事件

```go
Log(eventName string, keyValues ...any)
```

## 3. 后置接口

### 3.1 账户

```go
User() Address
Transfer(from, to Address, amount uint64) error
```

### 3.2 对象存储

```go
CreateObject() Object
GetObject(id ObjectID) (Object, error)
GetObjectWithOwner(owner Address) (Object, error)
DeleteObject(id ObjectID)
```

### 3.3 跨合约

```go
Call(contract Address, function string, args ...any) ([]byte, error)
```

### 3.4 辅助

```go
Assert(condition any)
GetHash(data []byte) Hash
```

## 4. Object 接口（后置）

```go
type Object interface {
  ID() ObjectID
  Owner() Address
  Contract() Address
  UpdatedAt() uint64
  SetOwner(addr Address)
  Get(field string, value any) error
  Set(field string, value any) error
}
```

## 5. 对象存储机制（设计方向，后置）

1. 合约创建时自动创建默认 Object
2. 统一账户模式：状态都写默认 Object（需串行）
3. 并行模式：按用户 Object隔离，交易携带 ReadList/WriteList
4. Object 所有者须为合约或交易发起方

## 6. 设计原则

1. 合约不能直接访问 OS / 网络
2. 默认库是唯一链交互面
3. MVP 先做只读上下文与事件，再扩展存储与跨合约
4. 包边界分离：宿主 `vm` ≠ 合约侧 `contractapi`
