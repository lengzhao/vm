# B3 产物清单与摘要校验实施计划

**Goal:** 完成工具链固定之后的 B3：持久化构建与规则清单，在缓存、部署、加载和执行前拒绝不完整或被修改的产物。

**Architecture:** core 仅声明清单数据；internal/artifact 提供共享 SHA-256、清单生成及完整性检查，policy 负责运行规则兼容性。Compiler 将 artifact 与 manifest.json 放入独立缓存目录原子发布；Store 写入完整快照后复核再发布，读取固定文件名且拒绝损坏的可选画像。旧产物没有清单则要求重新编译部署；摘要不是来源认证，不承诺对抗有权限同时重写清单和产物的攻击者。

**Tech Stack:** Go 标准库、SHA-256、规范 JSON、现有 PluginKit 能力接口。

## 实施步骤

1. 先补失败测试：缓存二进制篡改、Deploy/Load/Execute 拒绝篡改、缺失清单/规则版本、画像损坏；观察失败。
2. core/artifact.go 增加版本化构建清单数据；internal/artifact 实现有 context 的流式摘要、清单构造、结构和摘要校验。
3. Compiler 记录源码/插桩/入口/runtime/工具链摘要以及 ABI/协议/Gas 规则版本；缓存 key 覆盖清单输入与 ABI/画像。缓存目录完整发布，命中时与本次期望清单和二进制校验；损坏项明确失败，不静默重签。
4. Store 持久化 manifest.json，固定 artifact 名称；Get 拒绝画像 JSON 错误或摘要不符；Put 复制后复核再原子替换。默认 validator 与 ProcessRunner 在执行前验证，延续协议错误码，缺失/未知版本返回 unsupported version、摘要不符返回 artifact integrity。
5. 旧人工测试夹具改为带清单的有效产物，保留原本取消、超时、错误协议等验证；不添加兼容绕过。第三方自定义规则由自定义 ArtifactValidator 决定，存储只检查完整性。
6. 更新 docs/artifact_manifest.md、plugins.md、确定性说明与 TODO，顺序运行普通全量、race 全量、vet、diff 检查，并独立审查。保留此前工具链和删除 cap 的变更，不自动提交。


## 验收结果

2026-10-25：B3 清单部分完成。先验证缓存篡改/部署篡改/缺失清单失败回归，再实现；额外先复现 Engine.Get 接受不兼容规则和缓存描述文件错误码缺失，再修复。新增 10 个测试函数，并加强原有同源码并发编译发布测试。最终普通全量、race 全量、vet、diff 检查全部通过；独立审查与增量复核无 P1/P2。B3 已在 TODO 标记完成，下一项 B4。未提交。
