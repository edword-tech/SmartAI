# Unified Resource Manager 使用指南

## 功能使用说明

### 1. 节点 & 设备发现

#### 后端初始化
系统启动时会自动创建示例节点（GPU节点、NPU节点、混合节点），并设置混合设备发现器。

#### API 使用

**列出所有节点**
```bash
curl http://localhost:8080/api/v1/nodes
```

**获取节点详情**
```bash
curl http://localhost:8080/api/v1/nodes/node-gpu-1
```

**触发节点资源发现**
```bash
curl -X POST http://localhost:8080/api/v1/nodes/node-gpu-1/discover
```

#### 前端使用
1. 访问 "节点管理" 页面
2. 查看节点列表和 GPU/NPU 统计
3. 点击 "发现资源" 按钮触发资源发现
4. 展开节点行查看详细的 GPU/NPU 设备信息

#### 自定义设备发现器
如需对接真实的 GPU-device-plugin 或 flexnpud，可以：
1. 实现 `DeviceDiscoverer` 接口
2. 在 `main.go` 中替换默认发现器：
```go
clusterMgr.SetDeviceDiscoverer(&YourCustomDiscoverer{})
```

---

### 2. 按规格分配资源

#### 后端：资源规格注册
系统启动时自动注册以下默认规格：
- `gpu.a100.20g.infer` - A100 20GB 推理规格
- `gpu.a100.40g.train` - A100 40GB 训练规格
- `gpu.4090.24g.infer` - RTX 4090 24GB 推理规格
- `npu.910b3.8g.infer` - Ascend 910B3 8GB 推理规格
- `npu.910b3.16g.train` - Ascend 910B3 16GB 训练规格

#### API 使用

**获取所有规格**
```bash
curl http://localhost:8080/api/v1/flavors
```

**使用规格提交任务**
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-task",
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

#### 前端使用
1. 在 "任务管理" 页面点击 "提交任务"
2. 在表单中：
   - **选择模型**（可选）：从下拉列表选择已注册的模型，会自动填充推荐规格
   - **选择资源规格**（可选）：从下拉列表选择规格，会自动填充资源类型、内存、核心数
   - 或手动填写资源信息

---

### 3. 推理 / 训练分流管理

#### 后端：调度策略
- **推理任务**：使用 `InferenceStrategy`
  - 优先快速调度
  - 可以使用较小规格资源
  - 可以抢占低优先级训练任务
  
- **训练任务**：使用 `TrainingStrategy`
  - 优先使用整卡资源
  - 减少抢占
  - 需要更高优先级差才能抢占

#### API 使用

**列出推理任务**
```bash
curl "http://localhost:8080/api/v1/tasks?type=inference"
```

**列出训练任务**
```bash
curl "http://localhost:8080/api/v1/tasks?type=training"
```

#### 前端使用
1. 访问 "任务管理" 页面
2. 使用顶部 Tab 切换：
   - **全部任务**：显示所有任务
   - **推理任务**：只显示推理类型任务
   - **训练任务**：只显示训练类型任务
3. 每个 Tab 下会实时刷新对应类型的任务列表

---

### 4. 权限 & 多租户

#### 后端：示例用户和租户
系统启动时自动创建：

**用户**：
- `user-admin` (admin角色) - tenant-admin
- `user-operator` (operator角色) - tenant-team1
- `user-dev1` (user角色) - tenant-team1
- `user-viewer` (viewer角色) - tenant-team2

**租户**：
- `tenant-admin` - 配额：100 GPU, 50 NPU, 10240GB内存, 1000任务
- `tenant-team1` - 配额：20 GPU, 10 NPU, 2048GB内存, 200任务
- `tenant-team2` - 配额：10 GPU, 5 NPU, 1024GB内存, 100任务

#### API 使用（带认证）

**使用用户身份访问**
```bash
curl -H "X-User-Id: user-dev1" \
     -H "X-Tenant-Id: tenant-team1" \
     http://localhost:8080/api/v1/tasks
```

**提交任务（会检查租户配额）**
```bash
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
      "count": 1,
      "memory_gb": 20,
      "cores": 50,
      "priority": 5,
      "timeout": "3600s"
    }
  }'
```

如果超过配额，会返回：
```json
{
  "error": "tenant quota exceeded"
}
```

#### 权限说明
- **admin**: 所有权限
- **operator**: 可以查看、分配、释放资源，提交和取消任务
- **user**: 可以查看资源池，分配资源，提交任务
- **viewer**: 只能查看资源池、任务和指标

---

### 5. 模型市场

#### API 使用

**列出所有模型**
```bash
curl http://localhost:8080/api/v1/models
```

