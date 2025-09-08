# 文档目录结构

## 1. 概述

本文档描述了智能合约虚拟机项目的设计文档目录结构，方便开发人员快速找到所需的文档。

## 2. 目录结构

```
docs/
├── 00_directory_structure.md    # 文档目录结构说明
├── architecture.md              # 系统架构设计文档
├── modular_architecture_design.md # 模块化架构设计文档
├── default_library.md           # 默认库接口规范
├── execution_environment.md     # 执行环境设计
├── gas_metering.md              # Gas计费与资源控制设计
├── security_review.md           # 安全审查规范
├── abi_generation.md            # ABI生成与关键字处理设计
└── detailed_design/             # 详细设计文档目录
    ├── detailed_design_overview.md        # 详细设计文档总览
    ├── vm_engine_detailed_design.md       # 虚拟机执行引擎详细设计
    ├── security_review_detailed_design.md # 安全审查系统详细设计
    ├── compiler_detailed_design.md        # 编译器模块详细设计
    ├── execution_environment_detailed_design.md # 执行环境详细设计
    ├── gas_metering_detailed_design.md    # Gas计费系统详细设计
    ├── abi_generator_detailed_design.md   # ABI生成器详细设计
    ├── storage_manager_detailed_design.md # 存储管理模块详细设计
    ├── contract_manager_detailed_design.md # 合约管理模块详细设计
    └── contract_processing_flow.md        # 合约处理流程
```

## 3. 文档分类

### 3.1 架构设计文档
- [architecture.md](architecture.md) - 系统整体架构设计
- [modular_architecture_design.md](modular_architecture_design.md) - 模块化架构设计

### 3.2 详细设计文档
- [detailed_design_overview.md](detailed_design/detailed_design_overview.md) - 详细设计总览
- [vm_engine_detailed_design.md](detailed_design/vm_engine_detailed_design.md) - 虚拟机执行引擎详细设计
- [security_review_detailed_design.md](detailed_design/security_review_detailed_design.md) - 安全审查系统详细设计
- [compiler_detailed_design.md](detailed_design/compiler_detailed_design.md) - 编译器模块详细设计
- [execution_environment_detailed_design.md](detailed_design/execution_environment_detailed_design.md) - 执行环境详细设计
- [gas_metering_detailed_design.md](detailed_design/gas_metering_detailed_design.md) - Gas计费系统详细设计
- [abi_generator_detailed_design.md](detailed_design/abi_generator_detailed_design.md) - ABI生成器详细设计
- [storage_manager_detailed_design.md](detailed_design/storage_manager_detailed_design.md) - 存储管理模块详细设计
- [contract_manager_detailed_design.md](detailed_design/contract_manager_detailed_design.md) - 合约管理模块详细设计

### 3.3 规范文档
- [default_library.md](default_library.md) - 默认库接口规范
- [execution_environment.md](execution_environment.md) - 执行环境设计
- [gas_metering.md](gas_metering.md) - Gas计费与资源控制设计
- [security_review.md](security_review.md) - 安全审查规范
- [abi_generation.md](abi_generation.md) - ABI生成与关键字处理设计

### 3.4 流程文档
- [contract_processing_flow.md](detailed_design/contract_processing_flow.md) - 合约处理流程

## 4. 文档阅读建议

### 4.1 新手入门
1. [architecture.md](architecture.md) - 了解系统整体架构
2. [modular_architecture_design.md](modular_architecture_design.md) - 了解模块化架构设计
3. [detailed_design_overview.md](detailed_design/detailed_design_overview.md) - 了解详细设计概览
4. [default_library.md](default_library.md) - 了解默认库接口

### 4.2 开发人员
1. 相关模块的详细设计文档
2. 对应的规范文档
3. [contract_processing_flow.md](detailed_design/contract_processing_flow.md) - 了解合约处理流程

### 4.3 架构师
1. [architecture.md](architecture.md) - 系统架构
2. [modular_architecture_design.md](modular_architecture_design.md) - 模块化架构设计
3. [detailed_design_overview.md](detailed_design/detailed_design_overview.md) - 详细设计总览
4. 各模块详细设计文档

## 5. 文档维护

### 5.1 更新原则
- 文档应与代码保持同步
- 重要变更应及时更新相关文档
- 新增功能应配套相应的设计文档

### 5.2 命名规范
- 使用小写字母和下划线命名
- 文档名应清晰表达内容主题

### 5.3 版本控制
- 所有文档都应纳入版本控制
- 重要修订应添加修订记录
- 废弃文档应标记并归档