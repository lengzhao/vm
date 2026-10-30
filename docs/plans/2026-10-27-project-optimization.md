# Project Optimization Implementation Plan

**Goal:** 降低单键状态提交的复制成本，限制编译前端资源，并修正项目开发入口。

**Architecture:** 沿用已审查的优化方向。Memory/MVCC 使用共享不可变分片表，提交只复制被写入的分片；冲突令牌、锁顺序、持久提交后不可失败的发布语义不变。编译器在源码解析前执行大小检查并建立总预算，诊断保留有限前缀。core 接口和数据库格式不变。

**Tech Stack:** Go 1.26.1、现有 PluginKit、bbolt。

## 执行步骤

1. 新增单键提交分配预算回归和可重复 benchmark，运行旧实现确认失败与基线；新增跨多分片旧快照隔离测试。
2. 内部引入 256 分片不可变表，随机种子的哈希防止可预测分片热点；一次提交只复制涉及分片，每个分片最多复制一次。Memory/MVCC 使用此表，bbolt 启动在未发布表上直接初始化。
3. 新增编译源码超限、前端耗时计入总预算、诊断截断回归，先确认失败。添加 maxSourceBytes（默认 1MiB，可配置，负数拒绝）；buildTimeout 从 Compile 入口计时，解析阶段之间检查取消。同步插件配置和文档。
4. Makefile 按库调整 build/run/install/跨平台校验，添加顺序执行的 check 与 race，增加 CI；更新过时 README。保留已有测试预算，不通过放宽超时解决问题。
5. 运行针对性测试和前后基准。依次执行全量普通测试、race、vet、diff check；核对旧兼容、持久恢复、对象授权和插件替换测试。

## 后续边界

本轮不删除 Store/Validator/Runner 的独立完整性校验；可变路径缓存会破坏安全边界。首轮 race 在 WASI 事务集成测试中因重复 JIT 耗尽每包预算，因此扩展本轮实现有界编译缓存：每个 Runner 常驻最近一个二进制摘要的缓存，活跃旧代以 lease 保留到 runtime 关闭；Stop(ctx) 等待清理且允许取消后重试。同代 CompileModule 串行，InstantiateModule 独立；不复用 guest 内存或 Host，不改变超时。针对性 race 的 WASI 事务测试由超时栈记录的 4m27s 未完成降为约 13s 完成。这不是三后端完整性能对比。不会为减少重复哈希而扩大 core 契约。

## 兼容性与限制

- 保持 Memory 全库 revision 冲突语义和 MVCC 按键/ABA 检测语义。
- 分片复制仍随该分片大小增长；不宣称提交与全库规模完全无关。
- bbolt 仍在启动时将状态载入内存；墓碑回收与磁盘快照架构不在此次重构内。
- 同步插件不能被 context 强制抢占，大小限制和阶段边界取消检查不等于硬 CPU 隔离。

## 验证结果（2026-10-27）

- 新增分配回归在旧实现上先失败：Memory/MVCC 约 8 MiB / 14 MiB；分片后 10 万键单键更新基准约 36,248 / 60,424 B/op。基准为本机单轮平均值，不是端到端吞吐或 p95。
- 源码限额、前端总预算、真实子进程诊断截断回归均先失败后通过。
- 新增 WASI 缓存生命周期、淘汰旧代、Stop 超时重试、取消分类及两个真实 guest 并发取消测试。独立代码复核两轮未发现阻断 P1/P2。
- 固定最终功能代码后，全量普通测试、全量 race、go vet、git diff --check 均通过。普通 assembly 20.413 秒，race assembly 62.564 秒；未修改原有超时预算。首轮 race 超时后修正重复 JIT 路径，保留失败日志作为诊断依据。
- make build、make run 验证通过，basic 示例返回 30。Docker 真实隔离测试本轮未显式启用；新增 GitHub CI 配置尚未在远端运行。
