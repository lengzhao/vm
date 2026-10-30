# 可恢复批次与独占写入（E2）

E2 新增可选 `batch/recoverable` 和 `engine/recoverable`，使用 `state/bbolt` 的独占会话与原子回执。原 `core.Engine`、`BatchExecutor.ExecuteBatch`、`batch/ordered`、内存状态及默认配置不变。

## 使用

完整配置见 [`presets/recoverable.yaml`](../presets/recoverable.yaml)。由同一配置图保证 Engine、恢复调度器与会话所用的状态实例相同：

```yaml
state:
  use: state/bbolt
  config: {path: ./state.db, lockTimeout: 1s, maxBatchReceiptMB: 64}
batch:
  use: batch/recoverable
  config: {workers: 4, maxBatch: 1024, executionID: strict-process-gas1m-v1}
  deps: {state: state, executor: engine, contracts: engine}
app:
  use: engine/recoverable
  deps: {engine: engine, batch: batch}
```

```go
batch, ok := rt.Engine.(core.RecoverableBatchExecutor)
if !ok { return fmt.Errorf("recoverable batches not configured") }
items, err := batch.ExecuteRecoverableBatch(ctx, core.RecoverableBatchRequest{
    ID: "chain-1/block-100",
    Calls: requests,
})
```

调用方保存稳定 ID 与完整请求。重试时重新提交完整 Calls，而非只传剩余项。返回值包含历史已记录前缀及本次新完成项，顺序与输入相同；已完成的同 ID/同内容调用仅重放回执，不再次执行。相同内容但不同 ID 是新批次，可以再次改变状态。

ID 是同一数据库中的幂等命名空间，长度 1–256 UTF-8 字节。worker 默认 4、范围 1–64，maxBatch 默认 1024、范围 1–65536；输入序列化总计最多 16 MiB。executionID 必填，1–256 UTF-8 字节，表示可信部署方声明的执行规则身份。修改 Gas 上限、策略、协议语义、宿主/Runner 执行配置时须使用新的 executionID；同一批次不能在新身份下续跑。系统绑定这个声明，而不自动证明任意自定义插件/配置的行为相等。改变 workers 或提高存储回执额度不改变执行语义。

## 请求身份与确定性要求

请求摘要以版本 1 绑定 executionID、全部调用地址/函数/参数、显式 CallContext 及各入口部署产物的清单摘要。参数仅允许 ABI 标量（整数、json.Number、字符串、布尔及 null），不接受自定义对象、MarshalJSON、浮点或指针。Host 回调不接受，需显式传入链上下文字段。入口时复制并冻结输入，调用方仍不得在冻结期间并发修改输入。

部署产物必须存在、通过完整性及部署地址验证，并声明确定性保证。重放也需要产物身份可查询/校验。部署地址绑定清单，嵌套合约由 Engine 验证；配置须对整个调用树启用确定性产物策略，产物存储及可信插件不能被外部替换或绕过。指纹不是授权或代码来源认证；执行规则、时间/资源故障等确定性边界沿用 [D3](parallel_execution.md)。

同 ID 的摘要或数量不一致时返回可 `errors.Is(err, core.ErrBatchIdentityConflict)` 判断的错误，不覆盖已有回执。空 Calls 可建立可重复查询的空批次。

## 共享独占写入会话

`core.RecoverableBatchStore.AcquireBatch(ctx, identity)` 返回 `BatchSession`，只暴露会话快照、Prefix、Finalize 和 Close。调度器不要求具体 bbolt 类型，也不要求它实现 Engine 或普通 StateStore。

状态层等待已打开的普通事务结束后授予会话；有会话等待时，新普通事务不插队。活动会话期间，普通 Begin 与其它会话在同一个后端门限等待，调用方 context 可取消等待。会话提供独立 MVCC 快照，窗口至多 workers 项并行推测，按输入序号验证和提交；冲突项仍顺序重执行一次。会话事务不能直接 Commit，必须经过 Finalize 原子写回执。

这是同一后端实例内所有正常状态入口共享的约束，包括普通 Engine.Execute 和旧批量入口，不仅锁调度器自身。它不约束原始文件篡改或可信插件绕过接口的行为；不同文件是独立状态库。数据库文件仍由单实例独占。

