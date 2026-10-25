# ABI 与调用协议 v1

本项目当前只支持 ABI schema v1 和调用协议 v1。没有兼容旧格式的分支；旧产物、缺失版本或未知版本必须重新编译部署。

## ABI 类型契约

`abi.ExtractABI` 描述源文件中的公开包级函数，并保留 Go 类型语法（例如 `[3]int`、`<-chan int`、`...bool`）。描述能力不代表可直接调用。

`abi.ExtractCallableABI`、`VMEngine.GenerateABI` 和 Compile 共享可调用签名校验：

- 标量：bool、string、int/int8/int16/int32/int64、uint/uint8/uint16/uint32/uint64、byte、rune、float32/float64。
- 允许零个、一个或多个标量返回值。公开复合类型、命名类型与泛型不支持，直接返回 UNSUPPORTED_ABI。
- 仅允许最后一个返回值为 error；error 表示状态，不属于业务数据。非 nil 时返回 CONTRACT_ERROR；nil 时剔除末位 error，再编码业务返回值。
- 包级重定义预声明名称会被拒绝，以免覆盖入口依赖的 bool/string/any/len/true/nil 等含义。入口导入与局部变量使用 __vm 内部名称，合约参数 req/err/result/_ 不产生冲突。
- RequireDeterminism 在这份类型集合上继续收紧，例如拒绝浮点，详见确定性策略文档。

ABI JSON 顶层必须包含 `"version":1`。协议版本同时保存在 CompiledContract 与 metadata.json 的 protocol_version 字段；Store、Load、Runner 都检查版本。标签不代替二进制完整性或来源认证，认证仍属 B3。

## 请求

```json
{"version":1,"function":"Add","args":[10,20]}
```

function 是公开包级函数名。args 必须是数组，数量与 ABI 严格一致。标量值必须使用对应 JSON 原生类型：整数/浮点使用数字，bool 使用布尔，string 使用字符串。

null、数值字符串、溢出整数、缺参和多参均返回 INVALID_ARGUMENTS。不隐式转换类型。Go 调用方应使用 int64/uint64/json.Number 传递大整数，避免在发送前就经过 float64 丢失精度。

请求最多 1 MiB。未知字段、多条请求或非法结构返回 INVALID_REQUEST。Runner 自动给 Go CallRequest 加入当前协议版本；直接调用产物的客户端必须显式填写 version。

## 响应

成功：

```json
{"version":1,"ok":true,"result":30,"gas":11}
```

失败：

```json
{"version":1,"ok":false,"code":"INVALID_ARGUMENTS","error":"wrong argument count for Add","gas":10}
```

version、ok、gas 必填。仅允许一个 JSON 响应，未知字段与矛盾状态均拒绝。失败必须有已知 code 和非空 error，不能夹带业务结果；成功不能包含错误码或错误文本。

业务返回值：无返回值/仅 nil error → null；单值 → 标量；多值 → 数组。NaN/Inf 或其他不能编码的结果/事件返回 INVALID_RESULT，不再悄悄丢失响应。

通用 JSON 数值在 Runner 中解码为 json.Number，因此返回值和事件保留 uint64/int64 精度。每个输出流最多 1 MiB，超限取消直接进程。

## 稳定错误码

合约响应可能返回 INVALID_REQUEST、INVALID_ARGUMENTS、UNKNOWN_FUNCTION、CONTRACT_ERROR、CONTRACT_PANIC、GAS_LIMIT_EXCEEDED、UNSUPPORTED_VERSION、INVALID_RESULT。

宿主还区分 INVALID_RESPONSE、EXECUTION_TIMEOUT、EXECUTION_CANCELED、OUTPUT_LIMIT_EXCEEDED、PROCESS_FAILURE，以及签名校验的 UNSUPPORTED_ABI。构建、存储、源码安全审查仍保留各自的错误原因；本节不承诺把所有文件系统和编译错误都改成执行错误。

Go 调用方使用 errors.As 获取 *vm.ExecutionError 并比较 Code。Unwrap 保留底层原因；取消和超时可分别用 errors.Is(err, context.Canceled / context.DeadlineExceeded) 检查。诊断文本不作为程序分支依据。

Gas 超限由运行时专用 GasLimitError 标识；普通 panic 的文本即使相同，也分类为 CONTRACT_PANIC。

## 事件描述

静态 ABI 只收集真实导入绑定的 contractapi.Log，支持普通别名、字符串字面量和直接字符串常量。局部对象的同名 Log 不被误识别。

简单参数、显式类型和转换可以推断字段类型；无法静态确认时记录 unknown。动态事件名、帮助函数中的间接事件不保证出现在静态 ABI 中。ExecuteResult.Events 才是一次调用实际事件的依据；失败调用的事件只是诊断，当前尚无事务提交机制。
