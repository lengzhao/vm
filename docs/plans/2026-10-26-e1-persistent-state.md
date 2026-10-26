# E1 持久化状态插件实施计划

**Goal:** 新增可选 state/bbolt，支持已确认的重启恢复、原子提交与故障恢复范围，保留现有内存插件。

**Architecture:** 复用 MVCC 的不可变内存快照和按键冲突校验。写事务先准备新代际，在同一提交锁内用 bbolt 原子落盘，再发布内存代际；快照不长期占用数据库读事务，避免持有读事务时写入扩容死锁。构造不打开文件；Start 打开独占数据库、检查schema并加载，Stop 失效现有事务后关闭。默认配置不变，可替换 D3 batch.state。

**Tech Stack:** Go、PluginKit、固定稳定版本 go.etcd.io/bbolt（官方代理及 checksum DB 校验）。

## 设计边界

- 首版单实例/单进程拥有数据库文件，依赖库的文件锁拒绝第二写实例，非分布式数据库。
- 数据为二进制复合键（长度前缀合约地址+原始字段），空值与缺失可区分，schema v1。不回收/迁移未知schema，不静默清空坏数据库。
- 活动事务和冲突令牌不跨重启；Stop 同步取消旧事务，新实例从持久化值重建版本代际，不需要持久化旧快照。
- 同步提交开启，不提供 NoSync。成功提交表示数据库已同步；存储设备/OS必须兑现fsync。异常退出恢复测试不等于断电硬件认证。
- 在数据库 Commit 之前的验证、参数或取消错误保证无写发布；Commit I/O错误可能结果不确定，新增可 errors.Is 的 ErrStateCommitUncertain，并隔离该实例直到 Stop/新实例重新打开。普通 Rollback不能撤销可能已落盘的事务；调整核心契约注释明确该异常，不扩大接口。
- Start/Stop/Open/磁盘sync检查context但内核I/O不承诺硬实时抢占；不启动后台goroutine。构造无活动资源，直接PluginKit用户自行Start/Stop，assembly自动管理。
- 父目录由调用方提前创建。启动时同步数据库所在目录，确保首次创建目录项持久性；不承诺备份、迁移、加密、跨进程共享写和区块整批原子提交。

## 步骤（失败测试优先）

1. `internal/state/bolt_test.go`：先失败覆盖构造无I/O、生命周期、重开空值/删除/二进制键、冲突/ABA、取消、独占锁、坏schema与文件拒绝，再实现`internal/state/bolt.go`和MVCC提交持久化钩子。
2. 故障注入验证提交前失败零发布、提交后错误报告不确定并隔离（不假称回滚），新实例重开确认真实磁盘结果；辅助子进程验证提交后无Stop强退与未提交退出的恢复。
3. 注册`state/bbolt`，`assembly/persistent_state_test.go`真实合约与跨合约失败/OOG/DryRun、D3批量及重开验证；`presets/persistent.yaml`示例。
4. 文档`docs/persistent_state.md`及状态/插件/README/todo同步；独立代码审查修复。
5. 最终固定代码后依次全量普通、race、vet、diff检查，Docker用例显式启用；不自动提交或推送。

## 验收结果

13 个新增测试及全量普通/race/vet/diff 检查通过，真实 Docker 回归启用，测试容器无残留。首轮旧业务错误输出测试在原 1 秒预算超时；未改代码/预算，隔离连续 10 次 race 和完整 race 重跑通过，详见 todo.md 第十二批验收记录。独立审查无明确 P1/P2。实现、配置和文档保留 dev 工作区，未提交/推送。
