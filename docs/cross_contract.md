# 跨合约调用与临时 store（C3）

```go
result, err := contractapi.Call(targetAddress, "Read")
```

`Call(address, function, args...) ([]byte, error)` 返回子调用业务结果的 JSON 字节。公开参数仍按 ABI schema v1 的标量规则校验，大整数在宿主转发时使用 json.Number 保留精度。合约不能指定子调用 Sender、区块上下文、Gas 上限或事务。

## 一份临时 store，一次发布

最外层调用执行 `StateStore.Begin`，将得到的事务作为整条调用链的临时 store。所有合约按真实地址隔离字段，但都读写同一份本次覆盖，因此后续子调用能读到前面尚未正式发布的修改。子调用不会 Begin、Commit 或逐键写正式 store。

最外层成功且 Gas 校验通过后，执行一次原子 Commit，将临时 store 的修改发布到正式 store。业务错误、panic、OOG、取消/超时、目标加载/校验失败、深度/重入错误、无效或超限消息、提交冲突均使整个调用链失败，Rollback 仅丢弃临时数据。DryRun 同样丢弃。没有保存点，也没有失败后逐条补偿正式数据的路径。

子调用失败会记录在整链会话中，父合约即使忽略 `Call` 的 error，也无法提交。运行时本地的参数编码/通道错误同样记录失败，最终入口不能把它变成成功响应。失败调用不发布业务数据或事件；可获得的已用 Gas 作为诊断返回，不承诺节点基础设施故障时有完整计费收据。

默认 `state/memory` 只在当前 VM 实例存活期间保存正式状态，不跨实例或进程重启持久化。状态后端仍通过最小 `core.StateStore` 契约替换。

## 调用上下文与限制

- 根帧算第一层，默认最多 8 层；`VMConfig.MaxCallDepth` / `engine/default.config.maxCallDepth` 可设为 1–32，0 使用默认值。
- 调用栈中已存在的地址不能再次进入，包含自调用和 A→B→A；B 返回后再顺序调用 B 允许。
- 子帧的 Sender 为直接调用合约的真实地址，ContractAddress 为真实目标；不使用父帧可覆盖的展示地址充当发送者或存储命名空间。
- 区块高度/时间沿用最外层值快照，继承根调用截止时间；自定义 Runner 传入新的 context 也不能脱离根调用取消。进程 Runner 自身更短的超时仍生效。
- 整链最多 1024 个跨合约调用；每个子调用 JSON 结果最多 64 KiB，整链事件 JSON 编码累计最多 1 MiB。保留每进程 1024 次宿主请求、stdout/stderr 各 1 MiB、宿主回复累计 1 MiB 等限制。

默认 Object 的字段命名空间始终绑定当前实际执行地址。Call 是一个独立的 `core.ContractCaller` 能力，和 ObjectStore 一起按帧注入 Runner context；它们不暴露 Commit/Rollback，不注册成全局共享插件。

## 共享 Gas

Call 在发请求前扣固定 10 Gas，并报告调用点已用量（含之前的子调用）。宿主只给新子帧剩余预算。子帧消耗包含自身入口、函数/循环、API 和后续子树；回复带实际子树消耗，父 runtime 再扣同一数量，最终只将根帧累计量计入根计量器一次。

宿主同时验证调用点单调下界、最终 Gas 不少报、加法不溢出以及剩余预算。剩余量为零或不足入口 Gas 时直接 OOG，不会把 0 作为无限额传给子帧。失败也不恢复已使用预算；整个临时 store 会丢弃。静态 Gas 画像只计 Call 的固定成本，无法预估目标子树，仍是启发式值；DryRun 才测量给定输入与状态的一次整链执行。

## 事件顺序

运行时在每次 Call 时取出父帧此前缓冲的事件，作为请求前缀交给宿主；宿主依次拼接前缀、子帧事件和父帧后续事件。例如 A.Log(before) → B.Log(child) → A.Log(after) 返回 before、child、after。连续或更深层调用使用同一规则。仅最外层成功提交后对外返回整条事件序列，失败或 DryRun 全部丢弃。

## 协议与错误

协议 v3 新增 Call 帧：

```json
{"version":3,"type":"call","address":"contract_target","function":"Read","args":[],"gas":21,"events":[]}
```

回复中的 result 是子调用 JSON 字节的 base64，gas 是子树实际消耗：

```json
{"version":3,"ok":true,"result":"Nw==","gas":11}
```

Object 消息和最终结果也使用协议 3；未知字段/版本、缺失 Gas、无效参数结构、无权限的上下文字段和少报消耗均拒绝。子调用错误由宿主保留原始 code/cause；新增 `CALL_FAILURE`（通道、本地失败或整链调用资源限制）、`CALL_DEPTH_EXCEEDED`、`REENTRANCY`。错误文本不作为程序判断依据。

当前规则：`contractapi-v3-call`、`go-gas-v3-call`、`pipeline-v14-owned-gas-profile`、`go-development-v3-call` / `go-deterministic-v3-call`。ABI/Profile schema 仍为 1。Call runtime 源码进入内嵌、缓存及清单摘要；旧产物需重新编译部署，没有旧协议兼容路径。
