# 文档目录结构

## 1. 概述

本文档描述了智能合约虚拟机项目的设计文档目录结构，方便开发人员快速找到所需的文档。

## 2. 目录结构

```text
docs/
├── 00_directory_structure.md
├── architecture.md
├── modular_architecture_design.md
├── default_library.md
├── host_runtime.md              # Host Runtime MVP 设计
├── sandbox_roadmap.md           # 可插拔 Runner / 沙箱路线
├── execution_environment.md
├── gas_metering.md
├── security_review.md
├── abi_generation.md
└── detailed_design/
    ├── detailed_design_overview.md
    ├── vm_engine_detailed_design.md
    ├── security_review_detailed_design.md
    ├── compiler_detailed_design.md
    ├── execution_environment_detailed_design.md
    ├── gas_metering_detailed_design.md
    ├── abi_generator_detailed_design.md
    ├── contract_manager_detailed_design.md
    └── contract_processing_flow.md
```

代码侧当前为扁平结构（无 `internal`）：

```text
vm/
├── engine.go / compiler.go / runner.go / security.go / gas.go / contract.go / host.go
├── abi/
├── examples/{basic,gas,complete,advanced,event}/
└── docs/
```

## 3. 文档分类

### 3.1 架构与路线
- [architecture.md](architecture.md)
- [modular_architecture_design.md](modular_architecture_design.md)
- [host_runtime.md](host_runtime.md)
- [sandbox_roadmap.md](sandbox_roadmap.md)

### 3.2 详细设计
- [detailed_design_overview.md](detailed_design/detailed_design_overview.md)
- [vm_engine_detailed_design.md](detailed_design/vm_engine_detailed_design.md)
- [compiler_detailed_design.md](detailed_design/compiler_detailed_design.md)
- [execution_environment_detailed_design.md](detailed_design/execution_environment_detailed_design.md)
- [contract_manager_detailed_design.md](detailed_design/contract_manager_detailed_design.md)
- [gas_metering_detailed_design.md](detailed_design/gas_metering_detailed_design.md)
- [contract_processing_flow.md](detailed_design/contract_processing_flow.md)

### 3.3 规范
- [default_library.md](default_library.md)
- [execution_environment.md](execution_environment.md)
- [gas_metering.md](gas_metering.md)
- [security_review.md](security_review.md)
- [abi_generation.md](abi_generation.md)

## 4. 阅读建议

### 4.1 新手
1. architecture.md
2. contract_processing_flow.md
3. detailed_design_overview.md

### 4.2 下一阶段开发
1. host_runtime.md / default_library.md
2. gas_metering.md（EstimateGas）
3. sandbox_roadmap.md

## 5. 维护原则
- 文档与代码同步
- 显式标注 MVP / 后置 / 未实现
- 不以过时目标态覆盖当前真相源
