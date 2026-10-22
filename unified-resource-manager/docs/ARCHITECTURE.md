# Unified Resource Manager 架构设计

## 概述

Unified Resource Manager 是一个统一的异构 AI 资源管理平台，在保持原有 GPU-Virtual-Service 和 Ascend-Virtual-Service 代码不变的基础上，提供统一的资源管理能力。

## 架构原则

1. **非侵入性**: 不修改原有 GPU-Virtual-Service 和 Ascend-Virtual-Service 的代码
2. **适配器模式**: 通过适配器层与底层服务交互
3. **统一抽象**: 提供统一的资源抽象和管理接口
4. **可扩展性**: 支持未来添加新的资源类型

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Unified Resource Manager                  │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  REST API    │  │  gRPC API    │  │   Web UI     │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Core Services Layer                      │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐         │   │
│  │  │ Resource │  │Scheduler │  │  Cluster │         │   │
│  │  │   Pool   │  │          │  │  Manager │         │   │
│  │  └──────────┘  └──────────┘  └──────────┘         │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            Adapter Layer                               │   │
│  │  ┌──────────┐                    ┌──────────┐      │   │
│  │  │   GPU    │                    │   NPU     │      │   │
│  │  │ Adapter  │                    │ Adapter   │      │   │
│  │  └──────────┘                    └──────────┘      │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │         Observability Layer                            │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐         │   │
│  │  │ Metrics  │  │   Logs   │  │  Alerts   │         │   │
│  │  └──────────┘  └──────────┘  └──────────┘         │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
         │                              │
         ▼                              ▼
┌──────────────────┐          ┌──────────────────┐
│ GPU-Virtual-     │          │ Ascend-Virtual-  │
│ Service          │          │ Service           │
│ (Unchanged)      │          │ (Unchanged)       │
└──────────────────┘          └──────────────────┘
```

## 核心组件

### 1. Resource Pool Manager (资源池管理器)

**职责**:
- 管理多个资源池（GPU 池、NPU 池）
- 提供统一的资源抽象
- 资源发现和注册
- 资源状态管理

**关键接口**:
- `GetPoolInfo()`: 获取资源池信息
- `ListResources()`: 列出资源
- `AllocateResources()`: 分配资源
- `ReleaseResources()`: 释放资源

### 2. Scheduler (调度器)

**职责**:
- 任务调度和资源分配
- 支持多种调度策略（Binpack、Spread）
- 优先级调度和抢占
- 任务生命周期管理

**调度策略**:
- **Binpack**: 尽量将任务调度到资源最少的节点，提高资源利用率
- **Spread**: 将任务分散到不同节点，提高可用性

### 3. Cluster Manager (集群管理器)

**职责**:
- 节点注册和发现
- 节点健康检查
- 资源发现
- 集群统计

### 4. Adapter Layer (适配器层)

**职责**:
- 适配底层 GPU-Virtual-Service
- 适配底层 Ascend-Virtual-Service
- 统一资源抽象
- 协议转换

**GPU Adapter**:
- 调用 GPU device plugin API
- 调用 GPU runtime service
- 查询 GPU exporter metrics

**NPU Adapter**:
- 调用 flexnpud HTTP API
- 注册/注销 NPU 设备
- 查询 NPU 状态

### 5. Observability (可观测性)

**Metrics**:
- Prometheus 指标
- 资源使用率
- 任务执行统计
- 分配成功率

**Logs**:
- 结构化日志
- 操作审计日志
- 错误日志

**Alerts**:
- 资源异常告警
- 任务失败告警
- 节点离线告警

## 数据流

### 资源分配流程

```
1. 用户提交任务
   ↓
2. Scheduler 接收任务，加入待调度队列
   ↓
3. Scheduler 从 Resource Pool 请求资源
   ↓
4. Resource Pool 通过 Adapter 调用底层服务
   ↓
5. GPU/NPU Adapter 调用对应的底层 API
   ↓
6. 底层服务分配资源并返回
   ↓
7. Resource Pool 更新资源状态
   ↓
8. Scheduler 更新任务状态为 Running
```

### 资源释放流程

```
1. 任务完成或取消
   ↓
2. Scheduler 调用 Resource Pool 释放资源
   ↓
3. Resource Pool 通过 Adapter 调用底层服务
   ↓
4. GPU/NPU Adapter 调用对应的释放 API
   ↓
5. 底层服务释放资源
   ↓
6. Resource Pool 更新资源状态为 Available
```

## 扩展性设计

### 添加新的资源类型

1. 实现 `ResourcePool` 接口
2. 创建对应的 Adapter
3. 注册到 UnifiedResourcePool
4. 更新调度器支持新资源类型

### 添加新的调度策略

1. 实现 `SchedulingStrategy` 接口
2. 在 Scheduler 中注册新策略
3. 通过配置选择策略

## 部署架构

### Kubernetes 部署

```
┌─────────────────────────────────────────┐
│     Unified Resource Manager Pod        │
│  ┌──────────────────────────────────┐   │
│  │  API Server                      │   │
│  │  Resource Pool Manager           │   │
│  │  Scheduler                       │   │
│  │  Cluster Manager                 │   │
│  └──────────────────────────────────┘   │
└─────────────────────────────────────────┘
         │                    │
         ▼                    ▼
┌─────────────────┐  ┌─────────────────┐
│ GPU Device      │  │ NPU Hypervisor  │
│ Plugin          │  │ (flexnpud)      │
└─────────────────┘  └─────────────────┘
```

## 安全考虑

1. **认证授权**: 支持多租户和 RBAC
2. **资源隔离**: 租户级别的资源隔离
3. **审计日志**: 完整的操作审计
4. **网络安全**: TLS 加密通信

## 性能优化

1. **缓存**: 资源状态缓存
2. **批量操作**: 批量资源分配
3. **异步处理**: 异步任务调度
4. **连接池**: 底层服务连接池

## 监控指标

- 资源池利用率
- 任务调度延迟
- 资源分配成功率
- 节点健康状态
- API 请求延迟
