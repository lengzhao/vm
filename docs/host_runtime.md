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

合约只能 import `github.com/lengzhao/vm/contractapi`，禁止 import 宿主根包 `github.com/lengzhao/vm`。

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

// 指针字段区分「未设置」与零值（例如 BlockHeight=0）
type CallContext struct {
    Host            Host
    GasLimit        *uint64
    Sender          *Address
    ContractAddress *Address
    BlockHeight     *uint64
    BlockTime       *uint64
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

1. Runner 将解析后的 `CallContext` 写入**最小环境变量**（不继承完整 `os.Environ()`）：
   - `PATH`（便于启动子进程）
   - `VM_BLOCK_HEIGHT` / `VM_BLOCK_TIME` / `VM_SENDER` / `VM_CONTRACT_ADDRESS`（仅在已设置时注入）
   - `VM_GAS_LIMIT`
2. 合约通过 `contractapi` 读取上下文并 `Log` 事件
3. entry 在 JSON 响应中附带 `events`
4. Engine 将事件写入 `ExecuteResult`；`GetLastEvents()` 仅为兼容保留
5. **不再**把 events 回放到 `Host.Log`，避免重复记账

```json
{
  "ok": true,
  "result": [0, "alice", "contract_xxx"],
  "gas": 12,
  "events": [
    {"name": "InfoCalled", "fields": {"sender": "alice"}}
  ]
}
```

## 5. 后置

- Object 存储 / Transfer / Call
- 执行中双向 RPC/IPC

## 6. 验收

- [x] `Host` 可被 `MemoryHost` 替换
- [x] `ExecuteWithContext` 可注入 Sender / Height（含零值 Height）
- [x] 事件以 `ExecuteResult` 自包含返回
- [x] 禁止合约 import 宿主根包
- [x] Object / Call 明确后置
