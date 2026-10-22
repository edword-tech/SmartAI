# Unified Resource Manager 部署指南

## 前置要求

### 系统要求
- Kubernetes 1.31.1+
- GPU-Virtual-Service 已部署并运行
- Ascend-Virtual-Service 已部署并运行（如使用 NPU）
- 至少 2GB 可用内存
- 至少 1 CPU 核心

### 依赖服务
- GPU Device Plugin (来自 GPU-Virtual-Service)
- GPU Runtime Service (来自 GPU-Virtual-Service)
- GPU Exporter (来自 GPU-Virtual-Service)
- NPU Hypervisor (flexnpud, 来自 Ascend-Virtual-Service)

## 快速部署

### 1. 创建命名空间

```bash
kubectl create namespace flexai-system
```

### 2. 部署 Unified Resource Manager

```bash
cd unified-resource-manager/deploy/k8s
kubectl apply -f deployment.yaml
```

### 3. 验证部署

```bash
# 检查 Pod 状态
kubectl get pods -n flexai-system

# 检查服务状态
kubectl get svc -n flexai-system

# 查看日志
kubectl logs -n flexai-system -l app=unified-resource-manager
```

### 4. 访问服务

```bash
# 端口转发
kubectl port-forward -n flexai-system svc/unified-resource-manager 8080:80

# 测试健康检查
curl http://localhost:8080/health
```

## 配置说明

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `PORT` | API 服务器端口 | 8080 |
| `GPU_DEVICE_PLUGIN_ENDPOINT` | GPU device plugin 端点 | `unix:///var/lib/kubelet/device-plugins/gpu.sock` |
| `GPU_RUNTIME_ENDPOINT` | GPU runtime 服务端点 | `unix:///var/run/gpu-runtime.sock` |
| `GPU_EXPORTER_ENDPOINT` | GPU exporter 端点 | `http://xpu-exporter:9091` |
| `NPU_HYPERVISOR_ENDPOINT` | NPU hypervisor 端点 | `http://flexnpud:8080` |
| `NPU_ACL_SERVER_ENDPOINT` | NPU ACL server 端点 | `http://aclserver:8625` |

### 资源配置

在 `deployment.yaml` 中调整资源请求和限制:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

## 与 GPU-Virtual-Service 集成

### 1. 确保 GPU-Virtual-Service 已部署

参考 GPU-Virtual-Service 的部署文档，确保以下组件已部署:
- GPU Device Plugin
- GPU Runtime Service
- GPU Exporter

### 2. 配置端点

在 `deployment.yaml` 中配置正确的端点:

```yaml
env:
- name: GPU_DEVICE_PLUGIN_ENDPOINT
  value: "unix:///var/lib/kubelet/device-plugins/gpu.sock"
- name: GPU_RUNTIME_ENDPOINT
  value: "unix:///var/run/gpu-runtime.sock"
- name: GPU_EXPORTER_ENDPOINT
  value: "http://xpu-exporter:9091"
```

### 3. 挂载必要的卷

```yaml
volumeMounts:
- name: kubelet-socket
  mountPath: /var/lib/kubelet/device-plugins
  readOnly: true
- name: runtime-socket
  mountPath: /var/run
  readOnly: true
```

## 与 Ascend-Virtual-Service 集成

### 1. 确保 Ascend-Virtual-Service 已部署

参考 Ascend-Virtual-Service 的部署文档，确保以下组件已部署:
- flexnpud (hypervisor)
- aclserver

### 2. 配置端点

在 `deployment.yaml` 中配置正确的端点:

```yaml
env:
- name: NPU_HYPERVISOR_ENDPOINT
  value: "http://flexnpud:8080"
- name: NPU_ACL_SERVER_ENDPOINT
  value: "http://aclserver:8625"
```

## 高可用部署

### 多副本部署

修改 `deployment.yaml`:

```yaml
spec:
  replicas: 3
```

### 使用 StatefulSet

对于需要持久化状态的场景，可以使用 StatefulSet:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: unified-resource-manager
spec:
  serviceName: unified-resource-manager
  replicas: 3
  # ...
```

## 监控和告警

### Prometheus 集成

Unified Resource Manager 暴露 Prometheus 指标在 `/metrics` 端点。

配置 Prometheus 抓取:

```yaml
scrape_configs:
- job_name: 'unified-resource-manager'
  kubernetes_sd_configs:
  - role: pod
    namespaces:
      names:
      - flexai-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_pod_label_app]
    action: keep
    regex: unified-resource-manager
```

### Grafana 仪表板

创建 Grafana 仪表板监控:
- 资源池利用率
- 任务调度统计
- 节点健康状态
- API 请求延迟

## 故障排查

### Pod 无法启动

1. 检查 Pod 日志:
```bash
kubectl logs -n flexai-system -l app=unified-resource-manager
```

2. 检查端点配置是否正确
3. 检查底层服务是否正常运行

### 无法连接底层服务

1. 检查网络连接
2. 验证端点地址
3. 检查服务发现配置

### 资源分配失败

1. 检查资源池状态
2. 查看调度器日志
3. 验证底层服务资源可用性

## 升级指南

### 滚动升级

```bash
# 更新镜像
kubectl set image deployment/unified-resource-manager \
  manager=flexai/unified-resource-manager:v2.0.0 \
  -n flexai-system

# 查看升级状态
kubectl rollout status deployment/unified-resource-manager -n flexai-system
```

### 回滚

```bash
kubectl rollout undo deployment/unified-resource-manager -n flexai-system
```

## 卸载

```bash
kubectl delete -f deployment.yaml
kubectl delete namespace flexai-system
```
