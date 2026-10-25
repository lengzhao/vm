---
name: vm-plugins
description: Use when adding, changing, integrating, or reviewing PluginKit-based VM plugins, capability interfaces, dependency graphs, lifecycle, or legacy API adapters in this repository.
---

# VM 插件开发规范

目标：每个接口表达一个具体、自洽的能力；按消费者需要声明依赖；同一个插件可以实现多个窄接口。保留兼容 API 不意味着扩大插件契约。

## 开始工作

遵守仓库 AGENTS.md 的 CodeGraph 初始化与定位要求。先看本次涉及的 core/cap 契约、实际调用方、插件 Config/Deps 和配置图；接口职责与现状详见 [插件指南](../../../docs/plugins.md)。

## 分层与接口

| 层 | 职责与依赖 |
| --- | --- |
| core / cap | 稳定数据和最小能力契约，不依赖实现或插件工厂 |
| internal/<能力> | 实现该能力；只依赖契约和确有必要的共享领域工具 |
| plugins/<能力> | Config/Deps 构造及 PluginKit 注册，依赖对应实现 |
| assembly | 配置读取、PluginKit 构建及可选 Start(ctx)/Stop(ctx) |
| 根 vm / internal/compat | 旧签名、白名单查询、历史 getter、展示方法的适配 |

- Compiler 只要求 `Compile(ctx, source)`；Reviewer 只要求 `Review(source)`。
- ContractStore 组合 `Put/Get`；Deployer 独立负责地址和部署规则；ABI/Profile 查询可投影 artifact。
- Engine 只组合业务能力，不要求 Stop、GetConfig、String 或历史结果 getter。
- GasProfiler 围绕同一模型自洽；Compiler 只依赖 GasProfileBuilder，估算消费者只依赖 GasEstimator。不要为了单方法机械拆成不配对的插件。
- SourcePolicy 保留 Review + ID；确定性通过 PolicyGuarantor 声明，ArtifactValidator 独立检查身份及保证。禁止从策略 ID 字面值推断保证。
- 注册复用 `pluginkit.Register`，装配复用 `build.Build`；不新增并行注册中心或在执行路径创建默认子模块。
- `internal/builtin` 仅保存历史回归测试，不得成为生产插件的实现依赖。

## 实施与验收

1. 给新增能力或边界先补失败测试；更新具体实现、工厂和默认/YAML 图。
2. 业务操作贯穿调用方 context；业务 CallContext 不能替代 context.Context。工厂不获取活动资源。
3. 取消、回滚、存储覆盖、策略/模型版本、并发 Gas 等注意事项，按本次改动读取 [pitfalls.md](references/pitfalls.md)。
4. 用仅实现最小接口的自定义插件验证可替换性，保留旧根包 API 回归。接口变更同时更新 docs/plugins.md 和示例。
5. 从仓库根目录依次运行，避免普通/race 两套真实编译流水线重叠：

```sh
go test -p 1 -timeout 5m ./...
go test -race -p 1 -timeout 5m ./...
go vet ./...
git diff --check
```

受限环境默认 Go 缓存不可写时，把 GOCACHE 设为可写临时目录；不要修改项目代码来绕开缓存权限。测试同步子进程启动信号，分开启动等待和取消响应预算，不能仅放宽超时来掩盖取消失效。

行为场景自检见 [scenarios.md](references/scenarios.md)。用户要求提交时显式核对暂存内容，排除 CodeGraph 索引、构建/运行产物和缓存；技能不额外授权提交、推送或部署。
