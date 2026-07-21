# 智能合约虚拟机开发计划

## 项目概述
本项目是一个基于 Golang 的智能合约虚拟机，允许开发者直接使用原生 Golang 代码编写智能合约。通过模块化设计、关键字限制和导入控制确保执行安全性。

## 当前状态分析（MVP）
- Compile → Deploy → Execute 主链路已打通（`go build` + ProcessRunner）
- 安全审查（Import / 危险 AST / 包级 var）、ABI、Gas 控制点注入已可用
- Runner / Engine E2E / ContractManager 真实产物测试已补充
- 示例位于 `examples/{basic,gas,complete,advanced,event}`
- **尚未完成**：默认库 Host Runtime、EstimateGas、完整沙箱、合约升级与并行调度

## 开发计划

### 第一至三阶段：核心流水线（已完成）
- [x] 安全审查、ABI、Compiler、ProcessRunner、Gas 计量、合约文件存储
- [x] 统一 VMEngine 编排，去掉占位编译/执行路径

### 第四阶段：集成测试与文档收敛（进行中）
- [x] Runner 单元测试（JSON / 超时 / Gas 超限 / 未知函数）
- [x] Execute 失败路径 E2E（Gas、超时、参数错误、多返回值）
- [x] ContractManager 真实二进制 Deploy→Execute
- [ ] 详细设计文档全面收敛为 MVP / 后置标注
- [ ] 开发者指南与 API 参考

### 第五阶段：Host Runtime / 默认库 MVP
- [ ] 定义 Host / CallContext / Event
- [ ] 只读链上下文：BlockHeight / BlockTime / Sender / ContractAddress
- [ ] 事件日志 Log
- [ ] Object 存储与跨合约 Call 后置

### 第六阶段：Gas 模型推进
- [ ] 明确入口 / 函数 / 循环计费点基线
- [ ] 默认库接口 Gas 表（随 Host 生效）
- [ ] EstimateGas 保守估算
- [ ] 基准测试

### 第七阶段：沙箱增强
- [ ] 可插拔 Runner（Process / Docker / 未来 WASM）
- [ ] 负例安全测试（文件系统、网络）
- [ ] Linux seccomp / namespace 或 Docker 隔离评估

### 第八阶段：平台能力（后置）
- [ ] 合约升级
- [ ] Object 并行执行
- [ ] CLI / 部署工具
- [ ] TinyGo 可选构建器

## 质量保证
1. `go test ./...` 必过
2. 关键功能需同步更新 `docs/`
3. 文档显式标注 **MVP / 后置 / 未实现**
4. 默认不信任不可信合约，直到沙箱阶段完成
