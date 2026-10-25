# B4 确定性部署实施计划

**Goal:** 用显式部署输入及产物身份派生版本化、与时间无关的地址；重复部署不覆盖已有记录。

**Architecture:** core 声明 DeploymentInput/Request/Record 和仅创建 ContractCreator；ContractDeployer 接收请求，依赖 Creator 与 ArtifactValidator。internal/deployment 提供纯地址规则与部署编排；Store 实现 Create 的原子发布及相同身份重试，Put 保持替换。部署记录独立于编译清单，持久化输入并在加载时检查地址、清单摘要与记录一致性。根包旧入口采用固定本地域，新增显式 DeployWithInput/DeployContext。

**Tech Stack:** Go 标准库、SHA-256、长度前缀二进制编码、现有 PluginKit Config/Deps。

## 规则与边界

- domain 为 ASCII `vm-deployment-v1` 加 NUL；依次编码 ChainID 与 Sender（UTF-8，各 1–256 字节，uint32 大端长度前缀）、Nonce（uint64 大端）、Salt（32 字节）、ManifestSHA256（32 字节原始摘要）。输出 `contract_` 加完整 SHA-256 小写十六进制。
- 相同输入和同一产物得到同一地址；ChainID/Sender/Nonce/Salt/产物任一改变都会改变地址。规则版本记录为 1。
- 同地址重复 Create 在验证已存内容和部署身份完全一致时无写入成功，否则明确冲突；无修复/覆盖路径。独立 Store 实例通过完整目录原子 rename 避免互相覆盖。
- 这是地址和输入一致性规范，不提供交易签名认证、账户 nonce 记账、状态事务或升级；调用链应认证 Sender 并检查 nonce 使用规则。

## 步骤

1. RED：先复现同产物重复部署地址不一致，新增独立固定向量、输入变化/非法输入和不修改原产物的单测。
2. core/contract.go、core/compiler.go 增加请求、记录及 Creator 能力；internal/deployment 新地址规则与 Create 调用；Engine、根包与 compat 适配；更新 native 调用方和最小接口插件测试。
3. Store 重用完整快照写入，新增原子 Create、相同身份无写入重试、冲突拒绝、部署记录持久化与加载校验。保留 Put 替换测试；增加跨实例并发和已有损坏记录拒绝回归。
4. artifact 验证声明的 Deployment 记录；native 路径要求有地址产物携带部署输入。补部署输入篡改/缺失/版本错误、取消无副作用、不同节点重复执行回归。
5. 更新 docs/deployment.md、插件依赖与示例、README/TODO。顺序运行普通全量、race 全量、vet、diff 检查，独立审查；不自动提交。

## 验收结果

2026-10-25：B4 完成。先复现同产物重复部署地址不同，再实现确定性输入及仅创建发布；新增 11 个测试函数，独立固定向量与跨实例并发等回归通过。全量普通、race、vet、差异检查通过；独立审查及增量复核无 P1/P2。响应字段测试的短预算在隔离重复中复现基础设施超时，仅调整此格式测试为 5 秒，20 次重复通过，未修改生产或取消测试逻辑。变更未提交，下一项 C1。
