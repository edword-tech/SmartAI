# 完整功能快速开始指南

## 🎯 所有功能已实现并可用

### 1️⃣ 启动系统

#### 启动后端
```bash
cd unified-resource-manager
go run cmd/main.go
```

后端将在 `http://localhost:8080` 启动，自动初始化：
- ✅ 5 个资源规格（GPU/NPU 推理/训练）
- ✅ 3 个示例节点（GPU节点、NPU节点、混合节点）
- ✅ 4 个示例用户（admin/operator/user/viewer）
- ✅ 3 个示例租户（admin/team1/team2）

#### 启动前端
```bash
cd unified-resource-manager/web/frontend
npm install
npm run dev
```

前端将在 `http://localhost:3000` 启动

---

### 2️⃣ 功能验证清单

#### ✅ 节点 & 设备发现

1. **访问节点管理页面**
   - 打开 http://localhost:3000
   - 点击左侧菜单 "节点管理"
   - 应该看到 3 个示例节点

2. **查看节点详情**
   - 点击节点行的展开按钮
   - 查看 GPU/NPU 设备列表

3. **触发资源发现**
   - 点击 "发现资源" 按钮
   - 查看资源信息更新

4. **API 测试**
```bash
# 列出节点
curl http://localhost:8080/api/v1/nodes

# 触发资源发现
curl -X POST http://localhost:8080/api/v1/nodes/node-gpu-1/discover
```

---

#### ✅ 按规格分配资源

1. **查看资源规格**
   - API: `curl http://localhost:8080/api/v1/flavors`
   - 应该返回 5 个规格

2. **使用规格提交任务**
   - 访问 "任务管理" 页面
   - 点击 "提交任务"
   - 在 "资源规格" 下拉中选择一个规格（如 `gpu.a100.20g.infer`）
   - 系统自动填充：资源类型=GPU, 内存=20GB, 核心数=50
   - 填写任务名称、类型等
   - 提交

3. **API 测试（使用规格）**
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
      "memory_gb": 0,
      "cores": 0,
      "priority": 5,
      "timeout": "3600s",
      "labels": {
        "flavor_id": "gpu.a100.20g.infer"
      }
    }
  }'
```

---

#### ✅ 推理 / 训练分流

1. **前端验证**
   - 访问 "任务管理" 页面
   - 使用顶部 Tab 切换：
     - "全部任务" - 显示所有任务
     - "推理任务" - 只显示推理类型
     - "训练任务" - 只显示训练类型

2. **提交不同类型任务**
   - 提交一个推理任务（type=inference）
   - 提交一个训练任务（type=training）
   - 切换 Tab 验证过滤功能

3. **API 测试**
```bash
# 列出推理任务
curl "http://localhost:8080/api/v1/tasks?type=inference"

# 列出训练任务
curl "http://localhost:8080/api/v1/tasks?type=training"
```

---

#### ✅ 权限 & 多租户

1. **测试权限控制**
```bash
# 无用户ID（默认放行）
curl http://localhost:8080/api/v1/tasks

# 使用用户身份
curl -H "X-User-Id: user-dev1" \
     -H "X-Tenant-Id: tenant-team1" \
     http://localhost:8080/api/v1/tasks
```

2. **测试租户配额**
```bash
# 提交任务（会自动检查配额）
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "X-User-Id: user-dev1" \
  -H "X-Tenant-Id: tenant-team1" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-task",
    "type": "inference",
    "priority": 5,
    "resource_req": {
      "task_id": "task-001",
      "task_type": "inference",
      "resource_type": "gpu",
      "count": 25,
      "memory_gb": 20,
      "cores": 50,
      "priority": 5,
      "timeout": "3600s"
    }
  }'
```

如果超过配额（tenant-team1 只有 20 GPU），会返回：
```json
{
  "error": "tenant quota exceeded"
}
```

---

#### ✅ 模型市场

1. **注册模型**
   - 访问 "模型市场" 页面
   - 点击 "注册模型"
   - 填写：
     - 名称：DeepSeek-R1
     - 版本：v1.0
     - 类型：大语言模型
     - 框架：vLLM
     - 推荐资源规格：gpu.a100.20g.infer
     - 路径：/models/deepseek-r1
   - 提交

2. **在任务中使用模型**
   - 访问 "任务管理" → "提交任务"
   - 在 "选择模型" 下拉中选择刚注册的模型
   - 系统自动填充推荐规格
   - 提交任务

3. **API 测试**
```bash
# 列出模型
curl http://localhost:8080/api/v1/models

# 注册模型
curl -X POST http://localhost:8080/api/v1/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "DeepSeek-R1",
    "version": "v1.0",
    "type": "llm",
    "framework": "vllm",
    "resource_flavor_id": "gpu.a100.20g.infer",
    "path": "/models/deepseek-r1",
    "description": "DeepSeek R1 大语言模型"
  }'
```

---

## 📋 完整工作流程示例

### 场景：部署 LLM 推理服务

**步骤 1：注册模型**
```
1. 访问 "模型市场"
2. 注册模型：DeepSeek-R1，推荐规格 gpu.a100.20g.infer
```

**步骤 2：提交推理任务**
```
1. 访问 "任务管理" → "推理任务" Tab
2. 点击 "提交任务"
3. 选择模型：DeepSeek-R1（自动填充规格）
4. 填写任务名称、优先级
5. 提交
```

**步骤 3：监控任务**
```
1. 在 "推理任务" Tab 查看任务状态
2. 在 "资源池" 页面查看资源分配
3. 在 "监控指标" 页面查看性能
```

---

## 🔧 配置说明

### 默认资源规格

系统启动时自动注册以下规格：

| 规格ID | 名称 | 类型 | 内存 | 核心 | 用途 |
|--------|------|------|------|------|------|
| `gpu.a100.20g.infer` | A100 20GB 推理 | GPU | 20GB | 50 | 推理 |
| `gpu.a100.40g.train` | A100 40GB 训练 | GPU | 40GB | 100 | 训练 |
| `gpu.4090.24g.infer` | RTX 4090 24GB 推理 | GPU | 24GB | 50 | 推理 |
| `npu.910b3.8g.infer` | Ascend 910B3 8GB 推理 | NPU | 8GB | 2 | 推理 |
| `npu.910b3.16g.train` | Ascend 910B3 16GB 训练 | NPU | 16GB | 8 | 训练 |

### 示例用户和租户

**用户**：
- `user-admin` (admin) → tenant-admin
- `user-operator` (operator) → tenant-team1  
- `user-dev1` (user) → tenant-team1
- `user-viewer` (viewer) → tenant-team2

**租户配额**：
- `tenant-admin`: 100 GPU, 50 NPU, 10240GB, 1000任务
- `tenant-team1`: 20 GPU, 10 NPU, 2048GB, 200任务
- `tenant-team2`: 10 GPU, 5 NPU, 1024GB, 100任务

---

## 🎉 所有功能已就绪！

系统现在具备完整的商业化 AI 资源管理能力：
- ✅ 节点和设备发现
- ✅ 资源规格管理
- ✅ 推理/训练分流
- ✅ 权限和多租户
- ✅ 模型市场
- ✅ Web 管理界面

可以开始使用了！🚀
