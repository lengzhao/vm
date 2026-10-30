# PluginKit VM Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 VM 的全部可替换能力交由 PluginKit 装配，同时保留现有 API。

**Architecture:** core 定义稳定类型和能力；plugins 提供 Config/Deps 工厂；assembly 负责默认图、配置入口及生命周期。根包只提供兼容导出。

**Tech Stack:** Go 1.26.1, github.com/lengzhao/pluginkit v0.1.6, yaml.v3。

### Task 1: 装配行为测试（RED）
Create assembly/assembly_test.go：覆盖默认编译部署执行、第三方 Runner/ABI 注入、缺失依赖/循环/类型错误、严格 YAML、取消上下文及生命周期。
Run `GOCACHE=/private/tmp/vm-go-cache go test ./assembly`，确认新 API 缺失。

### Task 2: 抽取 core 与能力接口
在 core 中定义公共类型与能力接口；迁移现有实现到内部实现包。
将原有白盒测试随实现迁移，保留测试内容及已修改的源码。根包生成类型别名与兼容函数。

### Task 3: 全能力依赖注入
Modify 编译器和引擎构造：显式注入策略、ABI、Gas 注入/画像/工厂、存储及 Runner；调用过程不创建默认子模块。
Create plugins/*/register.go 和 plugins/all/all.go，以 PluginKit 注册工厂。
Create assembly/config.go/defaults.go/runtime.go。仅调用 PluginKit 构建，不另建注册中心。
Run `GOCACHE=/private/tmp/vm-go-cache go test ./assembly ./...`，修复回归。

### Task 4: 兼容 API、生命周期与文档
根包 NewVMEngine 等入口委托默认插件图；新增返回 error 的装配 API。
生命周期回收、自定义能力替换、Gas 独立性补充边界测试。
Create presets/default.yaml、examples/plugins/main.go、docs/plugins.md；更新 README。

### Task 5: 验证与审查
Run `GOCACHE=/private/tmp/vm-go-cache go test ./...`、`go test -race ./...`、`go vet ./...`（均设置可写 GOCACHE）。
检查依赖方向、没有硬编码嵌套创建、旧 API 编译兼容、原有加固行为完整。
不自动提交现有混合工作区的业务变更。

## 完成与验证

- 已实现全部能力插件、core 契约、默认配置图、严格 YAML、生命周期与兼容 API。
- 原有白盒测试随实现原样迁移，未删除或修改测试用例；新增装配/扩展/兼容测试。
- 代码审查发现并修复：自定义 Engine 的 Stop() 兼容、构造期过早使用 MeterFactory；均已先复现失败后修复。
- `GOCACHE=/private/tmp/vm-go-cache go test -p 1 -timeout 5m ./...` 通过。
- `GOCACHE=/private/tmp/vm-go-cache go test -race -p 1 -timeout 5m ./...` 通过。
- `GOCACHE=/private/tmp/vm-go-cache go vet ./...` 与 `git diff --check` 通过。
- 插件示例使用临时目录的 YAML 实际运行输出 `5`；原有公开符号检查无移除。
- 最初重叠运行全量普通/race 测试导致短超时用例失败，停止重叠后串行全量普通/race 均通过。
- 未提交或覆盖用户先前的未提交工作。
