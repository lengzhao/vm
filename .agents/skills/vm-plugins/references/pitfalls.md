# 插件注意事项

以下是本项目维护边界；实现细节以仓库当前 core、插件 Config/Deps 及测试为准。

## 取消、资源与生命周期

- Compiler、部署、读取、执行和估算使用调用方 context。编译/进程超时从该 context 派生，不在中途切成 Background。
- ProcessRunner 在入口和 Host 回调前观察取消；已取消的请求不应继续调用参数 MarshalJSON 或 Host 方法。
- 工厂构造函数不获取连接、goroutine 等活动资源。PluginKit 构造失败不提供部分构建结果，不能指望 assembly 替构造函数清理泄漏。
- Start 按依赖顺序执行。失败回滚只逆序清理已经尝试 Start 的实例，包括当前部分启动失败实例；不要 Stop 尚未 Start 的后续实例。
- 正常运行结束也清理 Stop-only 的懒加载资源插件。统一协议是 Start(ctx)/Stop(ctx)，旧 Stop() 仅在根包门面。
- Stop 超时或取消时，保留当前未完成实例及其依赖，允许新 context 继续逆序清理。成功项不得重停；普通永久错误聚合返回，不把瞬时 context 错误永久缓存成关闭完成。
- 启动取消后的回滚使用独立清理 context，不能复用已失效的启动 context。

依据：`assembly/runtime.go`、`assembly/rollback_test.go`、`internal/process/cancellation_test.go`。

## 策略、ABI 与模型

- 策略 ID 表示版本化语义，不是安全证明。自定义确定性策略 ID 不需要等于内建 ID；PolicyGuarantor.Guarantees 声明受信任实现的保证。
- Compiler 把保证写入产物；存储保留它；ArtifactValidator 分别检查可接受策略和所需保证。要求确定性时不能接受未声明保证的旧产物，应重新编译。
- 新 assembly 配置默认严格匹配。根包旧开发构造器的宽松接受规则只存在于兼容路径，默认/空 Runner/自定义 Runner 三条路径应一致。
- 内建 GasProfile 当前版本为 1，估算器拒绝其他版本。版本 0 的旧 API 兼容只在根包 EstimateFromProfile helper，以副本归一化；不要放进 core 估算器。
- 同一个 Gas 模型插件提供画像生产及解释的配对视图；替换算法时明确 profile 版本，不按未知 schema 静默估算。
- 保留 ABI/调用协议版本检查、标量边界、整数精度及结构化错误分类。关闭可选安全检查也不允许绕过 runtime 边界。

依据：`core/capabilities.go`、`internal/policy`、`internal/gas`、`vm.go`、`abi/schema.go`。

## 每次调用与编译缓存

- 每次执行获取独立 GasMeter，不把工厂返回的计量器或上一调用状态复用到另一调用。
- Host 只解析一次，向 Runner 传递独立 CallContext 值快照；不要修改调用方字段或回放事件到可变 Host。
- core 结果携带本次 Data/Events/Gas；历史 getter 只在兼容门面，不能扩大新 Engine 接口。
- 缓存身份覆盖源码、策略、实际插桩代码、生成入口、内嵌 contractapi、工具链及流水线版本；替换 ABI/插桩不能误命中旧二进制。
- 保留独立构建目录、绝对产物路径、完整二进制发布及取消后清理。

依据：`internal/engine`、`internal/callcontext`、`internal/compiler`、`internal/builtin/hardening_test.go`。

## 存储、部署与兼容

- Put/Get 是完整产物持久化；存储不生成地址，不附带审查/编译依赖。Deployer 消费写接口和产物校验能力。
- 新业务路径不修改编译产物的 Address/ExecutablePath；旧 API 需要的变化在 internal/compat 或根包处理。
- 相同地址 Put 表示完整记录替换。新可执行文件必须生效；新 profile 为空不能残留旧 profile。文件实现先写临时快照再发布，失败恢复旧记录；恢复失败保留备份并报告路径。
- 当前文件存储的锁作用于实例，不承诺多实例/多进程共享目录事务。
- 原子记录更新不等于 OS 沙箱。ProcessRunner 的进程超时、输出限制、环境过滤等现有隔离边界需保留，不能宣称完全沙箱化。
- 提交前检查迁移后的测试仍覆盖生产实现，不用测试专用的复制实现替代真实核心。

依据：`internal/storage`、`internal/deployment`、`internal/compat`、`docs/plugins.md`。
