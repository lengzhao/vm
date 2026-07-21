# 执行环境设计文档

## 1. 概述

当前阶段执行环境实现为最小 `Runner`：独立进程 + 超时 + JSON 输入输出。完整沙箱（系统调用限制、Docker）后置。

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
- 后续可增强：Docker、seccomp、内存上限

## 5. 并行执行（后置）
对象隔离与 ReadList/WriteList 并行判断由集成方负责；VM 库先保证单次调用正确性。

## 6. 作为开源库集成
本项目是 Go 库，不提供业务 `main.go`。集成方通过 `NewVMEngine` 使用 Compile/Deploy/Execute，并在后续实现默认库 Host 后端。
