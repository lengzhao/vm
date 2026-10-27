# E3 多对象与所有权设计

用户已批准：可选对象服务与独立授权插件；对象限所属合约访问，同合约可读，owner 可写、删除和转移；不含公开跨合约读取、共享授权或资产标准。DefaultObject 保留语义。

对象 ID 由调用方显式提供（1–128 字节 UTF-8），在合约内唯一；创建者为宿主解析的 Sender，不接受 guest 传入创建 owner。owner 必须是非空、至多 256 字节 UTF-8 地址；宿主负责认证根 Sender。子调用主体为调用合约，归属始终是实际执行地址。转移不改变归属。

对象服务 Bind 接收宿主绑定的事务与 scope，返回帧内服务；授权插件只消费 scope、对象 owner、操作。Engine 新增独立 engine/objects 工厂，默认图不变；不扩大 core.Engine、Runner 或 StateStore。服务错误锁存，忽略错误不能提交。

元数据和字段放在同一事务键，使用默认 Object 不允许的非 UTF-8 字段前缀隔离。每对象最多 128 字段、编码记录至多 1 MiB；字段仍为 1 KiB/64 KiB。删除保留墓碑，禁止 ID 重用及旧句柄复活。冲突粒度为对象；与 MVCC、bbolt 和恢复回执共享提交边界。

协议在 object 消息上添加可选 id/owner；默认请求不带扩展。新增 CreateObject/OpenObject、Owner/Transfer/Destroy。所有请求继续受次数、Gas、输入输出上限约束；新操作宿主验证最低 Gas。runtime 摘要随内嵌源码变化，旧宿主拒绝未知字段/操作。新对象配置的 executionID 必须更新，禁止旧程序对含对象记录的状态降级写入。

验证：先失败测试再实现；覆盖归属/owner/转移/墓碑/取消/复制/资源限制、默认键隔离、MVCC 冲突、失败与 DryRun 回滚、子调用 Sender、持久重启/恢复重放及最小能力插件替换。最终依次普通测试、race、vet、diff 检查。首版不提供对象枚举或墓碑清理。
