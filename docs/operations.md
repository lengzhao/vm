# 持久化运维：备份、恢复与容量

## 能力与边界

`state/bbolt` 可选提供 `core.StateBackupper.Backup(ctx, io.Writer)` 和 `core.StateInspector.StateStats(ctx)`。Engine、StateStore 契约不变；从 `rt.Result.Instances` 找到 state 实例后按这些窄接口断言。内存后端不伪装支持持久备份。

Backup 使用一致的 bbolt 读事务，包含所有已提交状态、批次身份、连续回执和 pending 标记，排除未提交快照。为避免 mmap 扩容和关闭死锁，备份/统计与 Commit、Finalize、Stop 通过同一门限串行；大库备份和统计会阻塞提交，应设置维护窗口。任意阻塞 Writer/同步磁盘 I/O 不能被 context 强制抢占；取消在阶段和写调用间检查，调用方负责 Writer 的期限及错误时丢弃半成品。Writer 不能回调同一个 store。

## 离线 CLI

先停止数据库拥有者。CLI 只读打开源并使用共享读锁，与运行中 VM 的独占写锁冲突，不会绕过锁；在线备份应调用宿主已打开实例的 Backup。源备份可位于只读介质，操作不会改变其文件摘要。

```sh
go run ./cmd/vmstate status -db /data/state.db
go run ./cmd/vmstate backup -db /data/state.db -out /backups/state-2026-10-30.db
go run ./cmd/vmstate restore -db /backups/state-2026-10-30.db -out /data/restored.db
```

源文件必须已存在、非空且具有已初始化的 VM schema；有效但未初始化的普通 bbolt 文件也会被拒绝，不会静默变为空 VM 库。不会因路径写错而创建新源库。目的路径必须不存在；不覆盖旧库。backup/restore 先写目的目录的临时文件，Sync/关闭并验证副本的 VM 启动和 schema，然后用不覆盖的原子链接发布并同步目录。status 在系统临时目录生成私有副本用于校验和扫描，因此同样需要足够临时磁盘空间。文件系统须支持同目录硬链接和目录同步；不支持时明确报错。目录同步失败时错误会指出已发布的路径，应核对它，不能据错误推断文件不存在。

backup/restore 都保留完整数据库语义；restore 是恢复到新文件，不是 schema 迁移。验证包含 bbolt 打开及 VM schema/状态载入检查，不能证明文件来源可信、业务正确或所有旧批次均可重放。若库原本使用更高回执额度，重开时追加 `-max-batch-receipt-mb 128` 等相同设置（允许 4–1024）。

## 恢复演练

1. 停止写入，取得一致数据库快照；同时固定合约产物目录、完整恢复批次请求、executionID 对应配置和 VM/工具链版本。产物存储与状态库没有跨文件事务，必须保留备份时所需的不可变产物。仅数据库文件不是完整业务备份。
2. 用 restore 恢复到新路径，用 status 核对 StateKeys、Batches、Receipts、PendingBatchID，再让新 Runtime 指向新库和原产物目录。
3. 如果 PendingBatchID 非空，提供相同 ID、内容及执行规则的完整请求续跑。恢复标记不能手工删除；没有原请求时应保留现场并恢复请求，不能放开其它写入。
4. 恢复接口先返回已提交回执，然后继续剩余项；按批次/项目/事件索引去重外部事件。普通 Execute 在提交结果不确定时仍需独立业务标识核对，不能自动重试。
5. 在隔离环境验证读取、owner、关键业务不变量与事件去重，再由宿主切换数据库路径。保留原库及备份，直到业务验证完成。

测试覆盖 pending 批次的状态+回执共同备份恢复；这不替代异机/坏盘/真实断电演练。

## 容量管理

StateStats 返回单次扫描的一致库存：DatabaseBytes（文件大小，含空闲页）、StateKeys、StateValueBytes（编码值字节）、Batches、Receipts、ReceiptBytes（编码回执字节）、PendingBatchID。不是 RSS 或磁盘剩余空间；扫描成本随数据增长，建议低频采集。

- 同时监控宿主 RSS、可用磁盘、数据库/产物目录增长、备份耗时和提交延迟；根据部署预算设置告警，不把值字节数当作总内存。
- 数据库启动加载全部状态入内存；MVCC 旧快照和对象墓碑也消耗内存。限制并发和长事务，按预期规模压测。
- 恢复回执累计默认每批 64 MiB，可配置 4–1024 MiB；这不是整库磁盘配额。单项 4 MiB，批次大小由调度器限制。达到回执额度时保留 fence，需要以更高额度重开并使用原请求续跑。
- 历史批次、对象墓碑不自动清理。删除回执会使旧 ID 的重试可能再次执行，删除墓碑会破坏 ID 不复用语义，因此本轮提供观察/限额/恢复规程，不提供无条件 purge。
- 需要归档/GC、压缩或 schema 迁移时，应先确定应用重放保留期、归档账本和升级协议，再通过独立受审查流程实施。当前没有自动迁移、降级写保护或透明跨进程共享写入。
