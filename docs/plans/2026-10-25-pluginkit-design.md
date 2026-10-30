# VM 完整插件化设计

用户已确认完整插件化，保留原有 API 兼容入口。

依赖方向：根包兼容 API → assembly → plugins → 实现与 core；core 不依赖实现或 PluginKit。
PluginKit v0.1.6 是唯一注册与依赖图构建机制，沿用 GoChainKit 的 Register、Config/Deps、build.Build。

能力覆盖：源代码策略、安全审查、ABI、Gas 注入、静态画像/估算、每次调用的 Gas 工厂、编译器、合约存储、Runner、引擎编排。
引擎及编译器只消费注入的能力，不自行创建子模块。每次调用必须创建独立 Gas 计量器。

默认预设保持已有安全检查、确定性策略、Gas、调用协议及产物校验行为；合约侧 contractapi 不依赖 PluginKit。
兼容构造函数委托默认插件图，新装配 API 返回错误及构建结果。注册通过显式包导入，第三方使用 core 接口扩展。
配置使用 root/plugins 和 use/config/deps；拒绝未知顶层字段、多文档和超大配置。
生命周期按依赖顺序启动、反向停止；装配/启动失败回收已创建资源，停止幂等。

验证：先增加插件替换、依赖失败、独立 Gas、配置与生命周期测试，再实现；运行原有测试、race 与 vet。
现有未提交的加固工作必须完整保留，不从 HEAD 重建源码。
