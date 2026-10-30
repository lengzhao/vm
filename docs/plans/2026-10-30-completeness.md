# VM 功能收敛实施计划

**Goal:** 补齐静态 Gas、开发者集成、Docker 验收和持久化运维入口。

**Architecture:** 保持 core.Engine、StateStore、Runner 的最小契约。Gas 画像补齐现有运行时操作，不改变实际扣费；schema v1 保留。持久化维护使用可选 StateBackupper/StateInspector 能力；备份在一致读事务中输出完整数据库（含恢复回执），恢复到新路径且验证后使用。容量先提供可观测指标、既有回执限额和运维规则，不自动删除影响重放语义的数据。

**Tech Stack:** Go 1.26.1、PluginKit、bbolt、GitHub Actions。

用户已在项目完整性审查后授权按建议处理；本轮直接执行，不自动提交、推送或部署。

## 1. 静态 Gas
- 为 CreateObject、Owner、Transfer、Destroy 以及链式对象方法新增画像/估算回归，先确认失败。
- 补价格表及 AST 调用识别，防止链式对象构造被方法名匹配截断；保留启发式限制。
- 更新编译流水线缓存身份，避免命中旧画像；运行 Gas 与对象集成测试。

## 2. 文档和集成
- 统一 architecture、default_library、gas_metering、详细设计入口及 todo 中过时描述。
- 新增开发者指南、API 入口参考及严格策略+对象+持久化恢复示例；可选 Docker 组合配置。
- 示例演示真实编译、部署、身份声明、恢复批次、停止/重开及事件去重键。

## 3. Docker CI
- 独立 Linux job 准备固定 Go 版本镜像，解析本地不可变 ID 和实际工具链版本。
- 显式启用真实隔离测试，普通/race 顺序执行，不改变测试预算。
- 本机 daemon 可用则实测，否则明确记录尚未验证远端运行。

## 4. 备份恢复与容量
- 先补一致备份、未提交写隔离、pending 批次与回执恢复、取消/写失败/生命周期回归。
- 添加独立 core 维护能力和 bbolt 实现，维护操作与 Stop/Commit 序列化。
- 运维 CLI 提供 backup/status/restore；源只读且摘要不变，拒绝覆盖目的，失败清理半成品；恢复保留 pending 身份。
- 文档说明完整恢复集合（数据库+产物+请求+执行配置）、磁盘/内存指标、容量告警及不支持自动迁移/回执清理的原因。

## 5. 验证
- 针对性测试、运行完整示例和 CLI 恢复演练。
- 依次 go test -p 1 -timeout 5m ./...、go test -race -p 1 -timeout 5m ./...、go vet ./...、git diff --check。

## 验收结果（2026-10-30）

- 多对象静态画像回归先失败后通过；CreateObject 的直接函数估算从 11 修正为 31，Owner/Transfer/Destroy 成本按 runtime 常量计入。流水线升级 pipeline-v14-owned-gas-profile，实际运行扣费与 schema 不变。
- 新增 developer_guide / operations、integration 示例、objects-docker 组合模板；收敛当前入口与各能力文档的旧后置描述。没有改写历史实施计划来伪造历史进度。
- 新增独立 Docker CI，准备镜像后冻结本地不可变 ID，显式执行普通/race 隔离、事务及完整对象恢复测试，并检查残留容器。远端 workflow 尚未触发。
- 可选备份/统计能力独立于 core.Engine/StateStore；备份与提交/关闭串行，状态、回执和 pending fence 同步恢复。CLI 源只读、目的原子发布且拒绝覆盖；源 schema 必须已初始化。取消/Writer 失败/短写、未提交写排除、并发提交一致性、只读源摘要不变和外来空 bbolt 拒绝均有回归。
- 独立只读审查发现『读源要求写权限』和『外来空 bbolt 被初始化为 VM』两项 P2，均补失败回归后修复；最终增量复核无新 P1/P2。
- 固定最终功能代码后，显式启用本地 Docker 的全量普通测试、全量 race、go vet、git diff --check 通过；assembly 普通 36.249s、race 88.720s。host Go1.26.1，已有本地 Docker 镜像实际 Go1.26.8；CI 按 go.mod 准备镜像。无测试容器残留。未放宽测试预算。
- 实际运行完整示例返回 hello 并重开重放 2 回执；CLI status / backup / restore 通过，恢复出的库重新运行示例仍能读取 hello 并返回相同回执。
- 验证日志：/private/tmp/vm-completeness-tests-final.log、/private/tmp/vm-completeness-race-final.log、/private/tmp/vm-completeness-vet-final.log。使用 GOCACHE=/private/tmp/vm-review-gocache。
- 容量管理本轮提供一致指标、既有批次限额和操作规程；未引入自动 schema 迁移、历史回执/对象墓碑删除或外部事件投递服务。保持宿主认证、产物/请求保存和事件去重责任边界。
- 全部改动留在 dev 工作区，未提交、推送或部署。
