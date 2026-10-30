# Plugin Contract Refinement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 完成已批准的七项接口审查修改，维持根包旧 API。

**Architecture:** 最小 core 能力接口由消费者声明，内建领域实现独立分包，PluginKit 装配与业务门面组合能力，兼容方法只留根包适配。

**Tech Stack:** Go 1.26.1, PluginKit v0.1.6。

1. `core/interface_test.go`：先检查 Compiler/Reviewer/Engine 的实际方法集，运行并观察失败。
2. `core/*`：定义上下文业务接口、ContractReader/Writer/Store、Deployer、ArtifactValidator、PolicyGuarantees/Guarantor 和 Gas 窄视图；保留公共结果类型。
3. 独立迁移实现：安全/策略/Gas/ABI 与编译/Runner；只创建各自目录，原 builtin 源码先只读。分别增加针对性测试。
4. `internal/storage` / `deployment` / `engine` / `compat`：实现存储部署分离、调用取消、无历史状态业务引擎、旧 API 适配；更新 plugins 和 assembly 图。
5. 将跨能力旧测试绑定到根包兼容实现；私有入口生成测试迁移到 compiler 所属包，超时测试改用显式构造配置，Runner 测试改用构造注入。
6. 更新 YAML、示例、assembly 测试和接口文档；验证最小接口实现可以注册、确定性策略不限具体 ID、生命周期统一。
7. 完成需求/质量审查；串行执行 `GOCACHE=/private/tmp/vm-go-cache go test -p 1 -timeout 5m ./...`、`go test -race -p 1 -timeout 5m ./...` 和 `go vet ./...`；不自动提交混合工作区。

## 实施记录

- 方法集测试先 RED 后 GREEN，core Compiler/Reviewer 只保留核心职责，Engine 移除状态展示与生命周期方法。
- core Engine 组合 context-aware 业务视图；执行使用 ExecutionRequest；ContractStore、ContractDeployer、ArtifactValidator 独立。
- 内建实现按领域分包，plugins 不再依赖 builtin；builtin 仅保留跨能力历史测试，原有 101 个测试名称全部保留，入口生成测试迁至 compiler 包。
- 自定义 compile-only/review-only/store-only 实现及自定义确定性策略通过 PluginKit 集成测试；策略保证持久化。
- 编译主动取消、进程已取消请求不调用序列化/Host、画像版本错配、同地址完整替换均有 RED→GREEN 回归。
- 生命周期已覆盖启动失败只回滚 attempted Starters、成功 Stop-only 懒资源清理、Stop 超时新 context 重试与成功项幂等。
- 普通全量测试与 vet 已通过；最终 race 结果另记下方。
- 依赖检查：plugins/abi 仅引入 internal/abigen；runner 仅引入 callcontext/process；storage 仅引入 internal/storage。

## 最终验证

- `GOCACHE=/private/tmp/vm-go-cache go test -p 1 -timeout 5m ./...`：通过。
- `GOCACHE=/private/tmp/vm-go-cache go test -race -p 1 -timeout 5m ./...`：通过。
- `GOCACHE=/private/tmp/vm-go-cache go vet ./...`、`git diff --check`：通过。
- YAML 插件示例以临时目录运行，实际输出 `5`。
- race 首轮暴露取消测试的启动等待窗口过短：补充提前退出诊断并分离启动预算，保留取消后 500ms 终止断言；该测试单独重复五次及最终全量普通/race 均通过。
- 最终审查发现的 Stop 超时重试问题已有回归覆盖及只读复查；未发现剩余阻塞问题。
- 未自动提交、推送或覆盖原有工作区中的其他变更。
