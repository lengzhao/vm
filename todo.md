# 智能合约虚拟机开发计划

## 项目概述
本项目是一个基于 Golang 的智能合约虚拟机，允许开发者直接使用原生 Golang 代码编写智能合约。通过模块化设计、关键字限制和导入控制确保执行安全性。

## 当前状态分析（MVP）
- Compile → Deploy → Execute 主链路已打通（`go build` + ProcessRunner）
- 安全审查、ABI、Gas 控制点注入、集成测试已可用
- Host Runtime MVP 已接入：`contractapi` + `ExecuteWithContext` + 事件回传
- 加固：禁止宿主根包 import、编译缓存 + embed contractapi、最小 env、CallContext 零值语义、结果自包含
- 默认 Object 已接入内存状态事务；失败、OOG、DryRun 无提交，事件仅随成功提交返回
- 编译与执行后端可选插件：Go / WASI 编译器，Process / Docker / wazero Runner；D2 原型比较已完成，默认配置不变
- 可选 D3：state/mvcc 按键冲突检测、batch/ordered 有界推测执行与按输入顺序提交；确定性前提见 docs/parallel_execution.md
- **尚未完成**：持久化状态后端、多 Object/所有权与生产链调度集成

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
- [x] 默认 Object 存储与执行事务
- [x] 跨合约 Call 与整链临时 store / Gas
- [ ] 多 Object / 所有权后置

### 第六阶段：Gas 模型推进（已完成）
- [x] 明确入口 / 函数 / 循环计费点基线
- [x] 默认库接口 Gas 表（只读上下文 + Log；Object/Call 后置）
- [x] EstimateGas 启发式估算（静态 Profile + 可选 DryRun）
- [x] 基准测试（Add / Loop / EstimateGasStatic）

### 第七阶段：沙箱增强
- [x] 可插拔 Runner（Process / Docker / wazero；Wasmtime 可后续独立扩展）
- [x] 负例安全测试（文件系统、网络及资源耗尽）
- [x] Docker 编译/执行隔离与资源限制（独立 Linux 过滤器后置）

### 第八阶段：平台能力（后置）
- [ ] 合约升级
- [x] 默认 Object 事务的可选有序并行执行（多 Object/所有权仍后置）
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
- [x] C1 状态事务接口与快照/提交/回滚
- [x] C2 Object 存储，失败及 DryRun 无提交
- [x] C3 跨合约共享事务/Gas、重入与深度限制
- [x] D1 编译沙箱与执行沙箱，资源限制与负例验证
- [x] D2 对比受限原生后端与 WASM 原型，按场景推荐插件配置
- [x] D3 基于读写集合的冲突检测及确定性并行执行（独占状态写入等前提）


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


### 第六批验收记录：C1 完成

- 新增 core.StateStore / StateTransaction、StateKey / StateWrite 及可 errors.Is 判断的关闭/冲突错误；快照内读写、删除、存在标志和排序读写集合形成独立状态契约。
- 新增 state/memory 插件，以全量快照和本地写覆盖实现隔离；原子提交使用全局版本身份令牌拒绝过期写事务，纯读事务不推进版本。输入输出及集合均复制，Store 和事务支持并发方法调用。
- Begin context 贯穿事务，操作 context 可进一步限制；失败提交不发布任何键，Rollback 清理不依赖有效 context。结束后拒绝读写/提交，Rollback 幂等，保留访问集合供检查。
- 新增 13 个测试，覆盖快照/自身写入、空值及缺失、删除/回滚、重复结束、字节与集合副本、合约命名空间、保守冲突、取消/deadline、并发提交/回滚、完整快照可见性及 PluginKit 图隔离与最小契约消费。
- 最终代码固定后，全量普通测试、全量 race、go vet、git diff --check 通过；独立审查及增量文档复核无 P1/P2。首轮普通测试期间补加测试 import 造成依赖扫描与编译不一致，固定文件后完整重跑通过，未修改生产代码绕过。
- 文档：docs/state_transactions.md，插件指南、README 和文档目录同步。内存实现不持久化；细粒度读写冲突检测后置 D3。
- 当前变更保留 dev 工作区，未提交。下一项 C2：Object 与 Engine/Runner 接入，保证失败、OOG、DryRun 无状态提交及事件一致性；C1 尚未接入执行路径。


### 第七批验收记录：C2 完成

