# 确定性部署 v1（B4）

原生部署使用显式输入与产物清单摘要生成地址，不读取时钟、文件路径或节点本地计数器。部署输入与规则版本随记录持久化，并在加载、执行前检查一致性。

## 调用方式

```go
input := core.DeploymentInput{
    ChainID: "chain-A",
    Sender:  core.Address("alice"),
    Nonce:   42,
    Salt:    [32]byte{7},
}
address, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{
    Contract: compiled,
    Input:    input,
})
```

核心 `ContractDeployer` 只提供 `Deploy(context.Context, DeploymentRequest)`；缺少链标识或部署者会返回 `INVALID_REQUEST`。Nonce 允许 0；未显式指定 Salt 等价于 32 字节全零。原生部署不修改输入产物的 Address、ExecutablePath 或 Deployment。

根包提供 `engine.DeployWithInput(compiled, input)` 和 `engine.DeployContext(ctx, compiled, input)`；可先用 `vm.DeriveDeploymentAddress(input, compiled.ManifestSHA256)` 预测地址。根包兼容门面部署成功后仍更新传入产物的 Address、ExecutablePath、Deployment。

旧 `engine.Deploy(compiled)` 与 ContractManager.Deploy 使用固定本地域 `ChainID="vm-local-v1"`、`Sender="local"`、Nonce=0、Salt=0。同一产物重复调用返回同一地址，重试不创建新部署；链集成应使用显式输入入口。

## 地址编码

规则版本 `DeploymentVersion=1`。ChainID 和 Sender 为不经归一化的 UTF-8 字节串，各 1–256 字节。编码顺序如下：

1. ASCII `vm-deployment-v1`，追加一个 NUL 字节。
2. ChainID：4 字节大端无符号长度，随后是该字符串原始字节。
3. Sender：同样的长度前缀及原始字节。
4. Nonce：8 字节大端无符号整数。
5. Salt：固定 32 字节。
6. ManifestSHA256：解码为 32 字节的原始清单摘要，输入文本必须是 64 字符小写十六进制。

输出为 `contract_` 加上述字节序列的 SHA-256 完整小写十六进制，不截断。链标识、部署者、nonce、salt、产物任意改变都会改变地址。长度前缀避免字段拼接歧义；来源源码相同但清单不同的产物不承诺相同地址。

固定向量：ChainID=`chain-A`，Sender=`alice`，Nonce=42，Salt 为 `00 01 ... 1f`，ManifestSHA256 为 32 次 `ab`，地址应为：

```text
contract_fcef3323949e2c2397b5761bc412e1282e402308fcb0f75b2cb89d38e832aee8
```

## 记录与一致性

`CompiledContract.Deployment` 与 metadata.json 的 `deployment` 保存：

- `version`：部署规则版本。
- `input`：ChainID、Sender、Nonce、Salt（JSON 中 Salt 为固定数组）。
- `artifact_sha256`：产物的 ManifestSHA256。

声明了部署记录时，完整性检查要求其产物摘要与编译清单一致，并重新派生地址与加载的 Address 比较。未知部署版本返回 `UNSUPPORTED_VERSION`；摘要或地址不一致返回 `ARTIFACT_INTEGRITY`。有地址的产物进入内建 native validator / Runner 时必须携带部署输入；旧本地时间地址需要重新部署。

部署字段独立于编译清单，因此一个可信产物可以使用不同输入部署多次。DeployTime、StoragePath 仅为本地诊断元数据，不参与部署身份，也不作为合约链上上下文。

## 仅创建与冲突

默认 Deployer 消费 `ContractCreator.Create(ctx, address, artifact)` 和 ArtifactValidator，不依赖读接口或可覆盖的 Put。存储插件可以同时实现 Creator 与 Put/Get；`ContractStore` 仍只组合 Put/Get，消费者按所需窄接口声明依赖。

内建文件存储的 Create 验证输入并发布完整非空目录：

- 地址不存在：完整快照原子发布。
- 已存在且清单、内容、部署身份相同：验证后成功返回，不重写文件或部署时间。
- 已有其他内容，或旧记录已损坏：返回 `DEPLOYMENT_CONFLICT`，保留原记录，不修复、不覆盖。

多个 Create 实例并发发布时只有一个完整目录胜出，其他调用验证获胜记录后按上述规则处理。原有 Put 的同地址完整替换语义保留；跨实例 Put、外部直接修改文件及其与 Create 的混合写入仍需外部协调。当前不提供断电恢复或全链状态事务。

## 集成边界

DeploymentInput 是调用方提供的域数据，VM 不据此认证 Sender，也不维护账户 nonce 使用账本。链宿主应认证发起者，并决定 nonce、salt、重复交易的处理规则。B4 完成的是地址及部署身份一致性；状态快照、提交/回滚与 Object 已实现，见状态事务与对象文档。