## 持久化未完成标记

首次开始非空批次时，先原子记录身份和未完成标记。每项成功或业务失败都有连续回执；只有最后一项的回执提交成功，才在同一数据库事务中删除标记。

若会话中断、调用取消、进程退出或数据库重开，标记仍保留。无活动会话但存在标记时，普通 Begin 立即返回带批次 ID 的 `ErrBatchPending`，而非无限等待；其它 ID 的 AcquireBatch 也拒绝。只有同 ID/同摘要可恢复。这保证两次恢复调用之间没有普通状态写入插入批次中间。

Close 不删除未完成标记；它取消并丢弃剩余推测快照、释放活动会话。数据库 Stop/fault 同样失效会话并唤醒等待者。不提供“放弃未完成批次并继续其它写入”的自动开关；调用方必须保留原请求并恢复，永久不可恢复时需设计业务恢复流程，不能手改标记后声称状态仍有同序保证。

并发调用同一 ID 会串行：先到者执行，后到者获取回执并跳过已完成项。不同批次的先后顺序由集成方安排，系统不定义网络到达顺序或区块共识顺序。

## 状态与回执一起提交

Finalize 在同一 bbolt 事务中写入：成功交易的状态变化、连续索引的回执、累计字节数，以及最后一项时的未完成标记删除。纯读交易也落回执。业务失败先验证观察到的读写版本，再丢弃 guest 写入并仅写失败回执，不将失败写入混入状态。

回执包含 Data、Events、Gas、稳定错误码和诊断。首次返回与重放统一使用 JSON 回执数值表示，事件中的整数使用 json.Number，uint64 不经过 float64；Data 原字节保留。业务失败的底层 Go cause 不持久化，恢复接口在首次返回时也只提供稳定错误分类和诊断。基础设施错误保留原始原因并停止，不写交易回执。

单项编码回执最多 4 MiB，业务诊断最多 1 MiB；累计回执默认每批 64 MiB。state/bbolt 的 maxBatchReceiptMB 可配置 4–1024 MiB。达到累计额度时，本项状态和回执都不提交，未完成标记保留；关闭并用相同数据库及更高额度重新打开后可续跑，不改变批次摘要。额度无法满足需求时不自动跳项或清理历史回执。

首版不清理历史批次。内存包含全量状态、当前批次已保存回执及返回结果；部署方应限制批次和回执大小，不能将这些上限当作整个进程 RSS 配额。

## 不确定提交与恢复

磁盘 Commit 报错时仍返回 `ErrStateCommitUncertain`，后端故障隔离，不自动再次执行该项。该项可能已落盘但不在本次返回值中。停止旧实例、使用新实例重新打开数据库，再以同 ID/同内容调用：

- 若状态+回执已提交，读取回执并跳过该项。
- 若均未提交，从该位置重新执行。

原子事务保证不存在“新状态却没有对应回执”或“成功回执却没有状态”的合法提交结果。数据库损坏/文件篡改、设备违背同步语义不在此保证内。校验连续索引、身份与字节计数，异常记录拒绝恢复；不静默修复。

最后一项提交同时删除标记，因此崩溃在最后一次成功提交后不会留下合法的未完成标记。已完成批次之后允许其它写入；重放旧批次只返回旧回执，不恢复当时状态。

## 事件与不包含的能力

回执保存事件供查询和重放，不主动投递外部系统。恢复调用会再次返回已保存事件，消费者必须按批次 ID/项目索引等自行去重；不承诺外部事件恰好一次。

这不是整批原子提交：已完成前缀保留。没有请求授权、分布式锁、链重组/撤销、跨文件事务或跨进程并行写入；一致数据库备份/恢复工具见 [运维指南](operations.md)。可信部署方仍负责执行配置身份、产物可用性、原请求保存和故障操作流程。

测试覆盖普通/批次入口互斥、旧事务等待、持久标记重启、同ID冲突、读/失败回执、MVCC冲突、提交前后故障、无Stop子进程退出、额度恢复、整数精度、真实合约跨调用/OOG/父失败以及中断后的续跑和重复调用。