- 新增 contractapi.DefaultObject().Get/Set/Delete，按实际 ExecutionRequest.Address 绑定默认对象字段；返回字节与存在标志，隔离合约命名空间，不扩大 Host 接口。ObjectStore 只暴露字段读写，Runner 无地址选择与提交能力。
- Engine 注入 StateStore，每次执行获取独立事务；仅成功结果且 Gas 验证通过后提交。业务失败、panic、OOG、取消/超时、空响应、宿主错误及提交冲突均回滚，失败结果清除 Data/Events。原生与根包 DryRun 都执行后回滚，不发布本次事件；回滚错误与原错误合并。
- ProcessRunner 接入协议 v2 的逐行双向宿主调用，保留 stdout/stderr 各 1 MiB 限制，另限制累计回复 1 MiB、1024 次宿主调用、UTF-8 字段名 1024 字节与单值 64 KiB。Get/Set/Delete 固定扣 5/10/5 Gas，宿主检查消息与最终 Gas 下界及上限。
- 审查发现并由失败测试复现：后代保留 stdin 读端时，取消不能解除同步大消息写入。现通过 context.AfterFunc 主动关闭输入写端，补齐初始大请求与大字段回复的取消回归，保留独立子进程启动同步和取消预算。
- 协议升级为 2，runtime/Gas 为 contractapi-v2-object / go-gas-v2-object，流水线为 pipeline-v12-object-runtime，源码策略为 go-development-v2-object / go-deterministic-v2-object。新增 Object 源码加入内嵌、构建缓存与清单摘要；旧产物需重新编译部署，ABI/Profile schema 保持 1。
- 默认配置和 YAML 接入 state/memory；新增仅 Begin 状态插件替换验证。16 个新增测试函数覆盖实际 Object 跨调用读写/删除/空值、失败与 OOG/DryRun 无提交、取消、确定性回放、命名空间、运行时符号权限、事务后端/冲突/Gas/回滚错误、RPC 格式/Gas/资源限制及管道取消。
- 全量普通测试、全量 race、go vet、git diff --check 通过；两个独立审查及增量复核无剩余 P1/P2。
- 文档：docs/object_runtime.md；同步状态事务、插件、调用协议、Host、Gas、确定性策略、产物清单与 README。内存状态不跨 VM 实例/进程重启持久化，多对象/所有权仍后置。
- C1/C2 变更保留 dev 工作区，未提交。当前下一项 C3：跨合约共享事务/Gas、重入与深度限制。


### 第八批验收记录：C3 完成

- 按用户确认的简化方案，将最外层 Begin 返回的事务当作整链临时 store；所有嵌套帧读写同一份覆盖，只有最外层成功后一次性原子 Commit，失败/DryRun 直接丢弃。不增加保存点、嵌套提交或逐键补偿写入。
- 新增 contractapi.Call(address,function,args...) 与 core.ContractCaller 作用域能力；目标加载与产物校验不绕过。子失败粘性保留，调用方忽略 error 也不能提交；本地参数编码/通道错误同样阻止入口成功响应。
- 父报告调用点已用 Gas，子仅获得剩余额度，子树实际消耗计入父 runtime，根计量一次。固定 Call 成本 10，验证累计下界、剩余量、最终少报和 uint64 溢出；零或不足入口的剩余预算不变成无限额。
- 默认深度 8（根为第 1 层），配置范围 1–32；禁止栈内自调用及 A→B→A，已返回地址可顺序重调。子 Sender 是真实直接调用合约，目标 ContractAddress 及存储命名空间不可被父展示地址覆盖；沿用根区块快照与取消/截止时间。
- Call 在调用点移交父事件前缀，按前缀/子树/后续事件拼接。整链跨调用最多 1024 次，事件编码累计 1 MiB，子结果 JSON 最多 64 KiB；保留每进程消息和回复预算、取消关闭 stdin。
- 协议 3、contractapi-v3-call / go-gas-v3-call、pipeline-v13-cross-contract、go-development-v3-call / go-deterministic-v3-call；ABI/Profile schema 保持 1，内嵌/缓存/清单纳入 Call 源码。旧产物需重编译部署。新增 CALL_FAILURE / CALL_DEPTH_EXCEEDED / REENTRANCY 错误分类。
- 新增 17 个测试函数，覆盖临时写入可见、父子统一发布/丢弃、忽略失败、DryRun、事件顺序、大整数转发、上下文与 Sender、8/9 层边界、直接/间接重入、顺序重调、严格确定性回放、Gas 精确一次/预算/少报/溢出、资源上限、子调用取消、畸形消息与本地编码错误。
- 全量普通测试、全量 race、go vet、git diff --check 通过；两位独立审查及增量复核无 P1/P2。文档：docs/cross_contract.md，并同步协议、Host、Object、状态事务、Gas、策略、产物清单、插件指南及 README。
- C1–C3 变更保留 dev 工作区，未提交。当前下一项 D1：编译沙箱与执行沙箱、资源限制及负例验证。


