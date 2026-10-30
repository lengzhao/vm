# C2 默认 Object 与执行事务实施计划

**Goal:** 将 C1 状态事务接入默认 Object、ProcessRunner 和 Engine，使失败及 DryRun 不提交。

**Architecture:** contractapi.DefaultObject 返回无身份参数的默认对象，Get/Set/Delete 访问字节字段。协议 v2 通过 stdout JSON 行请求和 stdin 回复提供同步宿主调用，最终结果仍为单个 JSON 响应。Engine 开启事务、绑定实际执行地址，只给 Runner 传递不带提交方法的 ObjectStore；执行和 Gas 验证成功后提交，其他路径回滚。默认状态后端为 state/memory。

**Tech Stack:** Go、标准库 os pipes / JSON、PluginKit。

## 已批准设计与边界

- 每合约一个默认 Object；多对象、所有权、转移后置。地址以 ExecutionRequest.Address 为准，不使用可覆盖 CallContext.ContractAddress 做授权。
- API：DefaultObject().Get(key) ([]byte,bool,error)、Set(key,value) error、Delete(key) error。Get 存在标志区分空值和缺失。
- 每个字段名最多 1024 字节、字段值最多 64 KiB；单调用最多 1024 个宿主操作；宿主回复和合约输出分别累计最多 1 MiB。固定 Get=5、Set=10、Delete=5 Gas；宿主验证调用 Gas 单调下界及最终 Gas，运行时在发请求前扣费。
- 独立 stdin 写管道，stdout 仅接受宿主请求帧后接单个最终响应。保留取消、超时、输出限制和 WaitDelay，错误不自动降级为空对象。
- Engine 独占提交：验证产物后 Begin，Runner 可写事务，普通成功且 Gas 通过才能 Commit；失败、nil result、OOG、超时、取消、提交冲突不发布数据或事件。成功 DryRun 也回滚，结果事件仅 Execute 成功提交后对外发布。
- 协议、runtime、Gas、编译缓存版本升级；旧产物必须重编译。新 API 必须加入安全白名单，管理/协议符号仍禁止；静态 Gas 画像对 Object 方法作保守识别并说明启发式边界。

## 实施步骤

1. 新增 contractapi Object API 与真实编译/部署/执行 E2E 失败测试；确认缺失 API/未提交行为。
2. 实现 core.ObjectStore、作用域绑定和 Engine 事务生命周期（包括 DryRun 入口），补单测确认失败事件抑制及提交失败；先失败再实现。
3. 新增双向协议及 contractapi 内嵌源码，升级版本/编译器/安全/Gas，先补宿主消息负例。
4. 注册图/default YAML 加 StateStore 依赖，更新最小插件测试、旧协议夹具和兼容根入口。运行定向测试。
5. 独立审查，修复确认的问题。文档与 TODO 同步，固定 Go 文件后依次跑全量普通、race、vet、diff 检查。所有变更保留 dev 工作区，未授权新提交/push。
