# 执行环境模块详细设计文档（更新版）

## 1. 引言

### 1.1 编写目的
本文档详细描述执行环境模块的设计与实现，确保智能合约在受限环境中安全执行。此版本基于模块化架构设计进行了更新。

### 1.2 术语定义
- ExecutionEnvironment: 执行环境
- Sandbox: 沙箱环境
- Resource Control: 资源控制
- Isolation: 隔离

## 2. 概述

### 2.1 功能概述
执行环境模块提供合约执行环境，包括：
- 沙箱执行环境
- 资源控制
- 执行监控
- 环境隔离

### 2.2 架构图
```mermaid
graph TD
A[执行环境模块] --> B[沙箱执行器]
A --> C[资源控制器]
B --> D[进程管理器]
C --> E[资源监控器]
```

## 3. 详细设计

### 3.1 核心数据结构

#### 3.1.1 ExecutionEnvironment 结构体
```go
type ExecutionEnvironment struct {
    config ExecutionConfig
    sandboxExecutor *SandboxExecutor
    resourceController *ResourceController
}
```

#### 3.1.2 ExecutionConfig 配置结构
```go
type ExecutionConfig struct {
    // 资源限制
    ResourceLimit ResourceLimit
    
    // 执行超时
    ExecutionTimeout time.Duration
    
    // 是否启用沙箱
    EnableSandbox bool
    
    // 工作目录
    WorkingDir string
    
    // 环境变量
    EnvironmentVariables map[string]string
}
```

#### 3.1.3 ExecutionResult 执行结果
```go
type ExecutionResult struct {
    // 执行结果数据
    Data []byte
    
    // Gas消耗
    GasConsumed uint64
    
    // 执行时间
    ExecutionTime time.Duration
    
    // 是否成功
    Success bool
    
    // 错误信息
    Error error
    
    // 资源使用情况
    ResourceUsage ResourceUsage
}
```

### 3.2 核心接口设计

#### 3.2.1 ExecutionEnvironment 接口
```go
type ExecutionEnvironment interface {
    // Run 在执行环境中运行合约
    Run(contract CompiledContract, function string, args ...interface{}) (*ExecutionResult, error)
    
    // SetResourceLimit 设置资源限制
    SetResourceLimit(limit ResourceLimit)
    
    // GetResourceUsage 获取资源使用情况
    GetResourceUsage() ResourceUsage
    
    // Stop 停止执行环境
    Stop() error
    
    // GetEnvironmentInfo 获取环境信息
    GetEnvironmentInfo() *EnvironmentInfo
}
```

### 3.3 核心功能实现

#### 3.3.1 执行流程
```mermaid
graph TD
A[输入执行参数] --> B[环境准备]
B --> C[资源限制设置]
C --> D[沙箱环境启动]
D --> E[执行合约]
E --> F[结果收集]
F --> G[资源回收]
G --> H[返回执行结果]
```

#### 3.3.2 资源监控流程
```mermaid
graph TD
A[开始执行] --> B[启动资源监控]
B --> C[定期检查资源使用]
C --> D{超出限制?}
D --> |是| E[终止执行]
D --> |否| F[继续监控]
F --> G{执行完成?}
G --> |是| H[停止监控]
G --> |否| C
```

## 4. 模块设计

### 4.1 沙箱执行器模块

#### 4.1.1 功能描述
负责在沙箱环境中执行合约代码。

#### 4.1.2 接口设计
```go
type SandboxExecutor interface {
    // Execute 在沙箱中执行
    Execute(executablePath string, args ...string) ([]byte, error)
    
    // SetSandboxConfig 设置沙箱配置
    SetSandboxConfig(config SandboxConfig)
    
    // IsSandboxEnabled 检查沙箱是否启用
    IsSandboxEnabled() bool
    
    // GetSandboxInfo 获取沙箱信息
    GetSandboxInfo() *SandboxInfo
}
```

#### 4.1.3 实现细节
1. 使用操作系统级隔离机制
2. 限制文件系统访问
3. 限制网络访问
4. 限制系统调用

### 4.2 资源控制器模块

#### 4.2.1 功能描述
负责监控和控制合约执行过程中的资源使用。

#### 4.2.2 接口设计
```go
type ResourceController interface {
    // ApplyLimits 应用资源限制
    ApplyLimits(limit ResourceLimit)
    
    // GetCurrentUsage 获取当前资源使用情况
    GetCurrentUsage() ResourceUsage
    
    // StartMonitoring 开始监控
    StartMonitoring()
    
    // StopMonitoring 停止监控
    StopMonitoring()
    
    // CheckLimits 检查是否超出限制
    CheckLimits() error
}
```

#### 4.2.3 实现细节
1. 使用cgroups (Linux) 或类似机制限制资源
2. 定期采样资源使用情况
3. 超出限制时终止执行

## 5. 环境隔离

### 5.1 文件系统隔离
```go
type FileSystemIsolator interface {
    // Mount 挂载隔离文件系统
    Mount() error
    
    // Unmount 卸载隔离文件系统
    Unmount() error
    
    // GetRoot 获取根目录
    GetRoot() string
}
```

