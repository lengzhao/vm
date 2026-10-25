# B3 工具链固定实施计划

**Goal:** 解决缓存记录的宿主 Go 版本与实际构建工具链不一致的问题，完成 B3 的工具链基础部分。

**Architecture:** Compiler 在 Compile 的调用方 context 和构建超时内解析、探测实际 Go 可执行文件；默认要求版本等于构建 VM 的 Go 版本，可通过 compiler/go 配置 goBinary、goVersion 显式固定。探测与构建共用去除外部 Go 配置影响的环境；缓存覆盖实际版本、平台和构建规则。工厂不启动进程。产物清单及摘要验证仍作为 B3 后续工作。

**Tech Stack:** Go 标准库、exec.CommandContext、现有 PluginKit 工厂。

## 步骤与验收

1. internal/compiler/compiler_test.go：先添加实际版本不匹配（即使缓存已存在）、探测取消、配置指定二进制和构建环境隔离测试；运行并确认失败。
2. internal/compiler/toolchain.go、compiler.go：解析一次可执行路径；探测 `go version`；严格校验版本和宿主目标；构建复用该绝对路径及同一环境；缓存包含工具链身份。默认禁用工具链自动下载、CGO、GOENV、外部 GOFLAGS/GOEXPERIMENT/GOWORK/GOTOOLCHAIN/目标覆盖。
3. plugins/compiler/register.go：增加 goBinary、goVersion 配置并传递给实现，不扩大 core.ContractCompiler。
4. 更新假编译器测试，保留取消响应时间与独立构建发布断言。docs/plugins.md、todo.md 说明本批完成范围。
5. 顺序运行 go test -p 1 -timeout 5m ./...、go test -race -p 1 -timeout 5m ./...、go vet ./...、git diff --check。变更保留工作区，不提交。

## 验收结果

2026-10-25：工具链基础部分完成；新增 3 个测试覆盖默认版本拒绝、指定路径/版本与缓存校验、环境隔离和探测取消。普通全量、race 全量、vet、差异检查通过，独立审查无 P1/P2。race 首轮一项既有 shell 协议测试出现 1 秒超时，单独重复 20 次及完整重跑通过，未更改其行为或预算。B3 清单、摘要与规则版本持久化继续保留待办。

后续状态：B3 剩余清单及摘要工作已完成并验收，见 [产物清单实施计划](2026-10-25-artifact-manifest.md)。
