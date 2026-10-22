# Unified Resource Manager API 文档

## 概述

Unified Resource Manager 提供完整的 RESTful API 用于管理异构 AI 资源（GPU 和 NPU）。

## 基础信息

- **Base URL**: `http://api.example.com/api/v1`
- **Content-Type**: `application/json`
- **认证**: 当前版本暂不支持认证（后续版本将支持）

## API 端点

### 资源池管理

#### 列出所有资源池

```http
GET /api/v1/pools
```

**响应示例**:
```json
[
  {
    "id": "gpu-pool-1",
    "name": "GPU Pool 1",
    "type": "gpu",
    "total_count": 100,
    "available_count": 50,
    "allocated_count": 50,
    "total_memory": 10240,
    "used_memory": 5120,
    "nodes": ["node1", "node2"],
    "created_at": "2025-01-23T10:00:00Z",
    "updated_at": "2025-01-23T10:00:00Z"
  }
]
```

#### 创建资源池

```http
POST /api/v1/pools
Content-Type: application/json

{
  "name": "GPU Pool 1",
  "type": "gpu",
  "nodes": ["node1", "node2"],
  "quota": {
    "max_count": 100,
    "max_memory_gb": 10240
  },
  "labels": {
    "environment": "production"
  }
}
```

#### 获取资源池信息

```http
GET /api/v1/pools/{poolId}
```

#### 列出资源池中的资源

```http
GET /api/v1/pools/{poolId}/resources?state=available&node_id=node1
```

**查询参数**:
- `state`: 资源状态 (available, allocated, maintenance, error)
- `node_id`: 节点 ID
- `label_key`: 标签键值对

#### 获取资源信息

```http
GET /api/v1/pools/{poolId}/resources/{resourceId}
```

#### 获取资源指标

```http
GET /api/v1/pools/{poolId}/resources/{resourceId}/metrics?duration=1h
```

**查询参数**:
- `duration`: 时间范围 (例如: 1h, 30m, 24h)

### 资源分配

#### 分配资源

```http
POST /api/v1/pools/{poolId}/allocate
Content-Type: application/json

{
  "task_id": "task-123",
  "task_type": "inference",
  "resource_type": "gpu",
  "count": 1,
  "memory_gb": 24,
  "cores": 50,
  "priority": 8,
  "labels": {
    "model": "deepseek-r1"
  },
  "timeout": "3600s"
}
```

**响应示例**:
```json
{
  "allocation_id": "alloc-123",
  "resources": [
    {
      "id": "node1-gpu0",
      "type": "gpu",
      "state": "allocated",
      "node_id": "node1",
      "device_id": "gpu0",
      "total_memory": 48,
      "used_memory": 24,
      "total_cores": 100,
      "used_cores": 50
    }
  ],
  "expires_at": "2025-01-23T11:00:00Z"
}
```

#### 释放资源

```http
POST /api/v1/pools/{poolId}/release/{allocationId}
```

### 任务管理

#### 提交任务

```http
POST /api/v1/tasks
Content-Type: application/json

{
  "name": "llm-inference-1",
  "type": "inference",
  "priority": 8,
  "resource_req": {
    "task_id": "task-123",
    "task_type": "inference",
    "resource_type": "gpu",
    "count": 1,
    "memory_gb": 24,
    "cores": 50,
    "priority": 8,
    "timeout": "3600s"
  },
  "labels": {
    "model": "deepseek-r1",
    "tenant": "tenant-1"
  }
}
```

**响应示例**:
```json
{
  "id": "task-123",
  "name": "llm-inference-1",
  "type": "inference",
  "priority": 8,
  "status": "pending",
  "created_at": "2025-01-23T10:00:00Z"
}
```

#### 列出任务

```http
GET /api/v1/tasks?status=running&type=inference
```

**查询参数**:
- `status`: 任务状态 (pending, scheduled, running, completed, failed, cancelled)
- `type`: 任务类型 (inference, training)
- `priority`: 优先级

#### 获取任务信息

```http
GET /api/v1/tasks/{taskId}
```

#### 取消任务

```http
DELETE /api/v1/tasks/{taskId}
```

### 健康检查

#### 健康检查

```http
GET /health
```

**响应**:
```json
{
  "status": "healthy"
}
```

#### 就绪检查

```http
GET /ready
```

## 错误响应

所有错误响应遵循以下格式:

```json
{
  "error": "错误描述信息"
}
```

**HTTP 状态码**:
- `200 OK`: 请求成功
- `201 Created`: 资源创建成功
- `400 Bad Request`: 请求参数错误
- `404 Not Found`: 资源不存在
- `500 Internal Server Error`: 服务器内部错误

## 使用示例

### 提交推理任务

```bash
curl -X POST http://api.example.com/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "llm-inference-1",
    "type": "inference",
    "priority": 8,
    "resource_req": {
      "task_id": "task-123",
      "task_type": "inference",
      "resource_type": "gpu",
      "count": 1,
      "memory_gb": 24,
      "cores": 50,
      "priority": 8,
      "timeout": "3600s"
    }
  }'
```

### 查询资源池状态

```bash
curl http://api.example.com/api/v1/pools/gpu-pool-1
```

### 获取资源指标

```bash
curl "http://api.example.com/api/v1/pools/gpu-pool-1/resources/node1-gpu0/metrics?duration=1h"
```
