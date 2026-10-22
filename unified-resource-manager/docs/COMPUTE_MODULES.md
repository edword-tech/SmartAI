# 算力管理模块文档

根据架构图实现的算力感知、算力质量、算力编排和训练加速模块，每个模块都可以独立管理和实现。

## 📋 模块概览

### 1. 算力感知 (Compute Perception)

**功能**：
- ✅ 算力资源绑定 (Compute Resource Binding)
- ✅ 算力资源管理 (Compute Resource Management)
- ✅ 算力资源监控 (Compute Resource Monitoring)
- ✅ 算力集群设置 (Compute Cluster Settings)

**文件**：`core/compute/perception.go`

**API**：
- `GET /api/v1/compute/perception/states` - 列出所有资源状态
- `GET /api/v1/compute/perception/states/{resourceId}` - 获取资源状态
- `POST /api/v1/compute/perception/bind` - 绑定资源到任务
- `POST /api/v1/compute/perception/unbind` - 解绑资源
- `GET /api/v1/compute/perception/cluster-settings` - 获取集群设置
- `PUT /api/v1/compute/perception/cluster-settings` - 更新集群设置

**特性**：
- 实时监控资源状态（利用率、温度、功耗）
- 资源健康状态评估（healthy/warning/critical）
- 资源绑定管理
- 集群配置管理

---

### 2. 算力质量 (Compute Quality)

**功能**：
- ✅ 算力切分设置 (Compute Slicing Settings)
- ✅ 算力独占设置 (Compute Exclusive Use Settings)
- ✅ 算力资源分配 (Compute Resource Allocation)
- ✅ 基础算力池进出 (Basic Compute Pool Ingress/Egress)

**文件**：`core/compute/quality.go`

**API**：
- `GET /api/v1/compute/quality/slicing-configs` - 列出切分配置
- `POST /api/v1/compute/quality/slicing-configs` - 创建切分配置
- `GET /api/v1/compute/quality/exclusive-configs` - 列出独占配置
- `POST /api/v1/compute/quality/exclusive-configs` - 创建独占配置
- `GET /api/v1/compute/quality/allocation-strategies` - 列出分配策略
- `POST /api/v1/compute/quality/allocation-strategies` - 创建分配策略

**特性**：
- 资源切分配置（vGPU/vNPU 切分）
- 独占资源配置（高优先级任务独占）
- 质量感知的资源分配
- 算力池进出记录

---

### 3. 算力编排 (Compute Orchestration)

**功能**：
- ✅ 训练/推理任务下发 (Training/Inference Task Distribution)
- ✅ 算力均衡调度 (Compute Balanced Scheduling)
- ✅ 算力最小碎片调度 (Compute Minimum Fragment Scheduling)
- ✅ 弹性调度优化 (Elastic Scheduling Optimization)

**文件**：`core/compute/orchestration.go`

**API**：
- `POST /api/v1/compute/orchestration/distribute` - 下发任务（支持多种策略）
- `POST /api/v1/compute/orchestration/optimize` - 优化调度
- `GET /api/v1/compute/orchestration/strategies` - 获取调度策略配置

**调度策略**：
- **balanced** - 均衡调度：将任务分配到负载最低的节点
- **min_fragment** - 最小碎片调度：优先使用碎片小的资源
- **elastic** - 弹性调度：根据利用率自动扩缩容

**特性**：
- 多种调度策略可选
- 负载均衡
- 资源碎片优化
- 弹性扩缩容

---

### 4. 训练加速 (Training Acceleration)

**功能**：
- ✅ 训练数据加载优化 (Training Data Loading Optimization)
- ✅ 分布式训练 (Distributed Training)
- ✅ 断点续训/故障预测 (Checkpoint/Restart Training / Fault Prediction)
- ✅ 显存细粒度使用 (Fine-grained Memory Usage)

**文件**：`core/compute/acceleration.go`

