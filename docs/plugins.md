# PluginKit 插件架构

VM 使用 `github.com/lengzhao/pluginkit v0.1.6`，与 agentkit / GoChainKit 采用相同的 `Register + Config/Deps + build.Build` 模式。宿主项目需要 Go 1.26.1；编译出的合约仍只依赖独立的 `contractapi`，不会带入 PluginKit。

## 分层与能力

- `core/`：公共数据类型、能力接口、稳定协议错误和调用上下文。
- `internal/{compiler,security,policy,gas,abigen,storage,deployment,process,engine}/`：按职责隔离的内建实现，只依赖公共契约和必要的公共实现工具。
- `internal/callcontext/`：调用上下文解析与传递；`internal/compat/`：旧 API 适配。
- `internal/builtin/`：历史白盒回归测试，不进入生产插件装配路径。
- `plugins/`：每种能力的 Config/Deps 构造与注册；第三方插件只需实现 core 接口。
- `assembly/`：配置读取、默认图、PluginKit 构建和实例生命周期。
- 根 `vm` 包：现有 API 的兼容入口，默认使用插件图。

插件种类：

| use | 能力 | deps |
| --- | --- | --- |
| `security/go` | AST 安全审查 | 无 |
| `abi/go` | ABI 生成 | 无 |
| `policy/go` | 开发/确定性源码策略 | reviewer |
| `policy/artifact` | 产物策略身份与保证校验 | policy |
| `gas/injector` | Gas 插桩 | 无 |
| `gas/profiler` | 静态画像与估算 | 无 |
| `gas/meters` | 每次调用的计量器工厂 | 无 |
| `compiler/go` | 独立进程 Go 编译 | policy, abi, injector, profiler |
| `storage/filesystem` | 合约产物 Put/Get 存储 | 无 |
| `deployment/default` | 部署规则与地址分配 | store, validator |
| `runner/process` | 子进程执行 | 无 |
| `engine/default` | VM 编排 | policy, abi, compiler, deployer, contracts, runner, meters, estimator, validator |
| `engine/components` | 兼容程序化 Runner 注入的依赖束 | 同上 |

Host、CallContext、事件和单次调用的 Gas 计量器是每次调用的数据/实例；不注册成共享的全局插件。`gas/meters` 必须每次返回一个独立计量器。

## 使用配置

```go
import (
    "context"
    "os"
    "github.com/lengzhao/vm/assembly"
    "github.com/lengzhao/vm/core"
    _ "github.com/lengzhao/vm/plugins/all"
)

file, err := os.Open("presets/default.yaml")
if err != nil { return err }
defer file.Close()
ctx := context.Background()
rt, err := assembly.Load(ctx, file)
if err != nil { return err }
defer rt.Stop(context.Background())
compiled, err := rt.Engine.Compile(ctx, source)
if err != nil { return err }
address, err := rt.Engine.Deploy(ctx, core.DeploymentRequest{
    Contract: compiled,
    Input: core.DeploymentInput{ChainID: "chain-A", Sender: "alice", Nonce: 1},
})
if err != nil { return err }
result, err := rt.Engine.Execute(ctx, core.ExecutionRequest{
    Address: address, Function: "Add", Args: []any{2, 3},
})
if err != nil { return err }
_ = result.Data // ExecuteResult 同时包含 Events 和 GasConsumed
```

运行 `go run ./examples/plugins presets/default.yaml`，输出 `5`。

`assembly` 本身不会隐式导入全部插件。可导入 `plugins/all`，或仅导入所需插件包及第三方扩展。根 `vm` 包为兼容旧构造函数会注册内建插件。

YAML 使用 `root/plugins` 和 `use/config/deps`，详见 [默认预设](../presets/default.yaml)。依赖引用实例 ID；同一实例被多个模块引用时由 PluginKit 共享。也支持 PluginKit 的内联依赖对象。配置只允许一个文档，最大 1 MiB；未知顶层和插件 Config 字段会报错。

Go 中可编辑默认图：

```go
cfg := assembly.DefaultConfig(core.VMConfig{
    ContractStorageDir: "./contracts",
    EnableSecurityChecks: true,
})
cfg.Plugins["runner"] = map[string]any{"use": "my/runner"}
rt, err := assembly.Build(ctx, cfg)
```

`DefaultConfig` 保留传入布尔值，包括显式 false；安全、确定性和 Gas 不会被悄悄开启。预设文件则显式开启安全审查与 Gas。超时配置使用 `30s` 等 Go duration 字符串。

## Go 编译工具链

`compiler/go` 可配置 `goBinary`（Go 可执行文件路径或命令名）与 `goVersion`（完整版本，例如 `go1.26.1`）。默认从 PATH 解析 `go`，版本必须与构建 VM 的 Go 版本完全一致。每次 Compile 在缓存查找前探测实际版本和宿主平台，不匹配直接失败；探测和构建共用 `buildTimeout` 预算及调用方取消信号。插件工厂不启动工具链进程。

```yaml
compiler:
  use: compiler/go
  config:
    goBinary: /opt/go/bin/go
    goVersion: go1.26.1
    buildTimeout: 30s
  deps:
    policy: policy
    abi: abi
    injector: injector
    profiler: profiler
```

此配置片段放在 `plugins` 下。编译禁用 CGO 和自动下载工具链，固定宿主 GOOS/GOARCH；忽略外部 GOROOT、GOFLAGS、GOEXPERIMENT、GOWORK、持久化 GOENV 和架构调优变量。构建与版本探测复用同一个绝对可执行路径；缓存身份包含实际版本、平台及流水线版本。开发模式同样使用这些构建约束，自定义源码策略不改变它们。

