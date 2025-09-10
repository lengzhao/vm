# 智能合约虚拟机 (vm)

这是一个基于Golang的智能合约虚拟机。智能合约是Golang源代码，而不是二进制程序，并且对一些关键字和导入库进行了限制以增强安全性。

## 特性

- **源码即合约**：智能合约本身就是Golang代码
- **安全优先**：通过关键字白名单和导入白名单确保执行安全性
- **开发友好**：降低智能合约开发门槛，让熟悉Go语言的开发者无缝接入
- **并行执行**：通过对象隔离机制支持交易的并行执行
- **Gas计费**：防止合约执行消耗过多系统资源

## 文档

### 架构设计
- [架构设计](docs/architecture.md)
- [模块化架构设计](docs/modular_architecture_design.md)
- [架构缺陷分析](docs/architecture_deficiencies.md)
- [目录结构说明](docs/00_directory_structure.md)

### 核心模块详细设计
- [详细设计总览](docs/detailed_design/detailed_design_overview.md)
- [虚拟机执行引擎](docs/detailed_design/vm_engine_detailed_design.md)
- [安全审查系统](docs/detailed_design/security_review_detailed_design.md)
- [编译器模块](docs/detailed_design/compiler_detailed_design.md)
- [执行环境](docs/detailed_design/execution_environment_detailed_design.md)
- [Gas计费系统](docs/detailed_design/gas_metering_detailed_design.md)
- [ABI生成器](docs/detailed_design/abi_generator_detailed_design.md)
- [合约管理模块](docs/detailed_design/contract_manager_detailed_design.md)

### 流程文档
- [合约处理流程](docs/detailed_design/contract_processing_flow.md)

### 规范文档
- [安全审查规范](docs/security_review.md)
- [默认库接口规范](docs/default_library.md)
- [执行环境设计](docs/execution_environment.md)
- [Gas计费机制](docs/gas_metering.md)
- [ABI生成与关键字处理](docs/abi_generation.md)