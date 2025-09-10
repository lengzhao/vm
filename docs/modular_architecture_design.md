# 智能合约虚拟机模块化架构设计

## 1. 引言

### 1.1 编写目的
本文档旨在提出一种合理的模块化架构设计，以解决当前架构中模块间耦合度高、接口设计不清晰、依赖关系复杂以及扩展性不足的问题。通过明确的模块划分和清晰的接口定义，提升系统的可维护性、可测试性和可扩展性。

### 1.2 术语定义
- **模块化架构**：将系统功能划分为独立模块，通过明确定义的接口进行交互的架构方式
- **耦合度**：模块间相互依赖的程度

## 2. 设计目标

1. **降低模块耦合度**：通过清晰的接口定义和依赖管理，减少模块间的直接依赖
2. **明确接口设计**：定义标准化的接口，提高系统的可维护性
3. **简化依赖关系**：通过接口抽象，简化模块间的依赖关系
4. **增强扩展性**：设计易于扩展的架构，支持新功能的快速集成

## 3. 整体架构设计

### 3.1 总体架构图

```
graph LR
    A[客户端/外部系统] --> B[API网关层]
    B --> C[虚拟机核心引擎]
    C --> D[安全审查模块]
    C --> E[编译器模块]
    C --> F[执行环境模块]
    C --> G[Gas计费模块]
    C --> H[ABI生成模块]
    C --> I[存储管理模块]
    C --> J[合约管理模块]
    
    D --> K[关键字分析器]
    D --> L[导入检查器]
    
    E --> M[TinyGo编译器适配器]
    E --> N[Gas注入器]
    E --> O[Main函数生成器]
    
    F --> P[沙箱执行器]
    F --> Q[资源控制器]
    
    G --> R[Gas计量器]
    
    H --> T[AST解析器]
    H --> U[函数识别器]
    H --> V[类型分析器]
    
    I --> W[文件存储适配器]
    I --> X[对象存储管理器]
    
    J --> Y[合约部署器]
    
    subgraph "核心模块层"
        C
    end
    
    subgraph "功能模块层"
        D
        E
        F
        G
        H
        I
        J
    end
    
    subgraph "子组件层"
        K
        L
        M
        N
        O
        P
        Q
        R
        T
        U
        V
        W
        X
        Y
    end
```

### 3.2 架构层次说明

整个架构分为三个层次：
1. **核心模块层**：虚拟机核心引擎作为系统的核心协调者
2. **功能模块层**：提供具体功能的独立模块
3. **子组件层**：功能模块内部的具体实现组件

## 4. 核心模块详细设计

### 4.1 虚拟机核心引擎 (VMEngine)

作为系统的入口点和协调者，负责协调各功能模块的工作。

```go
// VMEngine 虚拟机核心引擎接口
type VMEngine interface {
    // Compile 编译合约源代码
    Compile(sourceCode string) (CompiledContract, error)
    
    // Deploy 部署合约
    Deploy(contract CompiledContract) (Address, error)
    
    // Execute 执行合约函数
    Execute(address Address, function string, args ...interface{}) ([]byte, error)
    
    // EstimateGas 估算合约调用所需的Gas
    EstimateGas(address Address, function string, args ...any) (uint64, error)
    
    // GetContractABI 获取合约ABI
    GetContractABI(address Address) (ABI, error)
}
```

### 4.2 安全审查模块 (SecurityReviewer)

负责合约源代码的安全审查，包括关键字检查和导入检查。

```go
// SecurityReviewer 安全审查模块接口
type SecurityReviewer interface {
    // Review 对合约源代码进行安全审查
    Review(sourceCode string) (*ReviewResult, error)
    
    // IsKeywordAllowed 检查关键字是否被允许
    IsKeywordAllowed(keyword string) bool
    
    // IsImportAllowed 检查导入是否被允许
    IsImportAllowed(importPath string) bool
    
    // AddForbiddenKeyword 添加禁止关键字
    AddForbiddenKeyword(keyword string)
    
    // AddAllowedImport 添加允许导入
    AddAllowedImport(importPath string)
}
```

### 4.3 编译器模块 (ContractCompiler)

负责合约的编译处理，包括TinyGo编译、Gas注入和Main函数生成。

```go
// ContractCompiler 编译器模块接口
type ContractCompiler interface {
    // Compile 编译源代码
    Compile(sourceCode string) (*CompilationResult, error)
    
    // Validate 验证源代码
    Validate(sourceCode string) error
    
    // InjectGasMetering 注入Gas计费代码
    InjectGasMetering(sourceCode string) (string, error)
    
    // GenerateMainFunction 生成Main函数
    GenerateMainFunction(sourceCode string) (string, error)
}
```

### 4.4 执行环境模块 (ExecutionEnvironment)

提供合约执行环境，包括基础的执行接口和资源控制。

根据当前需求，执行器暂时不需要复杂实现，只需要有对应的接口，用cmd调用合约就行。

```go
// ExecutionEnvironment 执行环境模块接口
type ExecutionEnvironment interface {
    // Run 在执行环境中运行合约
    Run(contract CompiledContract, function string, args ...interface{}) (*ExecutionResult, error)
    
    // SetResourceLimit 设置资源限制
    SetResourceLimit(limit ResourceLimit)
    
    // GetResourceUsage 获取资源使用情况
    GetResourceUsage() ResourceUsage
}
```

