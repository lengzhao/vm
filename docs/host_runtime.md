# Host Runtime 设计（MVP）

## 1. 目标

让合约通过受控 API 访问链上下文，而不是只能执行纯函数。

## 2. 包边界

```text
vm/
├── core/host.go                 # Host / CallContext / MemoryHost
├── internal/engine/engine.go    # Execute(ctx, ExecutionRequest)
├── internal/process/runner.go   # 注入环境变量、解析 events
├── internal/callcontext/        # 上下文解析与传递
├── internal/compat/             # ExecuteWithContext 旧签名适配
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

// 新插件契约：取消上下文和业务上下文分别传入。
type Executor interface {
    Execute(context.Context, ExecutionRequest) (*ExecuteResult, error)
}
// ExecutionRequest 包含 Address / Function / Args / CallContext。
// 根包兼容门面仍保留：
func (vm *VMEngine) ExecuteWithContext(address, function string, callCtx *CallContext, args ...any) (*ExecuteResult, error)
```

### 合约侧（`github.com/lengzhao/vm/contractapi`）

```go
BlockHeight() uint64
BlockTime() uint64
Sender() string
ContractAddress() string
Log(eventName string, keyValues ...any)

// 以下 Gas 符号仅供生成的运行时入口使用，合约源码不可引用。
// 合约只能使用上述五个业务 API（默认库内部按表扣费）。
InitGas(limit uint64)
ConsumeGas(amount uint64)
GasUsed() uint64
ResetGas()
```

| 接口 | Gas | 状态 |
|------|-----|------|
| BlockHeight / BlockTime / Sender / ContractAddress | 1 | 已实现 |
| Log | 2 | 已实现 |
| Object / Transfer / Call | — | 后置 |

## 4. 通信方式（当前）

1. Runner 将解析后的 `CallContext` 写入**最小环境变量**（不继承完整 `os.Environ()`）：
   - `PATH`（便于启动子进程）
   - `VM_BLOCK_HEIGHT` / `VM_BLOCK_TIME` / `VM_SENDER` / `VM_CONTRACT_ADDRESS`（仅在已设置时注入）
   - `VM_GAS_LIMIT`
2. 合约通过 `contractapi` 读取上下文并 `Log` 事件
3. entry 在 JSON 响应中附带 `events`
4. Engine 将事件写入 `ExecuteResult`；`GetLastEvents()` 仅在根包兼容门面保留
5. **不再**把 events 回放到 `Host.Log`，避免重复记账

```json
{
  "version": 1,
  "ok": true,
  "result": [0, "alice", "contract_xxx"],
  "gas": 12,
  "events": [
    {"name": "InfoCalled", "fields": {"sender": "alice"}}
  ]
}
```

## 5. 后置

- Object 存储 / Transfer / Call（含对应 Gas）
- 执行中双向 RPC/IPC

## 6. 验收

- [x] `Host` 可被 `MemoryHost` 替换
- [x] `ExecuteWithContext` 可注入 Sender / Height（含零值 Height）
- [x] 事件以 `ExecuteResult` 自包含返回
- [x] 禁止合约 import 宿主根包
- [x] 默认库只读 + Log 按表扣费
- [x] Object / Call 明确后置


## 调用隔离约定

Engine 在调用 Runner 前把 Host 解析为独立的值快照，不将可变 Host 或调用方字段指针交给 Runner。调用方仍需避免在读取期间并发修改 Host 本身。
配置的最大 Gas 是硬上限，单次调用只能进一步收紧。兼容查询表示最近结束的调用；并发请求必须使用各自的 ExecuteResult。
事件字段中的通用 JSON 数值解码为 json.Number，以保留大整数。