### 5.2 网络访问控制
```go
type NetworkController interface {
    // SetPolicy 设置网络策略
    SetPolicy(policy NetworkPolicy)
    
    // CheckAccess 检查网络访问权限
    CheckAccess(host string, port int) error
}
```

### 5.3 系统调用限制
```go
type SyscallFilter interface {
    // FilterSyscall 过滤系统调用
    FilterSyscall(syscallNum int, args ...uintptr) error
    
    // IsAllowed 检查系统调用是否被允许
    IsAllowed(syscallNum int) bool
}
```

## 6. 安全设计

### 6.1 多层隔离
通过多层隔离机制确保合约安全执行：
1. 进程级隔离
2. 文件系统隔离
3. 网络访问控制
4. 系统调用过滤

### 6.2 资源限制
严格限制合约可使用的系统资源，防止资源滥用。

### 6.3 执行超时
设置执行超时机制，防止恶意合约长时间占用系统资源。

## 7. 性能优化

### 7.1 轻量级实现
使用轻量级技术实现沙箱环境，减少性能开销。

### 7.2 资源复用
支持沙箱环境的复用，减少创建和销毁开销。

### 7.3 异步监控
使用异步方式监控资源使用，减少对执行性能的影响。

## 8. 错误处理

### 8.1 错误分类
- 资源超限错误
- 执行错误
- 系统调用拒绝错误
- 文件系统访问错误
- 网络访问拒绝错误
- 执行超时错误

### 8.2 错误码设计
```go
const (
    // 资源相关错误
    ErrMemoryLimitExceeded = 1001
    ErrCPULimitExceeded = 1002
    ErrStorageLimitExceeded = 1003
    ErrNetworkLimitExceeded = 1004
    
    // 执行相关错误
    ErrExecutionFailed = 2001
    ErrExecutionTimeout = 2002
    ErrExecutionPanic = 2003
    
    // 系统调用相关错误
    ErrSyscallNotAllowed = 3001
    ErrSyscallFailed = 3002
    
    // 文件系统相关错误
    ErrFileAccessDenied = 4001
    ErrFileSystemFull = 4002
    
    // 网络相关错误
    ErrNetworkAccessDenied = 5001
    ErrNetworkTimeout = 5002
    
    // 系统相关错误
    ErrSystemError = 6001
)
```

### 8.3 错误信息结构
```go
type ExecutionError struct {
    Code       int
    Message    string
    Resource   string // 相关资源
    Usage      uint64 // 资源使用量
    Limit      uint64 // 资源限制
    Err        error
}
```

## 9. 测试设计

### 9.1 单元测试
为每个执行环境模块编写单元测试，确保功能正确性。

### 9.2 集成测试
编写集成测试，验证整个执行环境的功能。

### 9.3 安全测试
编写安全测试，验证执行环境的安全性。

### 9.4 性能测试
编写性能测试，验证执行环境的性能指标。

## 10. 部署与运维

### 10.1 系统要求
- Linux 内核 3.10+
- cgroups 支持
- 足够的系统资源

### 10.2 配置管理
```yaml
execution:
  resource_limit:
    memory_limit: 104857600 # 100MB
    cpu_time_limit: 5000 # 5秒
    file_descriptor_limit: 100
    network_bandwidth_limit: 1048576 # 1MB/sec
    storage_limit: 10485760 # 10MB
  execution_timeout: 30s
  enable_sandbox: true
  working_dir: "./execution_env"
```

### 10.3 监控指标
- 资源使用率
- 执行成功率
- 平均执行时间
- 安全违规事件

## 11. 与其他模块的交互

### 11.1 与虚拟机引擎的交互
```go
// VMEngineConfig 虚拟机引擎配置
type VMEngineConfig struct {
    ExecutionEnv       ExecutionEnvironment  // 执行环境模块
    // 其他模块...
}
```

### 11.2 与Gas计费模块的交互
执行环境模块需要与Gas计费模块协作，监控Gas消耗。

### 11.3 数据传输对象
```go
// 执行请求
type ExecuteRequest struct {
    Contract CompiledContract
    Function string
    Args     []interface{}
    GasLimit uint64
}

// 执行响应
type ExecuteResponse struct {
    Result *ExecutionResult
    Error  error
}
```

## 12. 附录

### 12.1 资源限制结构
```go
type ResourceLimit struct {
    // 内存限制 (bytes)
    MemoryLimit uint64
    
    // CPU时间限制 (milliseconds)
    CPUTimeLimit uint64
    
    // 文件描述符限制
    FileDescriptorLimit uint64
    
    // 网络带宽限制 (bytes/sec)
    NetworkBandwidthLimit uint64
    
    // 存储空间限制 (bytes)
    StorageLimit uint64
}
```

### 12.2 资源使用情况
```go
type ResourceUsage struct {
    MemoryUsage  uint64 // bytes
    CPUUsage     uint64 // milliseconds
    FDUsage      uint64 // file descriptors
    NetworkUsage uint64 // bytes
    StorageUsage uint64 // bytes
    StartTime    time.Time
    EndTime      time.Time
}
```

### 12.3 接口依赖关系
```mermaid
graph TD
A[VMEngine] --> B[ExecutionEnvironment]
B --> C[SandboxExecutor]
B --> D[ResourceController]
C --> E[进程管理器]
D --> F[资源监控器]
```