### 4.5 Gas计费模块 (GasMetering)

负责Gas的计量和管理。

```go
// GasMetering Gas计费模块接口
type GasMetering interface {
    // ConsumeGas 消耗Gas
    ConsumeGas(amount uint64, description string) error
    
    // GetConsumedGas 获取已消耗的Gas
    GetConsumedGas() uint64
    
    // GetRemainingGas 获取剩余Gas
    GetRemainingGas() uint64
    
    // SetGasLimit 设置Gas限制
    SetGasLimit(limit uint64)
}
```

### 4.6 ABI生成模块 (ABIGenerator)

负责合约ABI的生成和管理。

```go
// ABIGenerator ABI生成模块接口
type ABIGenerator interface {
    // Generate 从源代码生成ABI
    Generate(sourceCode string) (*ABI, error)
    
    // Validate 验证ABI的正确性
    Validate(abi *ABI) error
    
    // Serialize 序列化ABI
    Serialize(abi *ABI) ([]byte, error)
}
```

### 4.7 存储管理模块 (StorageManager)

负责合约和相关数据的存储管理。

```go
// StorageManager 存储管理模块接口
type StorageManager interface {
    // StoreContract 存储合约
    StoreContract(contract CompiledContract) (Address, error)
    
    // LoadContract 加载合约
    LoadContract(address Address) (CompiledContract, error)
    
    // DeleteContract 删除合约
    DeleteContract(address Address) error
    
    // StoreABI 存储ABI
    StoreABI(address Address, abi ABI) error
    
    // LoadABI 加载ABI
    LoadABI(address Address) (ABI, error)
    
    // GetContractPath 获取合约存储路径
    GetContractPath(address Address) string
}
```

对象存储系统可以由外部提供，如果外部没有提供，默认使用go-leveldb。

### 4.8 合约管理模块 (ContractManager)

负责合约的生命周期管理。

合约部署成功后，会生成两个部分：
1. **可以被import的合约模块**：包含合约的源代码，供其他合约通过import语句引用和复用合约功能
2. **可以被调用的合约程序**：包含编译后的可执行二进制文件，供外部系统通过虚拟机引擎调用执行

```go
// ContractManager 合约管理模块接口
type ContractManager interface {
    // Deploy 部署合约
    Deploy(contract CompiledContract) (Address, error)
    
    // Undeploy 卸载合约
    Undeploy(address Address) error
    
    // GetContract 获取合约
    GetContract(address Address) (CompiledContract, error)
    
    // ListContracts 列出所有合约
    ListContracts(offset,limit int) ([]Address, error)
    
    // GetContractStatus 获取合约状态
    GetContractStatus(address Address) (ContractStatus, error)
}
```

## 5. 模块间接口设计

### 5.1 模块间通信

模块间通过明确定义的接口进行通信，降低耦合度：

```go
// VMEngine 虚拟机核心引擎实现
type vmEngineImpl struct {
    securityReviewer SecurityReviewer
    compiler         ContractCompiler
    executionEnv     ExecutionEnvironment
    gasMetering      GasMetering
    abiGenerator     ABIGenerator
    storageManager   StorageManager
    contractManager  ContractManager
}

// NewVMEngine 创建新的虚拟机引擎实例
func NewVMEngine() VMEngine {
    return &vmEngineImpl{
        securityReviewer: NewSecurityReviewer(),
        compiler:         NewContractCompiler(),
        executionEnv:     NewExecutionEnvironment(),
        gasMetering:      NewGasMetering(),
        abiGenerator:     NewABIGenerator(),
        storageManager:   NewStorageManager(),
        contractManager:  NewContractManager(),
    }
}
```

### 5.2 数据传输对象

定义清晰的数据传输对象，确保模块间数据传递的一致性：

``go
// CompiledContract 编译后的合约
type CompiledContract struct {
    // 合约可执行文件路径
    ExecutablePath string
    
    // ABI信息
    ABI ABI
    
    // 编译时间
    CompileTime time.Time
    
    // 源代码哈希
    SourceHash string
    
    // 合约地址
    Address Address
}

// CompilationResult 编译结果
type CompilationResult struct {
    // 编译后的合约
    Contract CompiledContract
    
    // 编译日志
    Logs []string
    
    // 编译是否成功
    Success bool
    
    // 错误信息
    Error error
}

// ReviewResult 审查结果
type ReviewResult struct {
    // 审查是否通过
    Passed bool
    
    // 错误信息列表
    Errors []ReviewError
    
    // 警告信息列表
    Warnings []ReviewWarning
    
    // 审查时间
    ReviewTime time.Time
}

// ExecutionResult 执行结果
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
}
```

## 6. 扩展性设计

### 6.1 配置管理

通过统一的配置管理机制支持模块的灵活配置：

```go
// Config 配置接口
type Config interface {
    // Get 获取配置项
    Get(key string) (interface{}, error)
    
    // Set 设置配置项
    Set(key string, value interface{}) error
    
    // LoadFromFile 从文件加载配置
    LoadFromFile(path string) error
```
```

```
