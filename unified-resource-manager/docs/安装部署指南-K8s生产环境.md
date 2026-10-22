# Flex:ai 统一资源管理器 — Kubernetes 生产环境安装部署指南

本文档面向**生产级 Kubernetes 环境**，给出从构建镜像、推送仓库到集群部署、Ingress、监控与运维的完整步骤，并说明与 GPU/NPU 底层服务的对接方式。

---

## 一、概述

### 1.1 组件说明

| 组件 | 说明 | 端口 |
|------|------|------|
| **unified-resource-manager** | 后端 API 服务（Go） | 8080 |
| **unified-resource-manager-web** | 前端静态站点（Nginx），代理 `/api` 到后端 | 80 |

前端通过 Nginx 将 `/api` 代理到同命名空间内的后端 Service，前端请求基路径为 `/api/v1`。

### 1.2 生产环境假设

- Kubernetes 集群版本 **1.24+**（建议 1.26+）
- 已具备镜像仓库（如 Harbor、ACR、ECR）及集群拉取权限
- 需使用 GPU 时：集群已部署 GPU 设备插件 / GPU-Virtual-Service
- 需使用 NPU 时：集群已部署 Ascend-Virtual-Service（如 flexnpud、aclserver）
- 对外访问：通过 Ingress（如 Nginx Ingress）暴露 Web，可选 TLS

---

## 二、前置要求

### 2.1 集群与工具

- **kubectl** 已配置且能访问目标集群
- **Docker** 或 **containerd** + **buildkit**，用于构建镜像
- （可选）**Helm**，若后续采用 Helm 编排

### 2.2 依赖服务（按需）

| 能力 | 依赖组件 | 说明 |
|------|----------|------|
| GPU | GPU Device Plugin、GPU Runtime、GPU Exporter | 如 GPU-Virtual-Service 提供的组件 |
| NPU | flexnpud（Hypervisor）、aclserver | 如 Ascend-Virtual-Service |

未部署上述依赖时，本系统仍可启动，GPU/NPU 相关适配器会使用默认端点（可能不可用），不影响 API 与 Web 基础功能。

### 2.3 命名空间与镜像

- 默认使用命名空间：**flexai-system**
- 默认镜像名称示例：
  - 后端：`<your-registry>/flexai/unified-resource-manager:<tag>`
  - 前端：`<your-registry>/flexai/unified-resource-manager-web:<tag>`

---

## 三、构建与推送镜像

### 3.1 后端镜像

```bash
cd /path/to/unified-resource-manager

# 指定版本标签，便于生产追溯
export IMAGE_TAG=v1.0.0
export REGISTRY=hub.dataagent.cc

docker build -t ${REGISTRY}/dev/unified-resource-manager:${IMAGE_TAG} -f Dockerfile .
docker push ${REGISTRY}/dev/unified-resource-manager:${IMAGE_TAG}

hub.dataagent.cc/dev/unified-resource-manager:v1.0.0
```

- 镜像基于 **Dockerfile** 多阶段构建，最终运行用户为非 root，并带 `/health` 健康检查。
- 若使用私有仓库，部署前需在集群配置 **imagePullSecrets**。

### 3.2 前端镜像

```bash
cd /path/to/unified-resource-manager/web/frontend

docker build -t ${REGISTRY}/dev/unified-resource-manager-web:${IMAGE_TAG} -f Dockerfile .
docker push ${REGISTRY}/dev/unified-resource-manager-web:${IMAGE_TAG}

hub.dataagent.cc/dev/unified-resource-manager-web:v1.0.0 
```

- 构建产物由 Nginx 提供，**nginx.conf** 中已将 `location /api` 代理到 `http://unified-resource-manager:80`，与 K8s Service 名称一致。

---

## 四、部署步骤

### 4.1 创建命名空间

```bash
kubectl create namespace flexai-system
```

### 4.2 配置镜像与拉取密钥（可选）

若使用私有仓库：

