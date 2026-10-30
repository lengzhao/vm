# 插件接口职责收敛设计

用户已通过“修改”批准前轮接口审查的调整方向。

- core 契约与兼容 API 分离：Compiler 仅提供带 context 的 Compile；Reviewer 仅提供 Review；Engine 作为业务门面组合窄能力，不含历史状态、配置展示或 Stop。
- 执行使用 Execute(ctx, ExecutionRequest) 返回完整结果。编译、部署、读取、估算均贯穿调用方 context。
- ContractStore 只提供 Put/Get（读/写窄接口可组合）；独立 Deployer 负责地址生成及部署规则；ABI/Profile 从 artifact 投影。
- SourcePolicy 保留 Review+ID；可选 PolicyGuarantor 显式声明确定性保证。ArtifactValidator 单独负责运行产物接受规则，不在编译器/引擎中判断具体策略名称。确定性产物的保证写入元数据并严格校验；缺失保证不能用于要求确定性的运行时。
- GasProfiler 保留自洽模型，并提供 ProfileBuilder/Estimator 消费者视图，同一个插件实现二者。GasMetering 不机械拆分。
- internal 按 security、policy、gas、compiler、abigen、process、storage、deployment、engine、callcontext、compat 分包。插件工厂只引用对应实现。原 builtin 目录仅保留跨能力回归测试及测试适配。
- 根 vm 包保留旧签名与兼容历史 getter，适配新的 core 接口；生命周期通过 assembly 的带 context 协议管理。
- 先验证接口收缩测试失败，再增加自定义窄实现、取消传播、策略替换、存储/部署分离与完整流水线测试。最终串行运行普通测试、race 和 vet，避免重叠超时抖动。
