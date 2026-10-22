# Flex:ai 介绍及使用说明  

## 概述

Flex:ai是一个面向AI容器场景的开源项目，其核心能力包含两大部分，分别是XPU虚拟化和多级智能调度。其中XPU虚拟化分为本地XPU虚拟化和跨节点拉远虚拟化，本地XPU虚拟化支持将1台服务器上的XPU算力卡虚拟化分割出多个虚拟算力单元，实现单张算力卡在多个容器的共享。跨节点拉远虚拟化支持通过RDMA或TCP网络访问远端节点上的算力卡。多级智能调度支持对切分出的虚拟卡进行Binpack等资源级调度，以及对AI训推任务进行分时调度等能力。

## 动机

用户的AI业务集群通常是大小模型混部的场景，集群除了运行AI大模型如Deepseek、Qwen系列，还有大量的小参数量模型，如CV模型、Embedding模型等，这些小参数量模型无法充分使用整张算力卡的资源，从而导致昂贵的GPU/NPU资源存在浪费。此外算力集群中运行的AI工作负载千差万别，如何基于有限的算力资源对大并发的AI工作负载进行高效调度是一个难题。基于上述的背景，我们构筑了Flex:ai开源项目，提供将XPU算力卡进行虚拟化切分，以及面向AI训推任务和集群资源做智能调度的能力。

# Ascend NPU虚拟化

请查看[Ascend-Virtual-Service](Ascend-Virtual-Service/)

# GPU虚拟化

请查看[GPU-Virtual-Service](GPU-Virtual-Service/)

# 统一异构资源管理系统

请查看[Unified Resource Manager](unified-resource-manager/)

## 概述

统一异构资源管理系统是一个面向商业化的 AI 资源管理平台，在保持原有 GPU-Virtual-Service 和 Ascend-Virtual-Service 代码不变的基础上，提供统一的异构资源管理能力。

### 核心功能

- **异构资源池化管理**: 统一管理 GPU 和 NPU 资源池
- **智能资源分配**: 支持推理和训练任务的智能调度
- **可观测性**: 完整的监控、指标、日志和告警系统
- **大规模集群管理**: 节点管理、资源发现、健康检查
- **商业化特性**: RESTful API、Web 管理界面、多租户支持

### 快速开始

```bash
# 部署统一资源管理器
cd unified-resource-manager/deploy/k8s
kubectl apply -f deployment.yaml

# 访问 API
curl http://localhost:8080/api/v1/pools
```

详细文档请参考 [unified-resource-manager/README.md](unified-resource-manager/README.md)


1. 启动后端
cd unified-resource-manager
go run cmd/main.go
后端会在 http://localhost:8080 提供 API。
建议先启动后端，再启动前端，否则前端请求会连不上（ECONNREFUSED）。
2. 启动前端
新开一个终端：

cd unified-resource-manager/web/frontend
npm install    # 首次需要
npm run dev
前端会在 http://localhost:3000 启动。
开发时请求会通过 Vite 代理到 http://127.0.0.1:8080；如需改代理目标，可设环境变量 VITE_PROXY_TARGET。
3. 访问
Web 界面：http://localhost:3000
API 根路径：http://localhost:8080/api/v1