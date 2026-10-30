# 详细设计入口与当前功能

当前实现使用 PluginKit 装配；根包提供旧 API 兼容门面。新集成使用 `assembly` 和 `core`，以对应能力文档及源码为准。

## 当前已实现

1. 源码策略、安全审查、ABI schema v1 / 调用协议 v3、Go 编译与 Gas 插桩。
2. 版本化产物清单、内容摘要、确定性部署身份及地址。
3. Process / Docker / WASI-wazero 后端；Docker 同时支持编译隔离。
4. 链上下文、事件、默认对象和跨合约共享事务；失败/OOG/DryRun 回滚。
5. 可选 MVCC、有序推测并行、bbolt 持久化、可恢复批次与回执重放。
6. 可选多对象、owner 授权/转移/销毁。
7. 可选数据库一致备份与容量统计，`vmstate` 离线备份/恢复工具。

## 文档导航

- [架构](../architecture.md)、[插件契约](../plugins.md)
- [开发者指南](../developer_guide.md)、[合约 API](../default_library.md)、[调用协议](../call_protocol.md)
- [Gas](../gas_metering.md)、[确定性](../deterministic_execution.md)、[产物](../artifact_manifest.md)
- [沙箱](../sandbox_roadmap.md)、[后端比较](../backend_comparison.md)
- [状态事务](../state_transactions.md)、[多对象](../objects.md)、[跨合约](../cross_contract.md)
- [并行](../parallel_execution.md)、[持久化](../persistent_state.md)、[恢复批次](../recoverable_batches.md)
- [备份恢复与容量运维](../operations.md)

## 明确边界

核心库已提供可验证执行能力；默认配置用于开发，不包含生产宿主认证。链级共识/重组、签名和 nonce、资产标准、升级迁移、复合公开 ABI、TinyGo、分布式存储不属于当前实现。

静态 Gas 是启发式估算，不保证上界；每项批次原子提交不等于整批原子提交；回执重放不等于外部事件恰好一次。详细设计旧章节中的算法说明应与以上当前能力文档一起阅读。

## 验证入口

`make check` 顺序执行普通测试、race、vet 和差异检查。真实 Docker 测试需要 `VM_DOCKER_IMAGE` / `VM_DOCKER_GO_VERSION`，独立 CI 负责启用。变更不得只运行会跳过沙箱测试的默认命令后宣称沙箱已验证。
