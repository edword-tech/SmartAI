# Unified Resource Manager 快速开始指南

## 5 分钟快速体验

### 前置条件

1. Kubernetes 集群已部署
2. GPU-Virtual-Service 已部署（如使用 GPU）
3. Ascend-Virtual-Service 已部署（如使用 NPU）

### 步骤 1: 部署 Unified Resource Manager

```bash
# 创建命名空间
kubectl create namespace flexai-system

# 部署服务
cd unified-resource-manager/deploy/k8s
kubectl apply -f deployment.yaml

# 等待 Pod 就绪
kubectl wait --for=condition=ready pod -l app=unified-resource-manager -n flexai-system --timeout=60s
```

### 步骤 2: 访问 API

```bash
# 端口转发
kubectl port-forward -n flexai-system svc/unified-resource-manager 8080:80

# 测试健康检查
curl http://localhost:8080/health
```

### 步骤 3: 查看资源池

```bash
# 列出所有资源池
curl http://localhost:8080/api/v1/pools | jq

# 获取资源池详细信息
curl http://localhost:8080/api/v1/pools/gpu-pool-1 | jq
```

### 步骤 4: 提交一个推理任务

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-inference",
    "type": "inference",
    "priority": 5,
    "resource_req": {
      "task_id": "task-001",
      "task_type": "inference",
      "resource_type": "gpu",
      "count": 1,
      "memory_gb": 24,
      "cores": 50,
      "priority": 5,
      "timeout": "3600s"
    }
  }' | jq
```

### 步骤 5: 查看任务状态

```bash
# 列出所有任务
curl http://localhost:8080/api/v1/tasks | jq

# 获取任务详细信息
curl http://localhost:8080/api/v1/tasks/task-001 | jq
```

### 步骤 6: 查看资源指标

```bash
# 获取资源池指标
curl http://localhost:8080/api/v1/pools/gpu-pool-1/resources | jq

# 获取 Prometheus 指标
curl http://localhost:8080/metrics
```

## 常用操作

### 分配资源

```bash
curl -X POST http://localhost:8080/api/v1/pools/gpu-pool-1/allocate \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "task-002",
    "task_type": "inference",
    "resource_type": "gpu",
    "count": 1,
    "memory_gb": 24,
    "cores": 50,
    "priority": 8,
    "timeout": "3600s"
  }' | jq
```

### 释放资源

```bash
curl -X POST http://localhost:8080/api/v1/pools/gpu-pool-1/release/alloc-123
```

### 取消任务

```bash
curl -X DELETE http://localhost:8080/api/v1/tasks/task-001
```

## 配置说明

### 环境变量配置

编辑 `deploy/k8s/deployment.yaml` 中的环境变量：

```yaml
env:
- name: GPU_DEVICE_PLUGIN_ENDPOINT
  value: "unix:///var/lib/kubelet/device-plugins/gpu.sock"
- name: GPU_EXPORTER_ENDPOINT
  value: "http://xpu-exporter:9091"
- name: NPU_HYPERVISOR_ENDPOINT
  value: "http://flexnpud:8080"
```

### 资源限制

调整资源请求和限制：

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

## 故障排查

### Pod 无法启动

```bash
# 查看 Pod 状态
kubectl get pods -n flexai-system

# 查看日志
kubectl logs -n flexai-system -l app=unified-resource-manager

# 查看事件
kubectl describe pod -n flexai-system -l app=unified-resource-manager
```

### API 无法访问

```bash
# 检查服务
kubectl get svc -n flexai-system

# 检查端口转发
kubectl port-forward -n flexai-system svc/unified-resource-manager 8080:80

# 测试连接
curl -v http://localhost:8080/health
```

### 资源分配失败

```bash
# 检查资源池状态
curl http://localhost:8080/api/v1/pools | jq

# 检查底层服务
kubectl get pods -n kube-system | grep gpu
kubectl get pods | grep flexnpud
```

## 下一步

- 阅读 [API 文档](docs/API.md) 了解完整的 API 接口
- 阅读 [架构文档](docs/ARCHITECTURE.md) 了解系统设计
- 阅读 [部署文档](DEPLOYMENT.md) 了解生产环境部署
- 阅读 [实现说明](IMPLEMENTATION.md) 了解技术细节

## 获取帮助

- 查看日志: `kubectl logs -n flexai-system -l app=unified-resource-manager`
- 查看指标: `curl http://localhost:8080/metrics`
- 查看健康状态: `curl http://localhost:8080/health`
