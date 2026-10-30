# 合约侧 API 参考

合约导入 `github.com/lengzhao/vm/contractapi`；宿主导入 `core` / `assembly`。两者不能混用，合约不得导入宿主根包。

## 已实现 API

- 上下文：`BlockHeight() uint64`、`BlockTime() uint64`、`Sender() string`、`ContractAddress() string`。
- 事件：`Log(eventName string, keyValues ...any)`。只有成功提交返回事件；失败、OOG、DryRun 不发布。
- 默认对象：`DefaultObject() Object`，提供 `Get(key) ([]byte,bool,error)`、`Set(key string, value []byte) error`、`Delete(key string) error`。读写限于当前执行合约，共享本次事务。
- 多对象（需启用 E3 插件）：`CreateObject(id string) (Object,error)`、`OpenObject(id string) Object`；对象另提供 `Owner() (string,error)`、`Transfer(owner string) error`、`Destroy() error`。Get/Set/Delete 复用同一对象类型。ID 为合约范围内调用方指定的唯一字符串。
- 跨合约：`Call(contract, function string, args ...any) ([]byte,error)`，共享整条调用链的状态事务、Gas 和有序事件；子调用失败使整链失败。

`OpenObject` 只创建句柄，实际操作时检查存在及权限；不提供按 owner 列举对象。根 Sender 由宿主认证，子 Sender 是调用合约。对象 Transfer 只转移所有权，不是货币转账。

详见 [默认对象](object_runtime.md)、[多对象](objects.md)、[跨合约](cross_contract.md)、[调用协议](call_protocol.md)、[开发者指南](developer_guide.md)。

## 运行时保留 API

InitGas、ResetGas、ConsumeGas、GasUsed、Reset、DrainEvents 等由编译入口管理；合约源码不能直接引用。`init`、`recover` 和 `__vm` 标识受强制完整性检查约束。

## 未提供的能力

账户余额/资产转账、共享授权、统一资产标准、哈希工具 API、对象枚举，以及合约升级迁移尚未提供。宿主可以通过窄能力插件扩展，但应版本化执行规则、明确确定性与事务边界，不能把旧目标态伪装为可用 API。
