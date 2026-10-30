# 开发者指南

## 选择入口

新项目通过 `assembly.Load(ctx, reader)` / `assembly.Build(ctx, cfg)` 创建 Runtime，调用 `rt.Engine` 的 `Compile`、`GenerateABI`、`Deploy`、`Get`、`Execute`、`EstimateGas`，全部接收 context。结束后用独立、有期限的清理 context 调用 `rt.Stop`，不要复用已取消的请求 context。

根 `vm.NewVMEngine` 保留历史签名；并发请求使用本次 `ExecuteResult`，不要通过最近结果 getter 关联调用。核心接口见 `core/capabilities.go`、`core/contract.go`；扩展契约见 [插件指南](plugins.md)。

## 从最小合约到状态合约

1. 运行 `go run ./examples/basic` 验证本地工具链。
2. 使用 [默认配置](../presets/default.yaml) 接入开发环境；它使用内存状态与 ProcessRunner。
3. 运行完整示例：

```sh
go run ./examples/integration presets/objects.yaml
```

示例编译真实合约、以显式部署身份部署、创建 owner 为 alice 的对象、写入并读取字段、提交恢复批次，然后停止/重开数据库并对同 ID 重放回执。重复运行使用同一部署与批次身份，不重复创建对象。示例生成 `state.db` 和 `contracts/`；正式集成应显式选择工作目录并保存配置/请求。

`alice` 是示例中的可信身份声明。业务宿主必须验证请求签名、nonce、防重放，再构造 `CallContext.Sender`；把 HTTP 输入直接写入 Sender 会失去授权边界。对象 owner 转移不代表资产转账。

## 组合隔离、持久化与恢复

[objects-docker.yaml](../presets/objects-docker.yaml) 组合严格确定性策略、bbolt、对象 owner 授权、可恢复批次和 Docker 编译/执行后端。按 [沙箱指南](sandbox_roadmap.md) 准备本地镜像，用完整不可变 ID 替换 IMAGE_ID、实际工具链版本替换 GO_VERSION，再运行：

```sh
go run ./examples/integration /absolute/path/to/configured-objects-docker.yaml
```

配置本身是模板，含占位符时不能运行；没有自动拉取或降级。容器资源上限、整体并发和业务预算需要按负载设置。切换执行规则、授权或 Gas 配置时更新 `executionID`；已有批次须在原规则下完成，不能用同 ID 静默切换规则。

## 调用与错误

公开函数只接受/返回协议支持的标量，最后一个 error 表示失败。复杂业务数据可自行定义字符串编码；复合公开 ABI 尚未提供。完整约束见 [调用协议](call_protocol.md)。大整数使用 int64、uint64 或 json.Number；避免先经过 float64。

通过 `var ee *core.ExecutionError; errors.As(err, &ee)` 识别稳定 Code，通过 `errors.Is` 检查取消、超时和 `ErrStateCommitUncertain`。后者不能盲目重试：停止故障实例，重开后使用原恢复批次请求核对回执。普通 Execute 没有同样的请求幂等账本。

- Execute：成功才提交并返回事件，失败清除业务数据和事件。
- EstimateGas 静态模式：启发式画像，不是预算保证；跨合约目标和动态路径可能漏估。
- EstimateGas DryRun：执行给定状态和输入后回滚，不发布事件；失败返回错误。
- RecoverableBatchExecutor：逐项提交，业务失败形成失败回执，基础设施错误停止并返回前缀；不是整批原子性。

事件去重键可以使用 `(batch ID, item index, event index)`，且 ID 必须在应用命名空间内唯一。重放会再次返回事件，外部消息发送需消费方持久化去重/投递状态。示例只打印该键。

## 运维与验证

[运维指南](operations.md) 包含备份、恢复、容量指标、未完成批次和事件投递规则。备份数据库时还应保存产物、完整请求、执行配置和工具链身份。

`make check` 顺序跑普通/race/vet/差异检查。Docker 专用 CI 额外显式启用隔离与集成测试；详见 `.github/workflows/docker.yml`。本地应先设置 VM_DOCKER_IMAGE 与 VM_DOCKER_GO_VERSION，不能将 Skip 当作真实隔离验收。
