# Unified Resource Manager Web 管理系统

完整的 Web 管理界面，用于统一管理 GPU 和 NPU 异构资源。

## 功能特性

- 📊 **仪表板**: 实时显示资源池、任务、节点概览
- 🗄️ **资源池管理**: 查看和管理 GPU/NPU 资源池，查看资源详情
- ⚡ **任务管理**: 提交、查看、取消推理和训练任务
- 🖥️ **节点管理**: 查看集群节点状态和健康信息
- 📈 **监控指标**: 资源使用率和性能指标可视化

## 快速开始

### 方式一：本地开发运行

```bash
cd web/frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev
```

访问 http://localhost:3000

### 方式二：Docker 运行

```bash
cd web/frontend

# 构建镜像
docker build -t unified-resource-manager-web:latest .

# 运行容器
docker run -d -p 3000:80 \
  -e VITE_API_BASE_URL=http://your-api-server/api/v1 \
  unified-resource-manager-web:latest
```

### 方式三：Kubernetes 部署

```bash
# 部署 Web 服务
kubectl apply -f deploy/k8s/web-deployment.yaml

# 端口转发
kubectl port-forward -n flexai-system svc/unified-resource-manager-web 3000:80
```

访问 http://localhost:3000

## 技术栈

- **React 18**: UI 框架
- **TypeScript**: 类型安全
- **Ant Design 5**: UI 组件库
- **Vite**: 构建工具
- **Recharts**: 图表库
- **Axios**: HTTP 客户端

## 项目结构

```
web/frontend/
├── src/
│   ├── api/              # API 客户端
│   │   ├── client.ts     # Axios 配置
│   │   ├── pools.ts      # 资源池 API
│   │   └── tasks.ts      # 任务 API
│   ├── components/       # 组件
│   │   └── Layout/       # 布局组件
│   ├── pages/            # 页面
│   │   ├── Dashboard.tsx # 仪表板
│   │   ├── ResourcePools.tsx # 资源池管理
│   │   ├── Tasks.tsx     # 任务管理
│   │   ├── Nodes.tsx     # 节点管理
│   │   └── Metrics.tsx   # 监控指标
│   ├── App.tsx           # 主应用
│   └── main.tsx          # 入口文件
├── public/                # 静态资源
├── package.json          # 依赖配置
├── vite.config.ts        # Vite 配置
├── tsconfig.json         # TypeScript 配置
└── Dockerfile            # Docker 构建文件
```

## 环境变量

创建 `.env` 文件：

```env
VITE_API_BASE_URL=/api/v1
```

## 开发

### 安装依赖

```bash
npm install
```

### 启动开发服务器

```bash
npm run dev
```

### 构建生产版本

```bash
npm run build
```

构建产物在 `dist/` 目录。

### 预览生产构建

```bash
npm run preview
```

## 部署

### Docker 构建

```bash
docker build -t unified-resource-manager-web:latest .
```

### Kubernetes 部署

确保后端 API 服务已部署，然后：

```bash
kubectl apply -f deploy/k8s/web-deployment.yaml
```

Web 服务会自动代理 API 请求到后端服务。

## API 集成

Web 前端通过以下 API 与后端交互：

- `GET /api/v1/pools` - 获取资源池列表
- `GET /api/v1/pools/{id}` - 获取资源池详情
- `GET /api/v1/pools/{id}/resources` - 获取资源列表
- `GET /api/v1/tasks` - 获取任务列表
- `POST /api/v1/tasks` - 提交任务
- `DELETE /api/v1/tasks/{id}` - 取消任务

详细 API 文档请参考 [API.md](../docs/API.md)

## 功能说明

### 仪表板
- 显示资源池数量、总资源数、可用资源、运行中任务等统计信息
- 资源池概览卡片，显示每个资源池的基本信息
- 最近任务列表，显示最近的任务及其状态

### 资源池管理
- 列出所有资源池（GPU 和 NPU）
- 展开查看每个资源池中的资源列表
- 查看资源详情（内存、核心数、状态等）
- 创建新资源池（需要后端 API 支持）

### 任务管理
- 列出所有任务及其状态
- 提交新的推理或训练任务
- 取消运行中或待处理的任务
- 实时刷新任务状态（每 5 秒）

### 节点管理
- 查看集群节点列表
- 节点状态和类型信息
- 节点健康检查状态

### 监控指标
- 资源利用率趋势图
- 内存使用趋势图
- 支持选择不同资源池查看
- 支持时间范围选择

## 故障排查

### 无法连接 API

1. 检查后端服务是否运行
2. 检查 API_BASE_URL 配置
3. 检查网络连接和防火墙设置

### 页面空白

1. 检查浏览器控制台错误
2. 检查 API 响应格式
3. 检查环境变量配置

### 构建失败

1. 检查 Node.js 版本（需要 18+）
2. 清除 node_modules 重新安装
3. 检查 TypeScript 配置

## 贡献

欢迎提交 Issue 和 Pull Request！
