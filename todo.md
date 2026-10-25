# 智能合约虚拟机开发计划

## 项目概述
本项目是一个基于 Golang 的智能合约虚拟机，允许开发者直接使用原生 Golang 代码编写智能合约。通过模块化设计、关键字限制和导入控制确保执行安全性。

## 当前状态分析（MVP）
- Compile → Deploy → Execute 主链路已打通（`go build` + ProcessRunner）
- 安全审查、ABI、Gas 控制点注入、集成测试已可用
- Host Runtime MVP 已接入：`contractapi` + `ExecuteWithContext` + 事件回传
- 加固：禁止宿主根包 import、编译缓存 + embed contractapi、最小 env、CallContext 零值语义、结果自包含
- **尚未完成**：Object 存储、跨合约 Call、完整沙箱

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
- [x] 定义 Host / CallContext / Event
- [x] 只读链上下文：BlockHeight / BlockTime / Sender / ContractAddress
- [x] 事件日志 Log（contractapi + JSON events）
- [x] ExecuteWithContext / GetLastEvents
- [ ] Object 存储与跨合约 Call 后置

### 第六阶段：Gas 模型推进（已完成）
- [x] 明确入口 / 函数 / 循环计费点基线
- [x] 默认库接口 Gas 表（只读上下文 + Log；Object/Call 后置）
- [x] EstimateGas 启发式估算（静态 Profile + 可选 DryRun）
- [x] 基准测试（Add / Loop / EstimateGasStatic）

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

## 架构加固（2026-10-24，按顺序推进）

实施与验收见 `docs/plans/2026-10-24-architecture-hardening.md`。

- [x] A1 收紧 contractapi 符号权限及运行时保留标识，封堵 Gas 重置绕过
- [x] A2 JSON 返回值与事件的大整数无损传输
- [x] A3 每次调用独立 Gas / 上下文快照，计量溢出保护
- [x] A4 stdout/stderr 大小限制与超限终止
- [x] A5 并发编译隔离、缓存原子发布、构建超时
- [x] A6 Runner 依赖注入、协议与文档同步、完整测试
- [x] B1 可选确定性源码策略 v1 与受限 Go 子集
- [x] B2 统一 ABI 类型、错误码、协议版本
- [ ] B3 固定工具链及产物清单、摘要校验、规则版本
- [ ] B4 确定性部署输入与地址派生（当前本地部署仍依赖时钟）
- [ ] C1 状态事务接口与快照/提交/回滚
- [ ] C2 Object 存储，失败及 DryRun 无提交
- [ ] C3 跨合约共享事务/Gas、重入与深度限制
- [ ] D1 编译沙箱与执行沙箱，资源限制与负例验证
- [ ] D2 对比受限原生后端与 WASM 原型后确定生产后端
- [ ] D3 基于读写集合的冲突检测及确定性并行执行


### 第一批验收记录

- 2026-10-24：A1–A6 完成，新增 19 个回归测试。
- 全量测试、`go test -race ./...`、`go vet ./...`、`git diff --check` 通过。
- 本机 PATH / GOROOT 存在不同 Go 安装混用，验证时显式使用 `/usr/local/go/bin/go`、匹配的 GOROOT 和独立临时缓存；未修改全局环境。
- 独立审查发现的上下文指针共享、继承输出管道等待、导入别名遮蔽误判已修复并回归。
- 后续从 B1 开始：确定性规范 → 类型/版本协议 → 产物清单 → 状态事务 → 沙箱与并行。
- 当前 ProcessRunner 仍只适用于可信/已审查代码；完整进程树与内存隔离尚未完成。


### 第二批验收记录：B1

- 新增 RequireDeterminism 开关与 go-deterministic-v1 源码策略；默认开发模式保持兼容。
- 严格模式强制审查，限制机器时间、浮点/指针观测、直接输出、编译器指令和标准库符号。
- SourcePolicy 随产物持久化；缓存按策略分离；严格 Deploy/Execute 拒绝旧/开发模式产物。
- 严格构建固定本机 64 位目标、禁用 CGO、清理外部 GOFLAGS/GOEXPERIMENT/go.work 影响。
- 新增 8 个测试（含表驱动负例及回放、缓存、重载和环境干扰验证）；全量测试、race、vet、diff 检查通过，独立复审无 P1/P2。
- 文档：docs/deterministic_execution.md。策略标签不代替产物认证；不承诺完整共识规范或沙箱。
- 当前下一步：B2 ABI 类型、错误码与协议版本；随后 B3 产物与工具链规范及 B4 部署地址。


### 第三批验收记录：B2

- 用户明确不需要兼容性：仅支持 ABI schema v1 / 调用协议 v1，拒绝旧格式和缺失版本的产物，要求重新编译部署。
- GenerateABI / Compile 共享标量签名校验；末位 error 作为状态，参数严格校验数量、JSON 类型、null 与溢出。
- 入口改用内部编号变量和导入别名，禁止包级预声明名称遮蔽，避免参数名/包名冲突。
- ExecutionError.Code 区分业务错误、普通 panic、专用 OOG、版本/响应、超时、取消和输出限制；保留底层 cause。
- ABI 类型语法由 Go 格式器生成；静态事件仅识别真实 contractapi.Log，并推断简单字段类型。
- 新增 16 个回归测试，覆盖 16 种标量及协议、版本、名称遮蔽等边界；全量测试、race、vet、diff 检查通过。
- 独立复审发现的预声明名称遮蔽已修复，最终复审无 P1/P2。完整调用与事件示例实跑通过。
- 文档：docs/call_protocol.md。当前下一项 B3：固定工具链及产物清单、摘要校验、规则版本。
