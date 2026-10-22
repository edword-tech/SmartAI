# Flex:ai 统一异构资源管理系统

## 概述

Flex:ai 统一异构资源管理系统是一个面向商业化的 AI 资源管理平台，在保持原有 GPU-Virtual-Service 和 Ascend-Virtual-Service 代码不变的基础上，提供统一的异构资源管理能力。

## 核心功能

### 1. 异构资源池化管理
- **GPU 资源池**: 统一管理集群中的所有 GPU 资源，支持虚拟化切分
- **NPU 资源池**: 统一管理集群中的所有 NPU 资源，支持算力切分
- **资源发现**: 自动发现和注册集群中的 GPU/NPU 节点
- **资源聚合**: 将分散的 GPU/NPU 资源聚合成统一的资源池

### 2. 智能资源分配
- **统一调度**: 支持 GPU 和 NPU 的统一调度策略
- **任务感知**: 根据任务类型（推理/训练）智能分配资源
- **优先级调度**: 支持多级优先级和抢占机制
- **Binpack 调度**: 优化资源利用率

### 3. 可观测性
- **实时监控**: GPU/NPU 使用率、温度、功耗等指标
- **任务监控**: 推理/训练任务的状态、性能指标
- **告警系统**: 资源异常、任务失败等告警
- **日志聚合**: 统一的日志收集和分析

### 4. 大规模集群管理
- **节点管理**: 节点注册、健康检查、自动恢复
- **多租户**: 支持多租户资源隔离和配额管理
- **权限控制**: 基于角色的访问控制（RBAC）
- **资源配额**: 租户级别的资源配额限制

### 5. 商业化特性
- **RESTful API**: 完整的 REST API 接口
- **Web 管理界面**: 可视化的资源管理和监控界面
- **多租户支持**: 企业级多租户隔离
- **计费统计**: 资源使用统计和计费支持
- **审计日志**: 完整的操作审计日志

## 架构设计

```
┌─────────────────────────────────────────────────────────┐
│              Unified Resource Manager                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │  API Gateway │  │   Scheduler  │  │  Monitor     │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ Resource Pool│  │ Cluster Mgr  │  │  Web UI      │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────┘
         │                      │
         ▼                      ▼
┌─────────────────┐    ┌─────────────────┐
│ GPU-Virtual-    │    │ Ascend-Virtual- │
│ Service         │    │ Service         │
│ (Unchanged)     │    │ (Unchanged)     │
└─────────────────┘    └─────────────────┘
```

## 目录结构

```
unified-resource-manager/
├── api/                    # API 定义和实现
│   ├── rest/              # REST API
│   ├── grpc/              # gRPC API
│   └── proto/             # Protocol Buffers 定义
├── core/                   # 核心服务
│   ├── pool/              # 资源池管理
│   ├── scheduler/         # 调度器
│   ├── allocator/         # 资源分配器
│   └── cluster/           # 集群管理
├── observability/          # 可观测性
│   ├── metrics/           # 指标收集
│   ├── logs/              # 日志管理
│   └── alerts/            # 告警系统
├── web/                    # Web 管理界面
│   ├── frontend/          # 前端界面
│   └── backend/           # 后端服务
├── auth/                   # 认证授权
│   ├── rbac/              # 权限控制
│   └── tenant/            # 多租户
├── adapters/               # 适配器层
│   ├── gpu/               # GPU 服务适配器
│   └── npu/               # NPU 服务适配器
└── deploy/                 # 部署配置
    ├── k8s/               # Kubernetes 部署
    └── docker/            # Docker 配置
```

## 快速开始

### 前置要求
- Go 1.21+
- Node.js 18+（用于前端）
- Kubernetes 1.31.1+（可选，用于 K8s 部署）
- GPU-Virtual-Service 已部署（如使用 GPU）
- Ascend-Virtual-Service 已部署（如使用 NPU）

### 本地开发运行

#### 1. 启动后端服务

```bash
cd unified-resource-manager
go run cmd/main.go
```

后端服务将在 `http://localhost:8080` 启动

#### 2. 启动前端界面

**请先完成步骤 1 启动后端**，否则前端代理请求会报 `ECONNREFUSED`。

```bash
cd unified-resource-manager/web/frontend
npm install
npm run dev
```

前端界面将在 `http://localhost:3000` 启动；代理默认指向 `http://127.0.0.1:8080`，可通过环境变量 `VITE_PROXY_TARGET` 修改。

#### 3. 访问系统

- Web 界面：http://localhost:3000
- API 端点：http://localhost:8080/api/v1

### Kubernetes 部署

```bash
# 1. 部署统一资源管理器后端
cd unified-resource-manager/deploy/k8s
kubectl apply -f deployment.yaml

# 2. 部署 Web 管理系统
kubectl apply -f web-deployment.yaml

# 3. 访问 Web 界面
kubectl port-forward -n flexai-system svc/unified-resource-manager-web 3000:80
# 浏览器访问 http://localhost:3000
```

### Docker 运行

```bash
# 构建后端镜像
cd unified-resource-manager
docker build -t unified-resource-manager:latest .

# 构建前端镜像
cd web/frontend
docker build -t unified-resource-manager-web:latest .

# 运行
docker run -d -p 8080:8080 unified-resource-manager:latest
docker run -d -p 3000:80 unified-resource-manager-web:latest
```

详细说明请参考：
- [docs/安装部署指南-K8s生产环境.md](docs/安装部署指南-K8s生产环境.md) - **K8s 生产环境安装部署步骤**
- [WEB_QUICKSTART.md](WEB_QUICKSTART.md) - Web 界面快速开始
- [docs/USAGE_GUIDE.md](docs/USAGE_GUIDE.md) - 详细使用指南
- [docs/国产卡兼容性.md](docs/国产卡兼容性.md) - 国产 GPU/NPU 兼容程度与扩展方式
- [docs/异构算力平台对比与演进建议.md](docs/异构算力平台对比与演进建议.md) - GCP/AWS/阿里云/趋动/FlexAI/Run:ai 对比与平台补全建议
- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) - 实现状态
- [FEATURES_SUMMARY.md](FEATURES_SUMMARY.md) - 功能总结

## API 文档

详细的 API 文档请参考 [API.md](docs/API.md)

## 使用示例

### 创建资源池

```bash
curl -X POST http://api.example.com/v1/pools \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gpu-pool-1",
    "type": "gpu",
    "nodes": ["node1", "node2"],
    "quota": {
      "max_gpus": 100,
      "max_memory_gb": 10240
    }
  }'
```

### 提交推理任务

```bash
curl -X POST http://api.example.com/v1/tasks/inference \
  -H "Content-Type: application/json" \
  -d '{
    "name": "llm-inference-1",
    "model": "deepseek-r1",
    "resource": {
      "type": "gpu",
      "count": 1,
      "memory_gb": 24,
      "cores": 50
    },
    "priority": "high"
  }'
```

## 许可证

与 Flex:ai 项目保持一致
