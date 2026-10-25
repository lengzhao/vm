# 确定性源码策略 v1

## 定位与启用

`RequireDeterminism` 是可选的、版本化的源码策略。默认开发模式不承诺确定性。协议与 ABI 仅支持当前版本，旧产物需要重新编译部署。

```go
engine := vm.NewVMEngine(vm.VMConfig{
    RequireDeterminism: true,
    EnableGasMetering: true,
    MaxGasLimit: 100000,
    ContractStorageDir: "./contracts",
})
```

启用后 Compile 和 GenerateABI 始终运行基础安全审查与确定性策略，即使 EnableSecurityChecks 为 false。独立编译器可用 NewContractCompilerWithPolicy(dir, true, true)。ReviewDeterministic 可用于预检查，后续编译仍负责 Go 类型检查与入口可调用性。

策略版本为 `go-deterministic-v1`。接受的源码能力或执行语义改变时应升级策略版本。严格模式的 Deploy / Execute 拒绝开发模式或缺少策略标签的旧产物，需用严格模式重新编译部署。SourcePolicy 存入 metadata.json；不同策略使用独立编译缓存。

## 允许的子集

- 64 位平台的整数、布尔、字符串、数组、切片、结构体与顺序控制；基础审查继续禁止 map、goroutine、channel、select、goto、包级可变变量等。
- 内部函数、匿名函数、方法及递归仍可使用，受现有 Gas 控制点约束。公开 ABI 支持各宽度整数、byte/rune、bool/string；开发模式的浮点仍被确定性策略拒绝。复合公开参数/返回尚未接入，末位 error 用作状态。
- import 仅允许 strconv、errors、github.com/lengzhao/vm/contractapi，允许普通别名，不允许点导入和空导入。
- strconv 仅开放整数/布尔的 Format、Parse、Append，以及 Quote / Unquote / rune 引用与字符分类；完整符号列表以 determinism.go 为准。ParseInt/ParseUint 的 bitSize=0 采用本策略的 64 位布局。
- errors 仅允许 New。
- contractapi 仅允许 BlockHeight、BlockTime、Sender、ContractAddress、Log。
- 链时间、发送方、区块高度等必须来自 CallContext / Host 的值快照，不读取操作系统时间。

## 拒绝的能力

- time / fmt / math 以及其他未列出的包，避免墙钟、标准输入输出、指针格式化及浮点数学来源。
- 浮点/虚数字面量、float32/float64/complex64/complex128、complex/real/imag、uintptr。
- 指针类型、解引用、取地址和 new，避免可观测的分配身份与地址依赖。
- panic、print、println；runtime 保留的 __vm 前缀、init、recover 和 contractapi 管理符号继续由基础边界检查拒绝。
- //go: 编译器指令。
- strconv 的浮点转换、IntSize、Atoi / Itoa 等未明确列入的符号。建议显式 ParseInt(s, 10, 64) / FormatInt。

检查针对包符号引用，包括函数值别名；局部变量遮蔽导入名按作用域处理，不把普通结构体字段误判为包 API。

## 构建与回放约束

严格模式将构建目标固定为本 VM 的 GOOS / GOARCH，要求 64 位，禁用 CGO；清除 GOFLAGS、GOEXPERIMENT，关闭外部 go.work，并使用 local 工具链选择。它不会继承调用方设置的交叉编译目标。

验收是同一受信任产物、相同参数和 Host 值快照，重复调用产生相同的返回数据、事件和 Gas。策略不改变本地部署地址生成方式；目前地址仍包含本地时间，因此不能把本地 Deploy 当作共识部署协议。

以下能力仍后置：固定 Go 发行版与 Unicode/标准库版本、完整构建清单、二进制/ABI/Gas 规则摘要、来源认证、跨节点兼容性验证。SourcePolicy 是由受信任编译/部署流程维护的标签，不能证明外部二进制安全，也不防恶意调用方篡改元数据。

wall clock 超时、进程启动失败、主机内存不足属于节点基础设施失败，不能当作跨节点一致的合约业务结果。Gas 上限和显式合约结果才属于后续共识执行协议需要固定的规则。

## 安全边界

这是一项保守的源码限制策略，不是对全部 Go 语义的形式化证明，也不是操作系统沙箱。ProcessRunner 的完整进程树、CPU/内存、文件与网络隔离仍未完成。关闭可选策略后运行的开发产物也不会自动获得确定性。