**API**：
- `POST /api/v1/compute/acceleration/data-loading` - 配置数据加载优化
- `POST /api/v1/compute/acceleration/distributed` - 配置分布式训练
- `POST /api/v1/compute/acceleration/checkpoint` - 配置检查点
- `POST /api/v1/compute/acceleration/memory` - 配置显存优化
- `POST /api/v1/compute/acceleration/optimize/{taskId}` - 优化任务（应用所有配置）

**优化配置**：
- **数据加载优化**：预取大小、工作线程数、固定内存、异步加载、缓存策略
- **分布式训练**：DDP/FSDP/DeepSpeed、节点数、GPU数、通信后端、混合精度
- **检查点**：检查点间隔、最大检查点数、自动恢复、故障预测
- **显存优化**：内存高效注意力、梯度累积、激活检查点、卸载策略

**特性**：
- 数据加载性能优化
- 分布式训练支持
- 断点续训和故障恢复
- 显存使用优化

---

## 🏗️ 架构设计

### 模块独立性

每个模块都是独立的，可以单独使用和管理：

```
┌─────────────────────────────────────────┐
│         Unified Resource Manager        │
│                                         │
│  ┌──────────────┐  ┌──────────────┐    │
│  │  算力感知    │  │  算力质量    │    │
│  │ (独立模块)   │  │ (独立模块)   │    │
│  └──────────────┘  └──────────────┘    │
│  ┌──────────────┐  ┌──────────────┐    │
│  │  算力编排    │  │  训练加速    │    │
│  │ (独立模块)   │  │ (独立模块)   │    │
│  └──────────────┘  └──────────────┘    │
└─────────────────────────────────────────┘
         │              │
         ▼              ▼
┌──────────────┐  ┌──────────────┐
│ ResourcePool │  │  Scheduler   │
└──────────────┘  └──────────────┘
```

### 模块接口

每个模块都提供清晰的接口，可以独立测试和替换：

- **ComputePerception**：资源状态查询、绑定管理、集群设置
- **ComputeQuality**：切分配置、独占配置、分配策略
- **ComputeOrchestration**：任务分发、调度优化
- **TrainingAcceleration**：加速配置、任务优化

---

## 🚀 使用示例

### 1. 算力感知

**获取资源状态**：
```bash
curl http://localhost:8080/api/v1/compute/perception/states
```

**绑定资源**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/perception/bind \
  -H "Content-Type: application/json" \
  -d '{
    "resource_id": "gpu-001",
    "task_id": "task-123"
  }'
```

**更新集群设置**：
```bash
curl -X PUT http://localhost:8080/api/v1/compute/perception/cluster-settings \
  -H "Content-Type: application/json" \
  -d '{
    "cluster_id": "default-cluster",
    "name": "Production Cluster",
    "auto_discovery": true,
    "health_check_interval": "10s"
  }'
```

### 2. 算力质量

**创建切分配置**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/quality/slicing-configs \
  -H "Content-Type: application/json" \
  -d '{
    "id": "slice-config-1",
    "name": "GPU 20GB 切分",
    "resource_type": "gpu",
    "slice_size": {
      "memory_gb": 20,
      "cores": 50
    },
    "max_slices": 2,
    "isolation": true
  }'
```

**创建独占配置**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/quality/exclusive-configs \
  -H "Content-Type: application/json" \
  -d '{
    "id": "exclusive-config-1",
    "name": "高优先级独占",
    "resource_type": "gpu",
    "priority": 8,
    "max_duration": "24h"
  }'
```

### 3. 算力编排

**均衡调度任务**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/orchestration/distribute \
  -H "Content-Type: application/json" \
  -d '{
    "task": {
      "name": "training-task-1",
      "type": "training",
      "priority": 5,
      "resource_req": {
        "task_id": "task-001",
        "task_type": "training",
        "resource_type": "gpu",
        "count": 1,
        "memory_gb": 40,
        "cores": 100,
        "priority": 5,
        "timeout": "3600s"
      }
    },
    "strategy": "balanced"
  }'
```

