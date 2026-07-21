# Host Runtime 设计（MVP）

## 1. 目标

让合约通过受控 API 访问链上下文，而不是只能执行纯函数。

当前阶段只设计并落地**宿主侧接口与类型**；合约侧 SDK 与双向 IPC 后续实现。

## 2. 包边界

不使用 `internal`：

```text
vm/
├── host.go          # Host / Address / Event 等宿主接口
├── runtime.go       # CallContext（后续）
└── contractapi/     # 合约侧默认库（后置）
```

## 3. MVP 接口

```go
type Address string

type Event struct {
    Name   string
    Fields map[string]any
}

type Host interface {
    BlockHeight() uint64
    BlockTime() uint64
    Sender() Address
    ContractAddress() Address
    Log(eventName string, keyValues ...any)
}
```

### MVP 范围
- 只读链上下文
- 事件日志收集

### 后置
- `Transfer` / `User`
- Object 存储
- 跨合约 `Call`
- Assert / GetHash

## 4. 通信方案

### 4.1 近阶段（推荐）
继续单次调用模型：

1. 宿主在启动合约前注入只读上下文（环境变量或 stdin 前置 envelope）
2. 合约输出结果 JSON，可附带 `events` 数组
3. 宿主解析 events 并写入 Host 实现

示例响应扩展：

```json
{
  "ok": true,
  "result": 30,
  "gas": 12,
  "events": [
    {"name": "Transfer", "fields": {"from": "a", "to": "b", "amount": 1}}
  ]
}
```

### 4.2 后续
若需要合约执行中多次回调宿主（GetObject / Call），再引入：

- Unix domain socket / JSON-RPC
- 或独立 hostagent 进程

## 5. 与默认库关系

合约侧将来 import `github.com/lengzhao/vm/contractapi`（或等价包），而不是直接依赖宿主 `VMEngine` API。

见 [`default_library.md`](default_library.md)。

## 6. 验收标准（实现阶段）

1. `Host` 接口可被测试用假实现替换
2. Execute 可携带 CallContext（Sender、Height 等）
3. 事件可从执行结果收集
4. Object / Call 明确后置，不阻塞 MVP
