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
- [x] B3 固定工具链及产物清单、摘要校验、规则版本
- [x] B4 确定性部署输入与地址派生
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


### B3 第一批：实际编译工具链固定

- 新增 compiler/go 配置 goBinary / goVersion；默认要求实际工具链版本等于构建 VM 的 Go 版本，固定宿主平台。
- Compile 在缓存命中前验证实际工具链，构建复用同一绝对路径；缓存加入实际工具链身份，流水线升级为 v10。
- 全部模式禁用 CGO、自动下载和外部 Go 构建配置干扰；版本探测支持取消，并与构建共享超时预算。
- 本批不标记 B3 完成：产物清单、摘要校验及规则版本持久化仍待实施。
- 说明与配置示例：docs/plugins.md；实施计划：docs/plans/2026-10-25-toolchain.md。
- 验证：全量普通测试、全量 race、go vet、git diff --check 通过；新增 3 个工具链回归测试，独立审查无 P1/P2。race 首轮既有 TestProtocolRequiresResponseFields 出现 1 秒超时，未修改其超时或生产行为；单独重复 20 次及完整 race 重跑均通过。


### 第四批验收记录：B3 完成

- 在工具链固定基础上新增 ArtifactManifest v1，持久化实际工具链及二进制、源码、插桩、入口、runtime、ABI、画像与策略元数据摘要，记录 ABI/协议/runtime/Gas 规则版本。
- 缓存使用完整 SHA-256 身份；二进制、清单及根摘要以目录整体原子发布。命中时校验完整性与当前预期构建输入，损坏缓存明确拒绝；保留并加强同源码并发发布回归。
- Store 写入完整快照并在复制后二次校验，读取固定 artifact 文件名，画像损坏或缺失不能静默忽略。默认校验器、Engine 加载/执行/估算和 ProcessRunner 执行前统一检查。
- 缺少清单或版本不受支持的产物要求重新编译部署；内容不一致返回 ARTIFACT_INTEGRITY。摘要不是来源认证，签名与完整 OS 沙箱不属于 B3。
- 新增 10 个测试函数（含表驱动负例），覆盖篡改、缺失描述文件、未知规则、目标平台、取消、特殊文件及加载入口；原有手工测试产物补齐清单，未添加兼容绕过。
- 全量普通测试、全量 race、go vet、git diff --check 均通过；独立审查及增量复核无 P1/P2。
- 文档：docs/artifact_manifest.md；同步 README、调用协议、插件指南、确定性与编译设计文档。变更保留工作区，未提交。
- 当前下一项：B4 确定性部署输入与地址派生；当前地址仍包含本地时间。


### 第五批验收记录：B4 完成

- 新增 DeploymentInput / DeploymentRequest / DeploymentRecord v1。原生部署接口接收显式请求，将 ChainID、Sender、Nonce、32 字节 Salt 与清单摘要按版本化长度前缀编码派生完整 SHA-256 地址，不依赖时钟。
- 新增独立 ContractCreator，只创建或接受同身份有效重试；默认部署器不再消费可覆盖的 Put。文件存储通过完整非空目录原子发布支持跨实例并发 Create，不同或损坏的已有记录报 DEPLOYMENT_CONFLICT，保留 Put 的完整替换语义。
- 部署输入、版本和产物摘要持久化；加载与执行校验声明记录及地址。原生部署不修改输入产物，旧根包入口使用固定本地域，同一产物重复调用返回同一地址；新增 DeployWithInput / DeployContext 和地址预测函数。
- 新增 11 个测试函数，覆盖独立固定向量、每个地址输入、字段编码歧义、非法输入、跨存储一致性、输入不变、并发同身份/冲突、损坏记录、篡改及预取消无副作用；保留最小 Creator 插件与原 Put 替换回归。
- 全量普通测试、全量 race、go vet、git diff --check 均通过，独立审查及增量复核无 P1/P2。既有响应字段测试的 1 秒预算在隔离重复 20 次时也复现超时，现仅将该格式测试预算调整为相邻协议测试一致的 5 秒，重复 20 次通过；生产超时逻辑及专门取消/超时测试的短预算未改。
- 文档：docs/deployment.md；同步接口示例、插件指南、设计文档与 README。Sender 认证、账户 nonce 账本及状态事务仍由后续宿主集成负责。
- 变更保留工作区，未提交。当前下一项：C1 状态事务接口与快照/提交/回滚。
