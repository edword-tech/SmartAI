# Web 管理系统快速启动指南

## 5 分钟快速体验 Web 管理系统

### 前置条件

1. Node.js 18+ 已安装
2. 后端 API 服务已运行（或使用代理）

### 步骤 1: 安装依赖

```bash
cd unified-resource-manager/web/frontend
npm install
```

### 步骤 2: 启动开发服务器

```bash
npm run dev
```

### 步骤 3: 访问 Web 界面

打开浏览器访问: http://localhost:3000

## 功能演示

### 1. 仪表板

- 查看资源池统计
- 查看任务概览
- 实时数据刷新

### 2. 资源池管理

- 点击左侧菜单 "资源池"
- 查看所有资源池列表
- 点击展开查看资源详情

### 3. 提交任务

- 点击左侧菜单 "任务管理"
- 点击 "提交任务" 按钮
- 填写任务信息：
  - 任务名称
  - 任务类型（推理/训练）
  - 资源类型（GPU/NPU）
  - 资源数量
  - 内存大小
  - 核心数
  - 优先级
- 点击确定提交

### 4. 查看监控

- 点击左侧菜单 "监控指标"
- 选择资源池
- 查看资源利用率和内存使用趋势

## Docker 快速启动

### 构建镜像

```bash
cd unified-resource-manager/web/frontend
docker build -t unified-resource-manager-web:latest .
```

### 运行容器

```bash
docker run -d -p 3000:80 \
  --name resource-manager-web \
  unified-resource-manager-web:latest
```

访问: http://localhost:3000

## Kubernetes 部署

### 部署 Web 服务

```bash
# 确保后端服务已部署
kubectl get svc -n flexai-system unified-resource-manager

# 部署 Web 服务
kubectl apply -f unified-resource-manager/deploy/k8s/web-deployment.yaml

# 检查部署状态
kubectl get pods -n flexai-system -l app=unified-resource-manager-web

# 端口转发
kubectl port-forward -n flexai-system svc/unified-resource-manager-web 3000:80
```

访问: http://localhost:3000

## 配置说明

### 开发环境

创建 `web/frontend/.env.local`:

```env
VITE_API_BASE_URL=http://localhost:8080/api/v1
```

### 生产环境

在 `nginx.conf` 中配置 API 代理，或设置环境变量：

```env
VITE_API_BASE_URL=http://unified-resource-manager:80/api/v1
```

## 常见问题

### Q: 页面显示空白？

A: 检查浏览器控制台，确认 API 连接正常。确保后端服务运行在正确的地址。

### Q: API 请求失败？

A: 
1. 检查后端服务是否运行
2. 检查 API_BASE_URL 配置
3. 检查 CORS 设置（开发环境）

### Q: 如何修改 API 地址？

A: 
- 开发环境: 修改 `vite.config.ts` 中的 proxy 配置
- 生产环境: 修改 `nginx.conf` 中的 proxy_pass 配置

## 下一步

- 阅读 [Web README](web/README.md) 了解详细功能
- 阅读 [API 文档](docs/API.md) 了解 API 接口
- 阅读 [架构文档](docs/ARCHITECTURE.md) 了解系统设计
