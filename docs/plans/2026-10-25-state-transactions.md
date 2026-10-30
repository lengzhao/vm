# C1 状态事务实施计划

**Goal:** 提供可替换的状态事务契约和经过验证的内存实现。

**Architecture:** core 定义 StateStore / StateTransaction、复合键和读写集合。state/memory 插件提供 Begin 快照及事务内读写；提交使用全局版本检查和锁原子发布。Engine 和 Object 在 C2 接入。

**Tech Stack:** Go 标准库、PluginKit、testing/race。

## 已确认设计

- Begin 是事务快照边界；键由合约地址和字符串键组成，值为不透明字节。Get 用独立 bool 区分不存在与空值。
- Get/Set/Delete 均访问事务快照和本地覆盖；输入、输出及集合均复制，集合按地址/键排序。ReadSet 包含实际读取的键（含不存在），WriteSet 保留每键最终操作。
- 成功 Commit 原子发布所有写入并结束事务；全局版本改变时有写入的旧事务报 ErrStateConflict，不自动重试。纯读事务不修改版本。细粒度冲突检测后置 D3。
- 所有业务操作检查 Begin context 和操作 context。失败或取消的 Commit 不发布任何写入，可调用不依赖 context 的幂等 Rollback 清理。已结束事务拒绝读写/提交，集合仍可检查。
- 内存 Store 与事务支持并发调用，提交线性化；数据不持久化。C1 不接入 Engine，不宣称失败/DryRun 已具备状态原子性。

## 实施步骤

1. internal/state/memory_test.go 先写行为测试，运行 `go test ./internal/state` 确认缺失能力失败。
2. core/state.go 与 internal/state/memory.go 实现最小事务，重复运行定向测试及 race。
3. plugins/state/register.go 注册 state/memory，plugins/all/all.go 导入。添加使用仅依赖 core 契约的 PluginKit 装配测试，先观察失败，再补注册实现。
4. docs/state_transactions.md 和 docs/plugins.md 说明语义、示例与边界，todo.md 标记验收。
5. 依次运行全量普通测试、race、vet、git diff --check。变更留在 dev 工作区，提交和推送由用户另行要求。
