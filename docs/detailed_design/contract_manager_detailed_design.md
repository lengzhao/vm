# 合约存储与部署详细设计

## 核心契约

存储和部署是独立能力。core 不再暴露包含同义 Get/Load 入口的 ContractManager：

```go
type ContractReader interface {
    Get(context.Context, string) (*CompiledContract, error)
}
type ContractWriter interface {
    Put(context.Context, string, *CompiledContract) error
}
type ContractStore interface {
    ContractReader
    ContractWriter
}
type ContractDeployer interface {
    Deploy(context.Context, *CompiledContract) (string, error)
}
```

`internal/storage` 只负责完整产物记录持久化，由 `storage/filesystem` 注册。`internal/deployment` 使用 ContractWriter 和 ArtifactValidator，负责地址分配、产物接受规则与部署，由 `deployment/default` 注册。引擎查询仅依赖 ContractReader。

## 文件布局

```text
{directory}/{address}/
  ├── artifact          # 可执行文件，纯元数据旧记录可无此文件
  ├── abi.json
  ├── metadata.json
  └── gas_profile.json  # 可选
```

读取兼容原有 `contract_<hash>` 文件名。元数据保留 address/source_hash/deploy_time/status/storage_path/protocol_version/source_policy，并持久化 Guarantees。ABI 与协议版本继续校验。

Put 不修改调用方的 CompiledContract；相同地址表示完整记录替换，不保留旧可执行文件或旧可选 Gas 画像。先写临时快照，在实例锁保护下发布，失败时恢复旧快照。共享目录的多个 Store 实例或多个进程之间不提供事务协调。

## 部署流程

1. 检查调用方 context 和 ArtifactValidator。
2. 原生部署实现检查可执行文件存在。
3. 分配地址，把产物副本交给 ContractWriter。
4. 返回地址；原编译产物的 Address 和 ExecutablePath 保持不变。

挂起、销毁、升级和源码模块发布仍为后置能力。

## 旧 API

根包 `vm.ContractManager` 由 `internal/compat` 适配，保留 Deploy/GetContract/GetContractABI/GetGasProfile/StoreContract/LoadContract。ABI/Profile 查询是完整产物的投影；GetContract 与 LoadContract 共享同一读取入口。旧 API 需要的路径和 Address 更新由兼容层完成。

历史合约管理回归测试保留在 `internal/builtin/contract_test.go`；新存储测试覆盖保证字段持久化、取消无写入、输入不变和同地址完整替换；部署测试验证只需写接口和拒绝前不写入。
