# Host Runtime 设计（MVP）

## 1. 目标

让合约通过受控 API 访问链上下文，而不是只能执行纯函数。

## 2. 包边界

```text
vm/
├── host.go              # Host / CallContext / MemoryHost
├── engine.go            # ExecuteWithContext
├── runner.go            # 注入环境变量、解析 events
└── contractapi/         # 合约侧默认库
```

## 3. 已实现接口

### 宿主侧

```go
type Host interface {
    BlockHeight() uint64
    BlockTime() uint64
    Sender() Address
    ContractAddress() Address
    Log(eventName string, keyValues ...any)
}

func (vm *VMEngine) ExecuteWithContext(address, function string, callCtx *CallContext, args ...any) (*ExecuteResult, error)
```

### 合约侧（`github.com/lengzhao/vm/contractapi`）

```go
BlockHeight() uint64
BlockTime() uint64
Sender() string
ContractAddress() string
Log(eventName string, keyValues ...any)
```

## 4. 通信方式（当前）

1. Runner 将 `CallContext` 写入环境变量：
   - `VM_BLOCK_HEIGHT`
   - `VM_BLOCK_TIME`
   - `VM_SENDER`
   - `VM_CONTRACT_ADDRESS`
   - `VM_GAS_LIMIT`
2. 合约通过 `contractapi` 读取上下文并 `Log` 事件
3. entry 在 JSON 响应中附带 `events`
4. Engine 解析事件，写入 `ExecuteResult` / `GetLastEvents()`，并回放给 `Host.Log`

```json
{
  "ok": true,
  "result": [100, "alice", "contract_xxx"],
  "gas": 12,
  "events": [
    {"name": "InfoCalled", "fields": {"sender": "alice"}}
  ]
}
```

## 5. 后置

- Object 存储 / Transfer / Call
- 执行中双向 RPC/IPC
- 合约侧包与宿主包进一步隔离（禁止 import 宿主根包）

## 6. 验收

- [x] `Host` 可被 `MemoryHost` 替换
- [x] `ExecuteWithContext` 可注入 Sender / Height
- [x] 事件可从执行结果收集
- [x] Object / Call 明确后置
