# 默认 Object 与执行事务（C2）

每个合约拥有一个默认 Object。合约只能访问当前实际执行地址的字段，字段值为不透明字节串；不允许直接选择其他地址的字段。跨合约访问通过 [Call](cross_contract.md) 进入目标合约；多 Object 与所有权转移后置。

```go
package main

import "github.com/lengzhao/vm/contractapi"

func Save(value string) error {
    return contractapi.DefaultObject().Set("value", []byte(value))
}

func Load() (string, bool, error) {
    value, exists, err := contractapi.DefaultObject().Get("value")
    return string(value), exists, err
}

func Remove() error {
    return contractapi.DefaultObject().Delete("value")
}
```

`DefaultObject()` 不消耗 Gas。Get / Set / Delete 分别固定扣除 5 / 10 / 5 Gas，操作发送给宿主前扣费。Get 返回独立的存在标志：空值和缺失不同；nil 与空切片均可表示已存储的零长度值。字段名允许空字符串，必须为有效 UTF-8，最多 1024 字节；单值最多 64 KiB。合约不能访问初始化、帧结构、Gas 管理等运行时符号。

## 事务与事件

Engine 在产物验证后调用 `StateStore.Begin`，将事务按 `ExecutionRequest.Address` 绑定为 `core.ObjectStore`。`CallContext.ContractAddress` 仍可覆盖合约读取的展示上下文，但不影响状态命名空间。Runner 通过 `core.ObjectStoreFromContext` 获取 Get/Set/Delete 能力，不能选择地址或结束事务。

执行成功、有效结果和 Gas 校验均通过后才提交，提交成功才返回业务数据与事件。业务错误、panic、OOG、超时、取消、非法/超限响应、宿主存储错误、空结果和提交冲突都回滚；失败的 ExecuteResult 清除 Data / Events，保留可获得的 Gas 诊断。回滚错误与原错误合并并保留 cause。宿主存储错误即使被自定义 Runner 忽略，也会阻止提交。

原生及根包 `EstimateGas(..., DryRun:true)` 都走独立快照执行并回滚，返回本次 Gas；不会写入状态或发布本次事件。静态估算不运行合约。普通成功调用依然按 C1 的全局版本检查提交：不相交旧写事务也可能冲突；宿主应重新 Begin 并重执行业务，Engine 不自动重试。

`STATE_FAILURE` 表示状态后端/事务错误，`STATE_CONFLICT` 包装 `core.ErrStateConflict`；用 `errors.Is` 检查原始原因。取消及超时保留相应错误码和 context cause。直接调用 Runner 不负责提交或回滚，它返回的原始 Events 仅供诊断；应用应通过 Engine 获得提交后的事件。

## 进程协议 v3

Runner 使用 stdin / stdout 的逐行 JSON 双向通信：

1. stdin 第一行是唯一的调用请求，最多 1 MiB（不含换行）。
2. 执行中 stdout 可发送 Object 请求，每行带 `type:"object"`。
3. 宿主在 stdin 返回对应的一行回复；运行时同步等待。
4. stdout 最后返回唯一的最终调用结果；结果之后不能再进行宿主操作。

请求字段不包含地址、事务或 commit 操作。例：

```json
{"version":3,"type":"object","op":"set","key":"value","value":"aGVsbG8=","gas":21}
```

宿主回复：

```json
{"version":3,"ok":true,"exists":false,"value":null}
```

Get 回复的 exists / value 表示真实字段结果；Set/Delete 的回复无业务值。字节串按 JSON base64 传输。最终响应沿用 result / events / gas / code / error 结构，见 [调用协议](call_protocol.md)。

单次执行最多 1024 次宿主请求。合约 stdout（包括请求帧）与 stderr 各累计最多 1 MiB，宿主回复另累计最多 1 MiB。宿主验证消息版本、字段、操作、大小、Gas 单调下界及调用 Gas 上限；最终 Gas 不能低于已处理的宿主操作。未知操作、地址注入或缺失 Gas 等返回 INVALID_RESPONSE；没有 Object 能力不会静默返回空值。取消及输出超限主动关闭 stdin 写端，避免继承管道读端的后代阻塞宿主；这不代表完整进程树沙箱已完成。

## 插件与版本

默认图及 `presets/default.yaml` 给 Engine 注入 `state/memory`。自定义状态插件仅需实现 `core.StateStore.Begin` 及返回的事务契约；无需实现 Host、Runner 或生命周期方法。对象存储仍在事务契约之上，见 [状态事务](state_transactions.md)。

内存后端仅在当前实例存活期间保存数据，VM 重建或进程重启后不会恢复状态。合约文件持久化与对象状态持久化是两种独立能力，当前没有持久化状态后端。

本次版本为：调用协议 3、ABI schema 1、`contractapi-v3-call`、`go-gas-v3-call`、编译流水线 `pipeline-v13-cross-contract`，源码策略为 `go-development-v3-call` / `go-deterministic-v3-call`。产物清单绑定新增 Object runtime 源码。旧产物要求重新编译部署，不提供协议 v1 兼容分支。

多对象、所有权和转移不在本次范围；C3 已实现跨合约共享临时 store/Gas、重入与深度限制，见 [跨合约调用](cross_contract.md)；D3 负责细粒度冲突和确定性并行。