**最小碎片调度**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/orchestration/distribute \
  -H "Content-Type: application/json" \
  -d '{
    "task": {...},
    "strategy": "min_fragment"
  }'
```

### 4. 训练加速

**配置数据加载优化**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/acceleration/data-loading \
  -H "Content-Type: application/json" \
  -d '{
    "id": "data-loading-1",
    "task_id": "task-123",
    "prefetch_size": 4,
    "num_workers": 8,
    "pin_memory": true,
    "async_loading": true,
    "cache_strategy": "memory"
  }'
```

**配置分布式训练**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/acceleration/distributed \
  -H "Content-Type: application/json" \
  -d '{
    "id": "distributed-1",
    "task_id": "task-123",
    "strategy": "ddp",
    "num_nodes": 4,
    "num_gpus_per_node": 8,
    "communication_backend": "nccl",
    "mixed_precision": true,
    "gradient_checkpointing": true
  }'
```

**配置检查点**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/acceleration/checkpoint \
  -H "Content-Type: application/json" \
  -d '{
    "id": "checkpoint-1",
    "task_id": "task-123",
    "interval": "1h",
    "max_checkpoints": 5,
    "auto_resume": true,
    "fault_prediction": true,
    "storage_path": "/checkpoints"
  }'
```

**优化任务（应用所有配置）**：
```bash
curl -X POST http://localhost:8080/api/v1/compute/acceleration/optimize/task-123
```

---

## 🔧 初始化配置

在 `cmd/main.go` 中初始化所有模块：

```go
// 创建算力管理模块（独立管理）
computePerception := compute.NewComputePerception(clusterMgr, resourcePool)
computeQuality := compute.NewComputeQuality(resourcePool)
computeOrchestration := compute.NewComputeOrchestration(sched, resourcePool)
trainingAcceleration := compute.NewTrainingAcceleration(sched, resourcePool)

// 注入到 API 服务器
apiServer.WithComputeModules(computePerception, computeQuality, computeOrchestration, trainingAcceleration)

// 启动算力感知监控
computePerception.Start()
```

---

## 📊 模块关系

```
任务提交
    │
    ▼
算力编排 (选择调度策略)
    │
    ├──► 均衡调度
    ├──► 最小碎片调度
    └──► 弹性调度
    │
    ▼
算力质量 (资源分配)
    │
    ├──► 切分配置
    ├──► 独占配置
    └──► 分配策略
    │
    ▼
算力感知 (资源绑定和监控)
    │
    ├──► 资源绑定
    ├──► 状态监控
    └──► 集群设置
    │
    ▼
训练加速 (性能优化)
    │
    ├──► 数据加载优化
    ├──► 分布式训练
    ├──► 检查点配置
    └──► 显存优化
```

---

## ✅ 独立管理能力

每个模块都可以：

1. **独立初始化**：不依赖其他模块即可创建
2. **独立配置**：有自己的配置接口
3. **独立API**：提供独立的 REST API
4. **独立测试**：可以单独测试
5. **独立替换**：可以替换实现而不影响其他模块

---

## 🎯 完整工作流程

### 场景：训练大模型

1. **配置训练加速**
   ```
   配置数据加载优化、分布式训练、检查点、显存优化
   ```

2. **提交训练任务（使用算力编排）**
   ```
   使用均衡调度策略，将任务分配到负载最低的节点
   ```

3. **算力质量分配资源**
   ```
   根据切分配置和独占配置，分配合适的资源
   ```

4. **算力感知绑定和监控**
   ```
   绑定资源到任务，实时监控资源状态
   ```

5. **应用训练加速优化**
   ```
   自动应用所有加速配置，优化训练性能
   ```

---

所有模块已实现并可以独立管理和使用！🚀
