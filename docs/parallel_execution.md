# 有序并行执行（D3）

## 插件与边界

`state/mvcc` 提供按键版本校验，`batch/ordered` 提供有界推测执行和有序提交。二者均为可选插件，原 `state/memory`、单次 `Execute` 和默认配置保持原行为。执行 Runner 仍可选 Process、Docker 或 wazero，调度器不识别或创建具体后端。

相同输入和初始状态下，批量执行的最终状态、逐项 Data / Events / Gas / 业务错误与输入顺序的串行执行相同。此前提要求：

- 合约及 ABI/Gas/宿主插件满足确定性约束；严格策略声明由可信实现负责。示例配置同时要求确定性源码与产物。
- 批次期间独占状态写入权，无外部 `Execute`、事务提交或另一个调度器写入同一 store；合约部署产物不得替换。
- 调用期间请求、Args、CallContext 及指针值不可被调用方修改，参数编码不得带外部副作用。
- 宿主能力只能在事务中写状态，不能在推测阶段发送消息或写外部系统。超时、OOM、机器/容器故障不属于共识确定性保证。

同一 `batch/ordered` 实例用可取消的入口门限串行处理批次；此门限不约束其它实例、单次 Execute 或直接访问 store 的调用者。上述独占要求由集成方保证。D3 不实现持久化、区块整体原子提交、跨进程锁或并行速度保证。

## 能力契约

- `core.StateStore` / `StateTransaction` 保持原样。
- `core.SnapshotStore.BeginSnapshot(ctx)` 返回 `SnapshotTransaction`，除原事务操作外提供 `Validate(ctx)`。验证包含已读和已写键（含缺失键），不提交、不关闭；`Commit` 在同一锁内验证并发布，纯读 Commit 也校验。
- `core.TransactionExecutor.ExecuteTransaction(ctx, request, tx)` 运行完整调用树及 Gas 验证，不提交或回滚调用方事务。失败返回的 Data / Events 清空；调用方必须回滚失败写入。该能力仅供可信调度器使用，合约不可见。
- `core.BatchExecutor.ExecuteBatch(ctx, requests)` 返回按输入顺序的 `[]BatchItemResult` 及整体错误。`BatchItemResult.Err` 为该项最终业务失败。

`engine/default` 的具体实现额外提供 TransactionExecutor，未扩大 `core.Engine`。`engine/batched` 只组合 Engine 和 BatchExecutor 两个门面能力，便于现有 assembly 入口构建完整图；batch 本身仅依赖 SnapshotStore 与 TransactionExecutor，不要求 Engine。自定义插件只需实现消费者所需接口。

## 执行与提交

1. 按 `workers` 将输入切为有界窗口；为窗口内每个请求先获取快照事务。
2. 同时运行至多 workers 个请求，每个请求及其跨合约子树共享自己的事务。窗口全部结束后才进入提交阶段。
3. 按输入顺序处理结果。成功项原子 Commit；业务失败项先 Validate，再 Rollback。失败和纯读项也必须验证，以免返回基于旧状态的成功/失败或数据。
4. 发现冲突时，先回滚旧推测结果，再在最新状态上顺序重执行该项一次。丢弃推测的结果、事件及 Gas，只返回最终一次执行的计量；宿主已付出的 CPU/墙钟成本不会被撤销。
5. 重执行再次冲突时停止，避免外部写入导致无限重试。窗口全部处理完再开始下一窗口。

业务错误采用明确分类：参数/入口错误、合约 error/panic、OOG、结果错误、调用失败、深度/重入拒绝。其它未知错误、取消/超时、状态或产物错误、进程/容器及输出资源错误均停止批次。组合错误会检查所有分支，业务失败与 Docker 清理失败并存时仍停止。调用失败的原因若为基础设施错误同样停止。

整体错误只返回已经最终处理的输入前缀；其中成功项已提交，业务失败项已回滚。当前项和所有未提交快照被清理，清理错误合并返回。后续项可能已推测执行，但不能提交或对外发布事件。没有整批回滚、部分成功重放或自动续跑；调用者可根据返回前缀长度识别尚未处理项，但故障后重试需先确认宿主资源状态。

单项执行超时从执行开始计算，不包含窗口队列及等待提交时间；调用方 context 限制整个批次。自定义 Executor/Store 必须遵守 context。事务执行失败或回滚错误不会把事件回放至可变 Host。

## MVCC 内存实现

每个被写入的 `(Contract, Key)` 使用独立身份令牌，按访问键与快照令牌比较。无关键的提交不会冲突；同键盲写和读写依赖会冲突，避免丢失更新和写偏斜。版本不使用会溢出的整数，也不以字节值相等代替版本相等。

删除保留 tombstone 版本，删除缺失键同样产生版本，因而缺失→创建→删除及相同值写回都不能绕过验证。快照及提交代际不可变，对外字节/读写集合均为副本，读写集合唯一排序、结束后仍可检查。取消或验证失败发布零个键，失败事务可继续回滚，Rollback 幂等且不依赖有效 context。

当前实现每次有写提交复制状态及版本索引，历史键的 tombstone 不回收；活动快照保留旧代际。它用于验证语义，不是持久化或大规模状态数据库。

## 配置与使用

完整配置见 [`presets/parallel.yaml`](../presets/parallel.yaml)，使用严格模式的本机 Process Runner。需要 OS 隔离时按插件指南组合 Docker 编译与执行；WASI 使用其目标校验配置，均不改变 batch 插件。

```yaml
root: app
plugins:
  state:
    use: state/mvcc
  # engine/default 的 state 依赖也指向 state，其余依赖照常配置。
  batch:
    use: batch/ordered
    config: {workers: 4, maxBatch: 1024}
    deps: {state: state, executor: engine}
  app:
    use: engine/batched
    deps: {engine: engine, batch: batch}
```

`workers` 默认 4，显式范围 1–64；`maxBatch` 默认 1024，范围 1–65536。0 表示默认，负数或超范围拒绝。构造阶段不创建 goroutine、不打开资源，goroutine 仅在执行窗口存在并在返回前等待结束。成功 Commit 关闭事务，其余事务始终执行 Rollback。

```go
rt, err := assembly.Load(ctx, configReader)
if err != nil { return err }
defer rt.Stop(context.Background())
batch, ok := rt.Engine.(core.BatchExecutor)
if !ok { return fmt.Errorf("batch capability not configured") }
items, err := batch.ExecuteBatch(ctx, requests)
// 即使 err 非 nil，items 对应的成功前缀仍已提交。
```

也可直接用 PluginKit `build.Build[core.BatchExecutor](ctx, graph, "batch")` 构建独立批量能力，并按返回的实例列表管理插件生命周期。门面的 Engine 与 batch.executor 必须指向同一业务 Engine，Engine 与 batch.state 必须指向同一状态实例，示例已显式连接；PluginKit 不负责证明任意自定义配置的业务一致性。

## 验证

真实严格合约测试对照直接串行 Execute 与 workers=1/2/4 的结果和最终状态，包含过期业务失败转成功、只读结果、增量写、删除、失败/OOG、跨合约成功和父失败回滚。低层覆盖并发上限、取消、初始化失败、回滚失败、基础设施错误停止前缀、冲突重执行与二次冲突停止，另验证仅实现最小能力的自定义 PluginKit 插件及非验证状态插件拒绝。

MVCC 测试覆盖独立写、同键盲写、写偏斜、缺失/删除 ABA、只读验证、副本、context 和并发方法。D3 不声称经过跨主机共识测试或对 Docker/WASI 分别测过并行吞吐。