**注册模型**
```bash
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

**删除模型**
```bash
curl -X DELETE http://localhost:8080/api/v1/models/{modelId}
```

#### 前端使用
1. 访问 "模型市场" 页面
2. 点击 "注册模型" 按钮
3. 填写模型信息：
   - 模型名称、版本
   - 类型（llm/cv/asr/embedding）
   - 框架（pytorch/tensorflow/vllm）
   - 推荐资源规格
   - 模型路径
   - 描述
4. 提交后模型会出现在列表中

#### 在任务中使用模型
1. 在 "任务管理" 页面提交任务时
2. 在 "选择模型" 下拉中选择已注册的模型
3. 系统会自动填充该模型的推荐资源规格
4. 任务提交后，`model_id` 会记录在任务的 `labels` 中

---

### 5. NVIDIA NIM 推理后端

系统支持将推理任务路由到 **NVIDIA NIM (Inference Microservices)**。其他模块（GPU/NPU 发现、调度、资源池）保持不变，仅增加 NIM 作为可选推理后端。

#### 配置 NIM
- **API**：`GET /api/v1/nim/config` 获取配置，`PUT /api/v1/nim/config` 更新配置。
- **前端**：系统配置 → 「NVIDIA NIM」Tab，填写 NIM 服务地址 (Base URL)、启用开关、可选凭据 ID 与 namespace，保存。

**示例：更新 NIM 配置**
```bash
curl -X PUT http://localhost:8080/api/v1/nim/config \
  -H "Content-Type: application/json" \
  -d '{
    "base_url": "http://nim-gateway:8000",
    "enabled": true,
    "default_namespace": "nim"
  }'
```

#### 模型使用 NIM
创建或注册模型时指定 `inference_backend: "nvidia_nim"` 和 `nim_model_id`（NIM 侧模型 ID），该模型推理时将走 NIM。

**示例：注册走 NIM 的模型**
```bash
curl -X POST http://localhost:8080/api/v1/models \
  -H "Content-Type: application/json" \
  -d '{
    "id": "llama-nim",
    "name": "Llama-3-NIM",
    "version": "v1",
    "type": "llm",
    "framework": "nvidia_nim",
    "resource_flavor_id": "gpu.a100.20g.infer",
    "path": "",
    "inference_backend": "nvidia_nim",
    "nim_model_id": "meta/llama-3-70b-instruct"
  }'
```

#### 任务使用 NIM
提交推理任务时在请求体中设置 `inference_backend: "nvidia_nim"`，或通过 `labels.inference_backend` 指定，调度与执行侧可根据该字段将请求转发到已配置的 NIM 服务。

**示例：提交走 NIM 的推理任务**
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nim-infer",
    "type": "inference",
    "priority": 5,
    "inference_backend": "nvidia_nim",
    "resource_req": { ... },
    "labels": { "model_id": "llama-nim" }
  }'
```

---

## 完整工作流程示例

### 场景：部署一个 LLM 推理服务

1. **注册模型**
   - 访问 "模型市场"
   - 注册模型：DeepSeek-R1，推荐规格 `gpu.a100.20g.infer`

2. **提交推理任务**
   - 访问 "任务管理" → "推理任务" Tab
   - 点击 "提交任务"
   - 选择模型：DeepSeek-R1
   - 系统自动填充规格：`gpu.a100.20g.infer`
   - 填写任务名称、优先级等
   - 提交

3. **监控任务**
   - 在 "推理任务" Tab 查看任务状态
   - 在 "监控指标" 页面查看资源使用情况

4. **查看资源**
   - 在 "资源池" 页面查看 GPU 资源分配情况
   - 在 "节点管理" 页面查看节点和设备状态

---

## 配置说明

### 环境变量
- `PORT`: API 服务器端口（默认 8080）
- `GPU_DEVICE_PLUGIN_ENDPOINT`: GPU device plugin 端点
- `GPU_EXPORTER_ENDPOINT`: GPU exporter 端点
- `NPU_HYPERVISOR_ENDPOINT`: NPU hypervisor 端点

### 自定义资源规格
在 `cmd/main.go` 的 `initResourceFlavors` 函数中添加更多规格。

### 自定义用户和租户
在 `cmd/main.go` 的 `initRBACAndTenants` 函数中添加更多用户和租户。

---

## 故障排查

### 节点资源发现失败
- 检查节点是否已注册
- 检查设备发现器是否正确配置
- 查看日志中的错误信息

### 任务提交失败
- 检查资源池是否有足够资源
- 检查租户配额是否超限
- 检查用户权限是否足够

### 规格分配失败
- 检查规格是否已注册
- 检查规格的资源类型是否匹配资源池类型
- 检查资源池是否有足够资源
