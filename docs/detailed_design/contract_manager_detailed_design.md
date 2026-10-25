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
type ContractCreator interface {
    Create(context.Context, string, *CompiledContract) error
}
type ContractDeployer interface {
    Deploy(context.Context, DeploymentRequest) (string, error)
}
```

`internal/storage` 只负责完整产物记录持久化，由 `storage/filesystem` 注册。`internal/deployment` 使用 ContractCreator 和 ArtifactValidator，负责地址分配、产物接受规则与部署，由 `deployment/default` 注册。引擎查询仅依赖 ContractReader。

## 文件布局

```text
{directory}/{address}/
  ├── artifact          # 必需的可执行文件
  ├── abi.json
  ├── metadata.json     # 清单摘要与部署输入
  ├── manifest.json
  └── gas_profile.json  # 可选
```

仅按固定文件名读取，旧格式应重新编译部署。元数据保留 address/source_hash/deploy_time/status/storage_path/protocol_version/source_policy/guarantees，并持久化清单摘要及部署记录。ABI、协议、清单与部署版本继续校验。

Put 不修改调用方的 CompiledContract；相同地址表示完整记录替换，不保留旧可执行文件或旧可选 Gas 画像。先写临时快照，在实例锁保护下发布，失败时恢复旧快照。Put 的跨实例替换需要外部协调。Create 单独提供不覆盖的原子目录发布，相同有效部署幂等重试，不同或损坏的已有记录返回冲突。

## 部署流程

1. 检查调用方 context 和 ArtifactValidator。
2. 按显式 ChainID、Sender、Nonce、Salt 与清单摘要派生版本化地址。
3. 给副本附加部署记录，校验后交给 ContractCreator。
4. 返回地址；原编译产物的 Address、ExecutablePath 与 Deployment 保持不变。完整输入及冲突规则见 [确定性部署](../deployment.md)。

挂起、销毁、升级和源码模块发布仍为后置能力。

## 旧 API

根包 `vm.ContractManager` 由 `internal/compat` 适配，保留 Deploy/GetContract/GetContractABI/GetGasProfile/StoreContract/LoadContract。ABI/Profile 查询是完整产物的投影；GetContract 与 LoadContract 共享同一读取入口。旧 API 需要的路径、Address 与 Deployment 更新由兼容层完成。旧 Deploy 使用固定本地域，相同产物重复部署返回同一地址。

历史合约管理回归测试保留在 `internal/builtin/contract_test.go`；新存储测试覆盖保证字段持久化、取消无写入、输入不变和同地址完整替换；部署测试验证只需 Creator、输入无效或拒绝时不创建，以及固定向量、同身份重试、跨实例并发和冲突不覆盖。