```bash
kubectl create secret docker-registry flexai-registry-secret \
  --docker-server=${REGISTRY} \
  --docker-username=<user> \
  --docker-password=<password> \
  -n flexai-system
```

在 Deployment 的 `spec.template.spec` 中增加：

```yaml
imagePullSecrets:
  - name: flexai-registry-secret
```

### 4.3 后端配置说明（生产建议）

后端通过**命令行参数**读取配置（非环境变量），当前支持：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-port` | 8080 | API 监听端口 |
| `-gpu-device-plugin` | unix:///var/lib/kubelet/device-plugins/gpu.sock | GPU 设备插件 |
| `-gpu-runtime` | unix:///var/run/gpu-runtime.sock | GPU 运行时 |
| `-gpu-exporter` | http://localhost:9091 | GPU Exporter |
| `-npu-hypervisor` | http://localhost:8080 | NPU Hypervisor |
| `-npu-acl-server` | http://localhost:8625 | NPU ACL Server |

生产环境若需覆盖默认值，可在 **deploy/k8s/deployment.yaml** 的容器中增加 `args`，例如：

```yaml
containers:
  - name: manager
    image: ${REGISTRY}/flexai/unified-resource-manager:${IMAGE_TAG}
    args:
      - "-port=8080"
      - "-gpu-exporter=http://xpu-exporter.monitoring:9091"
      - "-npu-hypervisor=http://flexnpud.ascend:8080"
      - "-npu-acl-server=http://aclserver.ascend:8625"
    # ... 其余配置保持不变
```

若使用集群内 Service 名称，请根据实际部署的命名空间与服务名修改上述 URL。

### 4.4 应用清单

**1）修改镜像地址**

编辑 `deploy/k8s/deployment.yaml` 和 `deploy/k8s/web-deployment.yaml`，将 `image` 改为你的仓库与标签，例如：

```yaml
# deployment.yaml
image: your-registry.example.com/flexai/unified-resource-manager:v1.0.0
imagePullPolicy: IfNotPresent  # 生产建议 Always 或按策略
```

```yaml
# web-deployment.yaml
image: your-registry.example.com/flexai/unified-resource-manager-web:v1.0.0
```

**2）一键部署**

```bash
cd /path/to/unified-resource-manager/deploy/k8s

# 后端（含 Service、ServiceAccount、RBAC）
kubectl apply -f deployment.yaml

# 前端（含 Service、Ingress 示例）
kubectl apply -f web-deployment.yaml
```

**3）等待就绪**

```bash
kubectl rollout status deployment/unified-resource-manager -n flexai-system
kubectl rollout status deployment/unified-resource-manager-web -n flexai-system
```

### 4.5 生产可选增强

- **资源限制**：在 `deployment.yaml` / `web-deployment.yaml` 中已配置 `resources.requests/limits`，可按集群规模调整。
- **多副本**：将 `spec.replicas` 调大（如 2），并配合 Service 实现负载均衡；注意当前后端为无状态设计，无持久化状态。
- **PodDisruptionBudget**：建议为后端添加 PDB，避免同时驱逐过多副本：

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: unified-resource-manager-pdb
  namespace: flexai-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: unified-resource-manager
```

---

## 五、Ingress 与对外访问（生产）

### 5.1 使用 Nginx Ingress

`web-deployment.yaml` 中已包含 Ingress 示例，规则为 `resource-manager.local`。生产请改为实际域名并配置 TLS，例如：

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: unified-resource-manager-web
  namespace: flexai-system
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "50m"
    # TLS 与证书（按集群 Ingress 控制器支持方式配置）
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - resource-manager.your-domain.com
      secretName: flexai-web-tls
  rules:
    - host: resource-manager.your-domain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: unified-resource-manager-web
                port:
                  number: 80
```

- 用户访问 `https://resource-manager.your-domain.com` 即可使用 Web 界面。
- 前端请求 `/api/v1` 会由 Nginx 代理到后端 Service，无需在浏览器侧再配 API 地址。

### 5.2 仅内网访问时

若不使用 Ingress，可通过端口转发临时访问：

