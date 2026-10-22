# Unified Resource Manager 实现说明

## 实现概述

本系统在保持原有 GPU-Virtual-Service 和 Ascend-Virtual-Service 代码完全不变的前提下，实现了一个统一的异构资源管理系统，提供商业化级别的 AI 资源管理能力。

## 核心实现

### 1. 资源池管理 (`core/pool/`)

- **UnifiedResourcePool**: 统一资源池管理器，支持多个资源池的注册和管理
- **ResourcePool 接口**: 统一的资源池抽象接口
- **GPUResourcePool**: GPU 资源池实现（通过 GPU Adapter）
- **NPUResourcePool**: NPU 资源池实现（通过 NPU Adapter）

**关键特性**:
- 统一的资源抽象（Resource）
- 资源状态管理（Available, Allocated, Maintenance, Error）
- 资源分配和释放
- 资源指标收集

### 2. 适配器层 (`adapters/`)

#### GPU Adapter (`adapters/gpu/`)
- 适配 GPU-Virtual-Service 的以下组件:
  - GPU Device Plugin (通过 Unix socket)
  - GPU Runtime Service (通过 gRPC)
  - GPU Exporter (通过 HTTP/Prometheus)
- 实现资源发现、分配、释放和监控

#### NPU Adapter (`adapters/npu/`)
- 适配 Ascend-Virtual-Service 的以下组件:
  - flexnpud (hypervisor, 通过 HTTP API)
  - aclserver (通过 HTTP)
- 实现 NPU 设备注册、算力分配和监控

**设计原则**:
- 完全非侵入：不修改底层服务代码
- 协议转换：将底层服务的协议转换为统一接口
- 状态同步：维护资源状态的一致性

### 3. 智能调度器 (`core/scheduler/`)

- **IntelligentScheduler**: 智能调度器实现
- **SchedulingStrategy**: 调度策略接口
- **BinpackStrategy**: Binpack 调度策略（提高资源利用率）
- **SpreadStrategy**: Spread 调度策略（提高可用性）

**功能特性**:
- 任务队列管理
- 优先级调度
- 资源抢占
- 任务生命周期管理

### 4. 集群管理 (`core/cluster/`)

- **ClusterManager**: 集群管理器
- 节点注册和发现
- 节点健康检查
- 资源发现
- 集群统计

### 5. REST API (`api/rest/`)

完整的 RESTful API 实现，包括:
- 资源池管理 API
- 资源分配 API
- 任务管理 API
- 健康检查 API

### 6. 可观测性 (`observability/`)

- **MetricsCollector**: Prometheus 指标收集器
- 资源池指标
- 资源使用指标
- 任务执行指标
- 分配统计指标

### 7. 认证授权 (`auth/`)

- **RBAC**: 基于角色的访问控制
- **TenantManager**: 多租户管理
- 配额管理
- 使用量统计

## 架构特点

### 1. 非侵入性设计

- 通过适配器模式与底层服务交互
- 不修改 GPU-Virtual-Service 和 Ascend-Virtual-Service 的任何代码
- 通过标准接口（Unix socket、HTTP、gRPC）与底层服务通信

### 2. 统一抽象

- 统一的资源抽象（Resource）
- 统一的资源池接口（ResourcePool）
- 统一的调度接口（Scheduler）

### 3. 可扩展性

- 易于添加新的资源类型（只需实现 ResourcePool 接口和对应 Adapter）
- 易于添加新的调度策略（实现 SchedulingStrategy 接口）
- 插件化的架构设计

### 4. 商业化特性

- 完整的 REST API
- 多租户支持
- RBAC 权限控制
- 配额管理
- 审计日志（可扩展）
- 计费统计（可扩展）

## 与底层服务的集成

### GPU-Virtual-Service 集成

```
Unified Resource Manager
    ↓ (通过适配器)
GPU Adapter
    ↓
├── GPU Device Plugin (Unix socket)
├── GPU Runtime Service (gRPC)
└── GPU Exporter (HTTP/Prometheus)
```

### Ascend-Virtual-Service 集成

```
Unified Resource Manager
    ↓ (通过适配器)
NPU Adapter
    ↓
├── flexnpud (HTTP API: /device/register)
└── aclserver (HTTP)
```

## 部署架构

```
Kubernetes Cluster
├── Unified Resource Manager Pod
│   ├── API Server
│   ├── Resource Pool Manager
│   ├── Scheduler
│   └── Cluster Manager
├── GPU-Virtual-Service Components
│   ├── GPU Device Plugin
│   ├── GPU Runtime Service
│   └── GPU Exporter
└── Ascend-Virtual-Service Components
    ├── flexnpud
    └── aclserver
```

## 使用流程示例

### 1. 提交推理任务

```bash
# 1. 用户通过 API 提交任务
POST /api/v1/tasks
{
  "name": "llm-inference",
  "type": "inference",
  "resource_req": {
    "resource_type": "gpu",
    "count": 1,
    "memory_gb": 24,
    "cores": 50
  }
}

# 2. Scheduler 接收任务，加入待调度队列

# 3. Scheduler 从 Resource Pool 请求资源

# 4. Resource Pool 通过 GPU Adapter 调用 GPU Device Plugin

# 5. GPU Device Plugin 分配 vGPU 资源

# 6. 任务状态更新为 Running
```

### 2. 资源监控

```bash
# 查询资源池状态
GET /api/v1/pools/gpu-pool-1

# 查询资源指标
GET /api/v1/pools/gpu-pool-1/resources/node1-gpu0/metrics?duration=1h

# Prometheus 指标
GET /metrics
```

## 未来扩展

### 1. Web 管理界面

- 基于 React/Vue 的前端界面
- 资源池可视化
- 任务监控仪表板
- 集群状态展示

### 2. 高级调度策略

- 基于机器学习的调度优化
- 成本感知调度
- 负载均衡调度

### 3. 更多资源类型

- 支持其他 XPU 类型（如 TPU、DPU 等）
- 统一的资源抽象使得添加新类型变得简单

### 4. 高级功能

- 资源预留
- 资源抢占策略优化
- 自动扩缩容
- 故障自愈

## 代码结构

```
unified-resource-manager/
├── api/                    # API 层
│   └── rest/              # REST API 实现
├── core/                   # 核心服务
│   ├── pool/              # 资源池管理
│   ├── scheduler/         # 调度器
│   └── cluster/           # 集群管理
├── adapters/               # 适配器层
│   ├── gpu/               # GPU 适配器
│   └── npu/               # NPU 适配器
├── observability/          # 可观测性
│   └── metrics/           # 指标收集
├── auth/                   # 认证授权
│   ├── rbac/              # 权限控制
│   └── tenant/            # 多租户
├── cmd/                    # 主程序入口
├── deploy/                 # 部署配置
│   └── k8s/               # Kubernetes 配置
└── docs/                   # 文档
```

## 总结

本实现完全遵循了"保持原代码不变"的要求，通过适配器模式和统一抽象，实现了对 GPU 和 NPU 异构资源的统一管理。系统具备商业化软件的所有关键特性，包括完整的 API、多租户支持、权限控制、监控告警等，可以作为一个独立的商业化产品使用。
