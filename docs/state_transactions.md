# 状态事务（C1）

C1 提供 `core.StateStore` / `core.StateTransaction` 及 `state/memory` 插件。状态以 `StateKey{Contract, Key}` 为复合键，值为不透明 `[]byte`。合约地址和键允许空字符串，权限与地址合法性由调用方负责；不同合约的同名键互不影响。

## 使用

```go
import (
    "context"
    "github.com/lengzhao/vm/core"
    stateplugin "github.com/lengzhao/vm/plugins/state"
)

func update(ctx context.Context) error {
    store, err := stateplugin.New(stateplugin.Config{})
    if err != nil { return err }
    tx, err := store.Begin(ctx)
    if err != nil { return err }
    defer tx.Rollback()
    key := core.StateKey{Contract: "contract-A", Key: "counter"}
    if err := tx.Set(ctx, key, []byte("1")); err != nil { return err }
    // 同一个 store 上后续 Begin 才能读取已提交结果；宿主应保留 store。
    return tx.Commit(ctx)
}
```

PluginKit 可直接构建状态能力，或作为宿主插件的依赖：

```go
cfg := map[string]any{
    "state": map[string]any{"use": "state/memory"},
}
store, _, err := build.Build[core.StateStore](ctx, cfg, "state")
```

上例还需导入 `github.com/lengzhao/pluginkit/build`，并导入 `plugins/state` 或空白导入 `plugins/all` 以注册插件。使用方依赖 `core.StateStore`，不要求生命周期、配置查询或其他 VM 能力。同一图中的同实例引用共享状态，不同构建得到独立 store。C2 已将状态作为依赖加入默认 Engine 图；`assembly.Build` 的根节点仍要求 Engine，独立状态构建使用 PluginKit。

## state/memory 事务语义

- `Begin(ctx)` 引用当时的不可变一致快照，不复制全库。后续提交发布新的 map，不改变已打开事务的读取视图；事务可读到自己的写入和删除。
- `Get` 返回值、存在标志和错误。不存在与已存储的空值可区分；`nil` 与空切片均表示存在的零长度值。
- `Set` / `Delete` 只改变事务本地覆盖。同键反复操作只保留最终操作。
- `ReadSet` 记录实际调用 Get 的唯一键，含不存在的键及本地写入的键；Set/Delete 不隐式加入 ReadSet。`WriteSet` 记录每键最终值或删除标志。两者按地址、键的字符串字节顺序排序，结束后仍可检查。
- 所有输入/输出字节及访问集合均与内部数据隔离，调用方可修改返回值。事务和 Store 支持并发方法调用；同一事务内并发读写的先后取决于锁顺序，宿主须控制业务执行顺序以保证回放一致。
- `Commit(ctx)` 在锁内原子发布全部修改。带写入的事务要求 Begin 后没有其他写事务提交；否则返回 `core.ErrStateConflict`，任何键都不发布，包括不相交写集合也保守冲突。宿主需回滚后重新 Begin 并重执行业务，不能盲目重用旧快照。
- 纯读事务提交不改变状态版本，可在旧快照上成功结束。删除不存在键等带写操作也推进版本。版本使用内存身份令牌，不存在整数溢出回绕。
- 成功提交结束事务，后续读写及 Commit 返回 `core.ErrStateClosed`。Commit 失败保留未结束事务，便于检查集合及 Rollback；不能假设失败提交会自动清理。
- `Rollback()` 丢弃待提交修改并结束事务，重复调用或提交后调用均成功。结束后保留访问集合供检查，但不再保留完整读取快照。

## 取消与边界

Begin 的 context 约束整个事务；每次读写/提交还检查该操作的 context。预取消、等待锁期间取消或在准备提交期间观察到取消，均不发布数据；与最终发布并发的取消按提交时的线性化顺序处理。单次操作 context 取消不终止仍有效的 Begin context。错误保留 `context.Canceled` / `context.DeadlineExceeded`，可用 `errors.Is` 判断。Rollback 不使用 context，确保宿主请求取消后仍可清理。

内存实现的 Begin 共享不可变快照，写事务 Commit 复制全量键映射并复用未修改的值；输入输出字节仍独立复制。不持久化、不提供跨进程协调。C2 已将 Engine / ProcessRunner / contractapi 的默认 Object 接入事务，失败、OOG、DryRun 不提交，事件仅随成功提交返回，见 [Object 与执行](object_runtime.md)；跨合约共享临时 store 已由 C3 接入，见 [跨合约调用](cross_contract.md)，D3 已提供可选的细粒度冲突检测与有序并行插件，见下节。


## 可选 D3 MVCC 后端

`state/memory` 保留全局版本的保守提交行为。选择 `state/mvcc` 后按读取/写入键的版本令牌检测冲突，独立键写入可共同提交，包含缺失键及删除的 ABA 防护，纯读 Commit 也验证。MVCC 额外提供 SnapshotStore / SnapshotTransaction.Validate 供批量调度，未扩大原 StateStore 契约。内存实现复制代际并保留 tombstone，不持久化、不回收历史键版本。完整边界见 [有序并行执行](parallel_execution.md)。


## 可选 E1 持久化后端

`state/bbolt` 提供相同 StateStore/SnapshotStore 能力及独立 Start/Stop 生命周期。未启动或已关闭/故障返回 ErrStateUnavailable。提交边界 I/O 错误有明确的 ErrStateCommitUncertain 异常：磁盘结果可能已提交，Rollback 不承诺撤销；停止实例，重开核对后再重试。除该异常外保留失败提交零发布。完整配置、恢复和耐久性边界见 [持久化状态](persistent_state.md)，不替换默认内存插件。

E3 可选对象服务复用本事务，并将元数据/owner/字段编码为同一保留键，参见 [对象模型](objects.md)。原始状态接口是可信能力，不负责鉴权。


## 快照复制成本

内置 Memory、MVCC（包括 bbolt 的内存镜像）使用 256 分片的不可变快照表。Begin 共享当前代；写提交只复制被写入的分片，每个涉及分片在一次提交中最多复制一次，未变分片由各代共享。分片使用进程内随机哈希，不参与 Gas、地址、回执或持久化格式。对外值仍复制，Memory 的全库 revision 冲突与 MVCC 的按键/ABA 冲突语义不变。

单键更新不再复制全库，但仍随对应分片大小增长；大写集合可能触及全部分片。bbolt 仍在启动时加载完整状态，版本墓碑仍在进程生命周期内保留。可用 `go test ./internal/state -run '^$' -bench BenchmarkSingleKeyCommit -benchmem` 对比不同状态规模。
