# 执行环境设计文档

## 1. 概述

默认 `ProcessRunner` 提供独立进程、超时与双向 JSON 协议。D1 已实现可选 Docker 编译与执行沙箱，D2 已新增 compiler/wasip1 与 runner/wazero 插件；配置及资源边界见 [沙箱指南](sandbox_roadmap.md)。下方历史协议示例以 [当前调用协议](call_protocol.md) 为准。

## 2. 编译与执行

### 2.1 编译
1. AST 安全审查
2. Gas 注入与入口生成
3. 默认使用 `go build` 生成本地可执行文件
4. TinyGo 作为后续可选构建器，不阻塞主链路

### 2.2 执行
1. `ProcessRunner` 使用 `exec.CommandContext`
2. stdin 传入调用请求 JSON
3. stdout 读取执行结果 JSON
4. 超时由 `VMConfig.ExecutionTimeout` 控制

## 3. 数据交互格式

### 请求
```json
{
  "function": "transfer",
  "args": ["0xabc", "0xdef", 1000]
}
```

### 响应
```json
{
  "ok": true,
  "result": null,
  "gas": 42
}
```

失败时：
```json
{
  "ok": false,
  "error": "gas limit exceeded",
  "gas": 100
}
```

## 4. 隔离与资源限制（MVP）
- 进程隔离
- 执行超时
- Gas 上限（`VM_GAS_LIMIT`）
- 可选 Docker：编译/执行禁网、文件隔离、CPU/内存/PID/tmpfs 限制；默认 ProcessRunner 无这些 OS 保证

## 5. 可选有序并行执行
D3 提供 `state/mvcc` 与 `batch/ordered`：按键版本检测冲突，有界并行推测、按输入顺序提交及冲突重执行。需无外部并发写入，非整批原子提交，详见 [有序并行执行](parallel_execution.md)。

## 6. 作为开源库集成
本项目是 Go 库，不提供业务 `main.go`。集成方通过 `NewVMEngine` 使用 Compile/Deploy/Execute，并在后续实现默认库 Host 后端。