```bash
# Web
kubectl port-forward -n flexai-system svc/unified-resource-manager-web 3000:80

# 后端 API（调试用）
kubectl port-forward -n flexai-system svc/unified-resource-manager 8080:80
```

浏览器访问：`http://localhost:3000`。

---

## 六、健康检查与就绪

- **后端**
  - 存活：`GET http://<pod-ip>:8080/health`
  - 就绪：`GET http://<pod-ip>:8080/ready`
- **前端**：对 `GET /` 返回 200 即视为就绪。

当前 `deployment.yaml` 已配置 `livenessProbe` / `readinessProbe`，无需额外修改即可被 K8s 用于重启与摘流。

---

## 七、监控（Prometheus）

后端暴露 Prometheus 指标（若已接入 observability 模块，路径一般为 `/metrics`）。在 Prometheus 中增加抓取配置，例如：

```yaml
scrape_configs:
  - job_name: 'unified-resource-manager'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names: [flexai-system]
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: keep
        regex: unified-resource-manager
      - source_labels: [__meta_kubernetes_pod_ip]
        action: replace
        target_label: __address__
        replacement: ${1}:8080
      - source_labels: [__meta_kubernetes_pod_ip]
        action: replace
        target_label: instance
        replacement: ${1}
      - target_label: __metrics_path__
        replacement: /metrics
```

具体路径以项目中 `observability/metrics` 注册的路径为准。

---

## 八、与 GPU/NPU 环境对接

### 8.1 GPU

- 若 GPU 相关服务部署在同一集群，将后端 `args` 中的 `-gpu-exporter` 等改为对应 Service 地址（如 `http://xpu-exporter.<namespace>:9091`）。
- 若使用宿主机上的 socket（如 device-plugin、runtime），需在 Pod 中挂载 hostPath，并确保节点上有对应路径；当前 `deployment.yaml` 中已包含示例 volumeMount（kubelet device-plugins、/var/run），按实际路径调整。

### 8.2 NPU

- 将 `-npu-hypervisor`、`-npu-acl-server` 指向集群内 NPU 服务地址（如 `http://flexnpud.ascend:8080`、`http://aclserver.ascend:8625`），无需 hostPath。

---

## 九、验证部署

```bash
# Pod
kubectl get pods -n flexai-system -l 'app in (unified-resource-manager,unified-resource-manager-web)'

# Service
kubectl get svc -n flexai-system

# 后端健康（通过 port-forward 或集群内 curl）
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -s http://unified-resource-manager.flexai-system.svc.cluster.local/health
```

通过 Ingress 或 port-forward 访问 Web，登录后查看资源池、节点、任务等是否正常加载。

---

## 十、升级与回滚

**升级**（替换镜像标签后应用）：

```bash
kubectl set image deployment/unified-resource-manager \
  manager=your-registry/flexai/unified-resource-manager:v1.1.0 \
  -n flexai-system
kubectl set image deployment/unified-resource-manager-web \
  web=your-registry/flexai/unified-resource-manager-web:v1.1.0 \
  -n flexai-system

kubectl rollout status deployment/unified-resource-manager -n flexai-system
kubectl rollout status deployment/unified-resource-manager-web -n flexai-system
```

**回滚**：

```bash
kubectl rollout undo deployment/unified-resource-manager -n flexai-system
kubectl rollout undo deployment/unified-resource-manager-web -n flexai-system
```

---

## 十一、卸载

```bash
kubectl delete -f deploy/k8s/web-deployment.yaml
kubectl delete -f deploy/k8s/deployment.yaml
kubectl delete namespace flexai-system
```

---

## 十二、相关文档

- [README.md](../README.md) — 项目概述与本地开发
- [DEPLOYMENT.md](../DEPLOYMENT.md) — 部署说明与高可用、监控
- [docs/USAGE_GUIDE.md](USAGE_GUIDE.md) — 使用指南
- [docs/异构算力平台对比与演进建议.md](异构算力平台对比与演进建议.md) — 平台对比与演进建议
