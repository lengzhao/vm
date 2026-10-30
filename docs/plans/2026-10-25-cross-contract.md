# C3 跨合约调用实施计划

**Goal:** 使用一个临时 store 完成整条调用链，成功后一次性发布，失败或 DryRun 直接丢弃。

**Architecture:** 复用 StateStore.Begin 返回的事务作为临时 store，不增加保存点/补偿回滚。Engine 创建调用会话和每帧的窄 ContractCaller，所有帧共享事务。Call 通过现有双向通道发起；Gas 在父帧累计子帧实际消耗，事件在调用点按顺序拼接。

**Tech Stack:** Go、JSON 行协议、PluginKit。

## 已确认语义

- contractapi.Call(address,function,args...) ([]byte,error)，返回子调用 JSON 字节。任何子调用失败都使整链失败，即使调用方忽略错误。
- 临时 store 保存所有地址的本次覆盖；只有最外层成功且 Gas 通过才 Commit，其他路径 Rollback 即丢弃。不把逐键正式 Put 当作发布方式，保留原子提交。
- 调用栈内地址不得重入（含自调用、A→B→A）；根帧计一层，默认最多 8 层，可配置 1–32。顺序重复调用已经返回的地址允许。
- Sender 为直接调用合约的真实地址；子 ContractAddress 为真实目标，区块高度/时间和根 context 截止时间沿用最外层。
- Call 固定成本 10 Gas，父进程报告调用点已用 Gas，子帧仅获得剩余预算；回复带子树实际 Gas，由父 runtime 扣除且宿主校验最终下界，根计量一次。不允许剩余零变成无限 Gas。
- 每会话最多 1024 个跨合约调用；跨合约结果最多 64 KiB、整链事件编码累计最多 1 MiB；保留每进程消息上限与取消关闭管道。
- Call 请求携带父帧在调用前缓冲的事件，宿主在该位置插入子帧事件；最终追加父帧剩余事件。整链失败和 DryRun 不发布。
- 协议/runtime/Gas/源码策略/编译缓存升级 v3；ABI/Profile schema 不变，旧产物重新编译部署。

## 实施与验证

1. 先写 Engine 会话单测、API 帧/扣费单测和真实合约跨调用失败测试，观察缺失 Call 能力失败。
2. 新增 core.ContractCaller 与每帧能力，重用现有临时事务；验证唯一 Begin/Commit、跨地址读写、嵌套失败粘性、深度/重入与 Gas。
3. 扩展进程通道 Call 帧，补类型/大小/版本/Gas/后端失败测试；runtime 内嵌与安全白名单同步。
4. 配置及示例/文档/TODO 同步。独立审查后固定代码，顺序运行全量普通测试、race、vet、git diff --check。C1–C3 均保留当前 dev 工作区，用户另行要求时再提交/push。