B3 已实现版本化产物清单及完整性校验：记录实际工具链与 runtime/Gas 规则，校验缓存、部署、加载与执行的内容摘要。完整格式与插件职责见 [产物清单](artifact_manifest.md)。工具链与产物生产者仍须受信任；来源认证后置。

## 注册第三方插件

```go
package myrunner

import (
    "github.com/lengzhao/pluginkit"
    "github.com/lengzhao/vm/core"
)

type Config struct { Endpoint string `json:"endpoint"` }
type Deps struct { /* 按需声明 core 接口，以 json tag 命名 */ }

func New(cfg Config, deps Deps) (core.Runner, error) {
    return newRunner(cfg.Endpoint), nil // newRunner 是自定义实现
}
func init() { pluginkit.Register("my/runner", New) }
```

构造函数返回 `(T, error)`，通过 Deps 字段声明依赖，禁止在编译/执行路径中另建默认依赖。无需修改 VM 源码或维护第二套注册中心。

自定义源码策略只需实现 `Review(string) error` 和稳定、非空的 `ID() string`；改变语义时升级 ID。可选实现 `PolicyGuarantor.Guarantees() PolicyGuarantees`，由 `core.GuaranteesOf` 读取执行保证。确定性要求通过 `Deterministic` 保证检查，不绑定内置策略 ID。策略是受信任扩展点，声明本身不是确定性的证明。

`policy/artifact` 只校验产物接受规则：默认接受当前源码策略 ID；`acceptedPolicies` 可显式指定其他 ID；源码策略声明确定性或配置 `requireDeterminism` 时，产物必须包含确定性保证。`allowAnyPolicy` 只允许非确定性的内置开发模式，用于旧 API 兼容。部署与执行分别调用该校验器，存储不承担部署策略。

核心契约按消费者收窄：

- `ContractCompiler` 仅有 `Compile(context.Context, string)`；审查和 Gas 注入分别由 `SourcePolicy`、`GasInjector` 提供。
- `SecurityReviewer` 仅有 `Review(string) error`，白名单查询是具体实现的辅助方法。
- `ContractStore` 组合 `Put(ctx, address, artifact)` 与 `Get(ctx, address)`；独立 `ContractCreator.Create` 提供只创建及相同身份幂等重试，默认部署器只依赖 Creator。文件存储实现这三种窄能力，存储不修改调用方产物。
- `ContractDeployer.Deploy(ctx, DeploymentRequest)` 使用显式 ChainID/Sender/Nonce/Salt 和清单摘要派生地址，持久化部署记录；完整规则与兼容入口见 [确定性部署](deployment.md)。
- `Engine` 仅组合编译、部署、读取、执行、ABI 和估算等业务能力。`Execute(ctx, ExecutionRequest)` 返回自包含 `ExecuteResult`，不要求最近结果查询或生命周期方法。
- 编译器依赖 `GasProfileBuilder`，引擎依赖 `GasEstimator`；同一个 `GasProfiler` 插件同时实现两者，以保证画像和估算模型配对。内建估算器严格拒绝画像版本不为 1 的输入。

编译缓存同时考虑原始源码、策略 ID、实际 Gas 注入代码、生成入口和编译流水线版本，更换注入/ABI 行为不会误用旧二进制。存储与执行继续校验既有协议和 ABI 版本。

## 生命周期与错误

构造函数必须保持无活动资源：连接、goroutine 等放到可选的 `Start(context.Context) error` 中。PluginKit v0.1.6 构造失败不会返回部分构建结果，因此构造函数自身必须清理失败时获取的资源。

完整图构建后，assembly 按依赖顺序调用 Start；启动失败或上下文取消时，用独立的清理上下文逆序回滚已尝试启动的实例，包括当前启动失败的实例，不调用后续未启动实例的 Stop。插件应能安全清理部分启动状态。正常通过 `rt.Stop(ctx)` 逆序停止全部实例，包括运行中懒加载资源的 Stop-only 插件；成功停止的实例不会重复释放。取消或超时导致未完成的停止可换新上下文重试，其余错误聚合返回。`core.Engine` 不定义 Stop；可选 `Stop(context.Context) error` 由 assembly 统一处理。根包兼容门面保留 `Stop() error`，委托整个 Runtime 停止。

配置/构建错误直接返回给调用方；缺失依赖、循环、类型不匹配、未知插件由 PluginKit 检查。ProcessRunner 延续现有进程隔离边界，并不提供 OS 沙箱。

## 原有 API

`vm.NewVMEngine`、`NewVMEngineWithRunner`、独立编译器等入口保留。新增 `vm.OpenVMEngine(ctx, config)` 返回 `(*VMEngine, error)`，适合显式处理初始化错误。旧构造函数没有 error 返回值，装配失败时会 panic；原先忽略存储创建失败的行为已改为尽早报告。

原有公共数据类型保持 core 类型身份；旧方法和具体实现通过根包及 `internal/compat` 适配。旧 `EstimateFromProfile` helper 会复制并把版本 0 适配为 1，新 core 估算契约严格检查版本。历史白盒测试保留在 `internal/builtin`，根包另有兼容入口测试。

文件存储的 `Put` 按完整记录替换：先在临时目录写入快照，再在实例锁保护下发布；同地址覆盖会清除旧的可执行文件和可选 Gas 画像。发布失败时恢复旧记录；恢复本身失败时保留旧记录备份并返回其位置。共享目录的多个 Store 实例或多个进程之间不提供事务协调。
