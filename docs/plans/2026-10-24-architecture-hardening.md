# 架构加固实施计划

**Goal:** 保留原生 Go MVP，依次修复可信边界、协议正确性和并发隔离，再演进确定性、状态事务及沙箱。

**Architecture:** Engine 组织每次独立调用，Runner 负责受限进程与精确 JSON 协议，Compiler 发布不可变缓存产物。源码审查约束合约可访问的符号；当前 ProcessRunner 仍不承诺不可信代码隔离。

**Tech Stack:** Go 标准库、AST、子进程、JSON。

## 已接受的设计与取舍

用户已接受架构评估及推进顺序。采用渐进加固；暂不重写为 WASM，也不拆微服务。
每项先加失败回归，再实现，再运行对应测试。所有变更留在 dev 工作区供审阅。

## 第一批：可验证的正确性与边界修复

1. security.go/security_test.go：合约仅可访问 contractapi 的业务接口；拒绝管理符号、点导入、生成器保留标识、init 与 recover 绕过。验收：普通与别名引用、函数值引用均拒绝，正常业务 API 可编译。
2. runner.go/runner_test.go：以 json.Number 解码返回值及事件，拒绝多重响应。验收：uint64/int64 边界值、嵌套事件数值无损。
3. engine.go/engine_test.go、gas.go、contractapi/gas.go：调用独立计量，输入上下文快照，防 uint64 溢出。验收：并发调用结果互不影响，用户上下文不被写入，溢出失败。
4. runner.go/runner_test.go：限制输出缓冲，超限终止调用。验收：stdout/stderr 超限返回明确错误，普通响应不变。
5. compiler.go/compiler_test.go：独立构建目录、缓存原子发布、绝对产物路径和构建超时。验收：同源码并发编译成功，失败不发布半成品。
6. engine.go：可注入 Runner，并更新 README、Host/Gas 文档与 TODO。验收：现有构造器兼容，新构造入口可替换 Runner。

验证：PATH=/usr/local/go/bin:/usr/bin:/bin GOROOT=/usr/local/go GOCACHE=/private/tmp/vm-go125-cache /usr/local/go/bin/go test ./...；同环境 go test -race ./...；go vet ./...。

## 后续阶段与验收

- 执行规范：受限 Go 子集，禁止机器时间，精确 ABI 类型与错误分类；相同输入重复执行结果一致。
- 产物规范：工具链、目标平台、ABI/runtime/Gas 版本、源码及二进制摘要；部署和加载检测篡改，版本不兼容明确失败。
- 状态事务：先定义快照、读写集合、提交/回滚，再实现 Object；失败/OOG/DryRun 无状态提交，事件随事务提交。
- 跨合约：共享事务与 Gas、深度限制、重入规则；嵌套失败的原子性有集成测试。
- 沙箱：编译与执行分别隔离，进程/内存/文件/网络限制及逃逸负例；后端选择需在原生隔离与 WASM 原型的可测成本基础上决定。
- 并行：状态事务完成后检测读写冲突，验证并行结果与确定性串行结果相同。

这份计划不把静态检查等同于沙箱，也不把启发式 EstimateGas 或一次 DryRun 当成其他输入的严格上界。


## 当前进度（2026-10-24）

第一批 1–6 已实施并验证，变更保留在 dev 工作区，尚未提交。
新增 19 个回归测试；全量测试、race、vet、diff 检查均通过。独立代码审查提出的三项问题均修复并回归。
本批额外修复匿名函数漏计 Gas、入口 Gas panic 的结构化错误，以及调用 Gas 上限不能提升引擎上限。
后续阶段尚未实施，按 todo.md 的 B1–D3 逐项跟踪。

## 第二批：B1 确定性策略

采用显式 `VMConfig.RequireDeterminism`，默认保持开发模式兼容。启用时即使可选安全开关为 false，也强制原安全检查和确定性 profile。策略版本 `go-deterministic-v1`：64 位平台、整数/布尔/字符串、结构与顺序控制；禁止 time/math/fmt 导入、浮点/复数、指针语法、uintptr、panic/print/println/new，以及 strconv 的浮点或平台查询 API。strconv 按符号白名单，contractapi 按现有业务能力白名单；禁用点导入和空导入以避免歧义和隐式初始化。链时间仅从 BlockTime 注入。

步骤：先添加策略负例、正例、不可关闭性和回放集成测试并确认失败；实现策略检查与 Compiler/GenerateABI 接入；固定构建目标与环境以保证 v1 的 64 位执行约束；补文档、跑完整测试/race/vet、独立审查。策略不宣称覆盖所有 Go 语义证明，也不把 wall clock 超时纳入共识结果；Gas/OOG 是执行规则，超时是节点基础设施失败。


B1 实施补充：严格模式产物携带 SourcePolicy，写入部署元数据；Compile 缓存按策略分离，Deploy/Execute 检查匹配。标签仅防可信流程误用，摘要与认证仍属 B3。部署地址目前仍是本地标识，确定性交易地址方案后续单列。


## 第二批验收结果

B1 已完成，新增 8 个测试覆盖源码能力限制、不可关闭性、相同输入的返回值/事件/Gas 回放、开发产物拒绝、策略缓存隔离、元数据重载、编译指令及环境约束。全量测试与 race 均通过，vet/diff 检查通过；独立复审无 P1/P2。
后续 B2–D3 尚未实施。确定性部署地址单列为 B4，防止本地 Deploy 被误当作共识部署。

## 第三批：B2 单一新版 ABI / 调用协议

用户明确不需要兼容性，不实现 legacy 分支。ABI 与协议均为版本 1，缺失/不匹配版本要求重新编译部署。

- ExtractABI 保留源码描述能力，使用 Go 格式器正确还原数组、通道、变参等类型；ExtractCallableABI / GenerateABI / Compile 统一校验可调用标量（bool/string、全部整数宽度与 byte/rune、float32/64）。复合/命名类型、泛型及非末位 error 拒绝；最后一个 error 表示调用失败状态，不序列化为普通值。
- 生成器统一用内部编号变量，不使用合约参数名。参数必须严格匹配数量与 JSON 类型，null、数值字符串、溢出均拒绝。
- 请求/响应 version=1，失败携带 code+error。Go ExecutionError 可 errors.As / Unwrap。OOG 使用专用 panic 类型，和普通 panic 区分；宿主分别编码超时、取消、输出限制、协议错误、进程失败。
- 协议版本落盘，部署/加载/执行均验证；无兼容路径。错误文本仅供诊断，程序以稳定错误码分支。
- 先写失败回归，再实现；更新旧测试夹具和示例为新协议；跑完整测试/race/vet并独立审查。


## 第三批验收结果

B2 已完成，新增 16 个测试覆盖全部 16 种标量、错误返回、panic/OOG 区分、JSON 边界、协议/ABI 版本、入口标识隔离和事件类型。所有测试与 race、vet、diff 检查通过；complete/event 示例实跑通过。独立审查提出的预声明标识遮蔽已修复并回归，最终复审无 P1/P2。
按用户最新要求，无任何 legacy 协议兼容路径。后续从 B3 开始。