### 第九批验收记录：D1 完成（2026-10-25）

- 按用户确认的 Docker 方案，新增 compiler/go 可选 sandbox 配置和 runner/docker。保留默认本机开发路径；配置失败、镜像缺失或容器运行失败不降级宿主。构造不访问 daemon，不拉取镜像；只接受本地不可变 sha256 镜像 ID。
- 两种沙箱均非 root、禁网、只读根目录、移除 capabilities、no-new-privileges，使用私有命名空间与默认 seccomp；CPU、内存（禁额外 swap）、PID、文件描述符、tmpfs 和运行时间均有限制，关闭 Docker 日志落盘。编译仅开放单次只读源目录及可写产物目录，工具链子进程在容器内；执行只挂载私有、重新校验过的二进制快照。
- Docker 编译目标为 Linux/宿主架构，清单与缓存纳入镜像 ID 及实际工具链摘要；policy/artifact 显式配置 targetOS/targetArch，ProcessRunner 与 Docker Runner 分别独立检查目标。核心能力接口未扩大，runtime/协议/Gas 语义未改变。
- 容器先 create 再 attach/start，运行后所有路径强制删除整个容器。清理失败进入 Runner 错误并让 Engine 回滚状态和事件；create 期间取消仍保留 CANCELED 分类。create CLI 超时明确报告结果不确定及具体随机容器名，绝不启动；daemon 故障/宿主崩溃后的恢复仍需集成方治理，不能承诺后台自动回收。
- 新增 20 个测试函数，覆盖配置/镜像拒绝、隔离参数、预取消、创建失败/超时、取消/清理、真实编译/缓存/编译失败、只读构建挂载、宿主文件和网络访问拒绝、OOM、实际 PID 耗尽、tmpfs 配额、后代清理，以及 Object/跨合约/失败/DryRun/OOG/清理失败事务回滚。
- 使用本机 Docker 28.5.2、Linux arm64 Go 1.26.9 镜像实际验证；VM 测试工具链为 Go 1.26.1。显式启用 Docker 的全量普通测试、全量 race、go vet、git diff --check 均通过，测试后无 vm-d1 容器残留。独立审查及最终增量复核无剩余明确 P1/P2。
- 文档与配置：docs/sandbox_roadmap.md、docs/plugins.md、presets/docker.yaml；同步 README、产物清单和架构/执行环境文档。Docker、镜像和宿主内核仍在信任边界内，D2 生产后端比较尚未完成。
- 变更保留当前 dev 工作区，未提交。下一项 D2：对比受限原生后端与 WASM 原型后确定生产后端。


### 第十批验收记录：D2 完成（2026-10-26）

