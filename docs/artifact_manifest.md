# 产物清单与完整性校验（B3）

内建 Go 编译流水线只产生清单版本 1 的产物。没有清单或版本不兼容的旧产物必须重新编译部署，不提供旧格式接受路径。

## 清单内容

`core.CompiledContract` 携带 `Manifest` 和 `ManifestSHA256`。清单记录：

- `version`：清单 schema，当前为 1；`protocol_version` 与 `abi_version` 分别绑定调用协议与 ABI schema。
- `build.compiler_version`：编译流水线版本；当前为 `pipeline-v13-cross-contract`。
- `build.toolchain`：实际 Go 版本、Go 可执行文件 SHA-256、GOOS/GOARCH、CGO 开关及固定构建参数；Docker 编译额外记录可选 `container_image` 镜像 ID，并纳入缓存与摘要。
- `build.source_sha256`：原始源码字节摘要；`injected_sha256` 和 `entry_sha256` 是实际插桩源码及生成入口摘要。
- `build.runtime_version` / `gas_version`：当前为 `contractapi-v3-call` / `go-gas-v3-call`；`runtime_sha256` 绑定内嵌的 contractapi、Object、Call、Gas 与协议源码。
- `binary_sha256`：完整可执行文件摘要。
- `abi_sha256` / `gas_profile_sha256`：ABI、GasProfile 的规范 JSON 摘要；没有画像时摘要输入为 JSON `null`。
- `metadata_sha256`：以下对象的规范 JSON 摘要，字段顺序依次为 `source_hash`、`source_policy`、`guarantees`、`protocol_version`，值来自 CompiledContract。

摘要均为完整、小写的 SHA-256 十六进制字符串。规范 JSON 是 Go `encoding/json.Marshal` 对相应 core/abi 类型的输出，不带缩进；map 键按该编码器排序。`ManifestSHA256` 是整个 Manifest 的同种规范 JSON 摘要。`runtime_sha256` 的输入是 `[contractapi.Source, contractapi.GasSource, contractapi.ProtocolSource, contractapi.ObjectSource, contractapi.CallSource]` 的 JSON 编码。

`SourceHash` 为历史命名，继续表示构建缓存身份，现在使用完整的 64 字符十六进制摘要；原始源码摘要应读取 `Manifest.Build.SourceSHA256`。部署地址、路径和时间不参与编译内容摘要；B4 的版本化地址规则及部署身份绑定见 [确定性部署](deployment.md)。

## 缓存与部署目录

```text
编译输出目录/
  contract_<构建身份>/
    artifact
    manifest.json
    manifest.sha256

合约存储目录/
  <address>/
    artifact
    manifest.json
    metadata.json       # 包含 manifest_sha256
    abi.json
    gas_profile.json    # 存在画像时写入
```

编译器在私有构建目录中写完二进制、清单与摘要，再原子发布整个缓存目录。相同源码并发编译时，失败的发布者验证已发布的完整条目后复用。缓存身份覆盖工具链与流水线、生成代码、ABI、画像、策略和保证；命中前还会与当前预期清单比较并校验二进制。损坏缓存明确失败，不会给损坏内容重新生成清单；清理该损坏缓存条目后可重新编译。

构建使用 `-mod=mod -trimpath -buildvcs=false -ldflags=-buildid=`，固定环境与工具链配置见 [插件指南](plugins.md)。这些参数减少环境差异，不代表已建立跨平台、跨 Go 安装的可复现构建规范。

Store 的 Put 先验证输入，再写临时完整快照，并校验实际复制的二进制后替换记录；失败保留旧记录。Get 读取固定的 `artifact` 文件名，不再从目录里猜测可执行文件。损坏或缺失的画像不能被静默忽略，只有清单中绑定的 `null` 画像允许没有画像文件。

## 运行规则与插件边界

- Store 负责清单结构和内容完整性，不负责决定可接受的源码策略或自定义 runtime。
- 默认 ArtifactValidator 同时检查源码策略、保证、清单完整性及 native runtime/Gas 规则兼容性。Engine 的加载、执行和静态估算均调用 validator。
- ProcessRunner 在调用前也验证完整性和原生规则，拒绝不同目标平台、启用 CGO、未知 runtime/Gas 版本或不匹配的 runtime 摘要。
- 版本不兼容返回 `UNSUPPORTED_VERSION`，ABI 不受支持返回 `UNSUPPORTED_ABI`，摘要或内容不一致返回 `ARTIFACT_INTEGRITY`；错误保留底层 cause。取消通过 context 传播，二进制摘要采用流式计算。
- 自定义编译器是受信任生产者，需提供完整清单及匹配摘要；自定义 runtime 的兼容规则由相应 ArtifactValidator 实现。无需扩大 Compiler、Runner 或 Store 的能力接口。

## 可信边界

摘要校验用于检测产物与清单不一致，不认证生产者身份，也不证明二进制确由记录的源码产生。源码、插桩和工具链输入在部署端不一定存在，因此消费端检查这些摘要的格式和绑定关系，不尝试重新构建。能同时改写产物及所有配套摘要的操作者仍可伪造记录；签名、可信发布链、操作系统隔离和消除路径校验与执行之间的竞争窗口不属于本批。
