# 合约管理模块详细设计

## 1. 概述

`ContractManager` 负责合约部署、本地存储与查询。当前 MVP 使用**文件系统**，不是 LevelDB。

## 2. 接口

```go
type ContractManager interface {
    Deploy(contract *CompiledContract) (string, error)
    GetContract(address string) (*CompiledContract, error)
    GetContractABI(address string) (*abi.ABI, error)
    StoreContract(contract *CompiledContract, address string) error
    LoadContract(address string) (*CompiledContract, error)
}
```

## 3. 存储布局

```text
{ContractStorageDir}/{address}/
  ├── contract_<hash>   # 可执行文件
  ├── abi.json
  └── metadata.json
```

`metadata.json` 字段：

- `address`
- `source_hash`
- `deploy_time`
- `status`
- `storage_path`

## 4. Deploy 流程

1. 由 `source_hash` + 时间戳生成地址
2. 复制可执行文件到合约目录
3. 写 ABI 与 metadata
4. 更新 `CompiledContract.ExecutablePath` 为部署后路径

## 5. 状态

```go
const (
    ContractStatusUnknown ContractStatus = iota
    ContractStatusDeployed
    ContractStatusSuspended
    ContractStatusDestroyed
)
```

MVP 部署后固定为 `Deployed`。挂起 / 销毁 / 升级后置。

## 6. 后置

- 外部对象存储 / LevelDB
- 合约升级与版本管理
- 可被其他合约 import 的源码模块发布

## 7. 测试

`contract_test.go` 使用 `t.TempDir()`，并覆盖真实编译产物 Deploy 后再 Execute。
