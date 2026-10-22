# Unified Resource Manager Web UI

统一异构资源管理系统的 Web 管理界面。

## 功能特性

- 📊 **仪表板**: 资源池、任务、节点概览
- 🗄️ **资源池管理**: 查看和管理 GPU/NPU 资源池
- ⚡ **任务管理**: 提交、查看、取消推理和训练任务
- 🖥️ **节点管理**: 查看集群节点状态
- 📈 **监控指标**: 资源使用率和性能指标可视化

## 技术栈

- React 18
- TypeScript
- Ant Design 5
- Vite
- Recharts (图表)
- Axios (HTTP 客户端)

## 开发

### 安装依赖

```bash
npm install
```

### 启动开发服务器

**请先启动后端 API 服务**，否则前端代理会报 `ECONNREFUSED`（/api/v1/pools、/api/v1/tasks 等请求失败）。

在项目根目录（unified-resource-manager）执行：

```bash
make run
# 或
go run ./cmd/main.go -port 8080
```

再启动前端：

```bash
npm run dev
```

访问 http://localhost:3000

若后端运行在其他地址，可设置代理目标：

```bash
VITE_PROXY_TARGET=http://192.168.1.100:8080 npm run dev
```

### 构建生产版本

```bash
npm run build
```

## 部署

### Docker 构建

```bash
docker build -t unified-resource-manager-web:latest .
```

### Kubernetes 部署

```bash
kubectl apply -f ../../deploy/k8s/web-deployment.yaml
```

## 环境变量

- `VITE_API_BASE_URL`: API 基础 URL (默认: `/api/v1`)
- `VITE_PROXY_TARGET`: 开发时代理目标 (默认: `http://127.0.0.1:8080`)，后端未启动时会报代理错误

## 功能说明

### 仪表板
- 显示资源池数量、总资源数、可用资源、运行中任务等统计信息
- 资源池概览卡片
- 最近任务列表

### 资源池管理
- 列出所有资源池
- 查看每个资源池的详细信息
- 展开查看资源池中的资源列表
- 创建新资源池（需要后端支持）

### 任务管理
- 列出所有任务及其状态
- 提交新的推理或训练任务
- 取消运行中或待处理的任务
- 实时刷新任务状态

### 节点管理
- 查看集群节点列表
- 节点状态和类型信息

### 监控指标
- 资源利用率趋势图
- 内存使用趋势图
- 支持选择不同资源池查看