- 按用户确认的插件化选择，新增 compiler/wasip1 与 runner/wazero，支持 compiler / interpreter 引擎。编译器可组合 Docker 编译沙箱；产物目标显式为 wasip1/wasm，工具链、缓存及清单覆盖目标，不改变核心能力接口或默认插件。
- 将有界协议会话、Object 与跨合约消息处理提取到 internal/execution，由进程与 WASI 共用；WASI 同步回复使用有界协议约束下的输入队列，避免 fd_write 回调等待下一次 fd_read 的管道死锁。协议、Gas、状态事务语义保持一致。
- 固定 wazero v1.12.0；每帧独立 runtime，默认无宿主文件/网络/环境/真实时钟/随机源授权，限制线性内存、模块字节及 context 执行时间。支持中止无源码插桩的 Wasm 死循环；不承诺 JIT/验证硬实时抢占或 OS 总 RSS 配额。
- 新增 16 个测试函数，保留并迁移低层协议测试，覆盖实际编译、目标缓存分离、整数宽度及边界、原生结果/事件/Gas 一致、并发、Object、跨合约事务、失败/OOG/DryRun、取消分类、权限和内存/输出负例。
- 基准实际运行三轮、每轮三次完整 Engine 调用，原始数据及方法见 docs/backend_comparison.md、docs/benchmarks/2026-10-26-backends.txt。当前每帧重新 JIT 原型的 Add / 跨合约耗时约为 Docker 的 2.14 / 1.82 倍；推荐需要 OS 配额的不可信原生执行选 Docker，WASI 保留纯 Go 嵌入原型选项。未测总 RSS，不将宿主 Go 分配与跨进程总内存混比。
- race 下曾在 Child 触发父 20 秒预算取消；拆分最小合约后仍在全量与单独运行复现。记录显示单帧 Add / Width 约 14.40 / 12.89 秒、Child 19.67 秒。仅语义集成测试预算改为 Runner 60 秒 / Engine 90 秒，生产默认及专用 100ms 死循环取消测试未改，不宣称默认预算适用于所有嵌套负载。资源负例加强为真正超过 1 MiB 的 OUTPUT_LIMIT 与含 memory 诊断的内存错误，避免协议错误或超时充数。
- 最终固定代码后，显式启用 Docker 的全量普通测试、全量 race（assembly 222.039 秒）、go vet、git diff --check 通过，测试后无 vm-d1 容器残留。独立审查及测试预算增量复核无剩余明确 P1/P2。
- 配置与文档：presets/wasi.yaml、docs/plugins.md、docs/backend_comparison.md、docs/plans/2026-10-26-d2-wasi.md；同步 README 与架构/执行环境文档。变更保留 dev 工作区，未提交。下一项 D3：基于读写集合的冲突检测及确定性并行执行。


### 第十一批验收记录：D3 完成（2026-10-26）

- 按用户确认的插件方案，新增 state/mvcc 与 batch/ordered；原 state/memory、core.Engine 及默认配置保持原样。MVCC 同时提供独立 SnapshotStore/可验证事务能力，Engine 额外提供 TransactionExecutor，在调用方事务中执行完整调用树、不提交或回滚。engine/batched 仅组合两个门面能力，插件依赖由 PluginKit 明确连接。
- MVCC 按实际读/写键的版本身份令牌校验，独立写可以共同提交；缺失键、删除及 ABA、同键盲写、写偏斜、纯读均验证。提交原子，失败不发布；对外字节及排序集合为副本，取消/关闭/幂等回滚语义保留。内存实现复制代际，tombstone 不回收，不持久化。
- 调度器按有界窗口并行推测、按输入顺序提交，冲突项在最新状态顺序重执行一次；业务失败及纯读结果也验证，推测 Gas/事件不进入最终结果。再次冲突停止，不无限重试。单实例用可取消门限串行批次，构造不创建活动资源，窗口返回前等待所有 worker。
- 明确区分业务失败与基础设施错误，检查组合错误全部分支；取消、状态/产物、进程/容器清理及未知错误停止批次并清理未提交窗口，返回已处理前缀。成功前缀保持提交，业务失败项回滚，不承诺整批原子性。
- 新增 20 个测试函数。真实严格合约直接串行 Execute 与 workers=1/2/4 比较 Data、Events、Gas、错误码和最终状态，覆盖过期业务失败转成功、只读、删除、失败/OOG、跨合约成功/父失败及非法请求后续处理；另覆盖独立/冲突/缺失 ABA、并发副本/context、事务归属、有界并发、取消、初始化及回滚失败、基础设施停止前缀、二次冲突停止、最小自定义插件及不支持验证的后端拒绝。
- 独立审查发现并修复三项边界，均补失败回归后修复：基础设施错误误作业务失败继续；事务执行加载错误包装与单次路径不同；空地址/空函数未分类。现两路径共享加载校验，空请求返回 INVALID_REQUEST，增量复核无剩余明确 P1/P2。
- 最终固定代码后，显式启用 Docker 的全量普通测试、全量 race（assembly 236.753 秒）、go vet、git diff --check 均通过，测试后无 vm-d1 容器残留；没有调整 D3 生产或测试超时预算。
- 文档与配置：docs/parallel_execution.md、presets/parallel.yaml、docs/plugins.md、docs/plans/2026-10-26-d3-parallel.md；同步状态、架构和 README。确定性以无外部状态写入/产物替换、不可变输入、确定性合约及可信宿主能力为前提，时间/资源故障不属于共识保证，未测并行吞吐或声称性能提升。
- D1–D3 变更保留当前 dev 工作区，未提交。本轮架构加固 A1–D3 已完成；持久化状态、多 Object/所有权、生产调度与开发者指南仍待后续规划。
