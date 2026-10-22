import { useEffect, useState } from 'react'
import { Table, Card, Tag, Button, Space, Modal, Form, Input, Select, InputNumber, message, Row, Col, Statistic, Typography, Tabs } from 'antd'
import { PlusOutlined, ReloadOutlined, StopOutlined, ThunderboltOutlined, CheckCircleOutlined, ClockCircleOutlined } from '@ant-design/icons'
import { tasksApi, Task, SubmitTaskRequest } from '../api/tasks'
import { modelsApi, Model } from '../api/models'
import { flavorsApi, ResourceFlavor } from '../api/flavors'
import { projectsApi, Project } from '../api/projects'
import './Tasks.css'

const { Title, Text } = Typography

const Tasks: React.FC = () => {
  const [tasks, setTasks] = useState<Task[]>([])
  const [activeTab, setActiveTab] = useState<string>('all')
  const [loading, setLoading] = useState(false)
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [models, setModels] = useState<Model[]>([])
  const [flavors, setFlavors] = useState<ResourceFlavor[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [form] = Form.useForm()

  useEffect(() => {
    loadTasks()
    loadModels()
    loadFlavors()
    loadProjects()
    const interval = setInterval(loadTasks, 5000)
    return () => clearInterval(interval)
  }, [])

  const loadModels = async () => {
    try {
      const data = await modelsApi.listModels()
      setModels(data)
    } catch (error) {
      console.error('Failed to load models:', error)
    }
  }

  const loadFlavors = async () => {
    try {
      const data = await flavorsApi.listFlavors()
      setFlavors(data)
    } catch (error) {
      console.error('Failed to load flavors:', error)
    }
  }

  const loadProjects = async () => {
    try {
      const data = await projectsApi.listProjects()
      setProjects(data)
    } catch (error) {
      console.error('Failed to load projects:', error)
    }
  }

  const loadTasks = async (type?: 'inference' | 'training') => {
    setLoading(true)
    try {
      const filters: Record<string, string> = {}
      if (type) {
        filters.type = type
      }
      const data = await tasksApi.listTasks(filters)
      setTasks(Array.isArray(data) ? data : [])
    } catch (error: any) {
      message.error('加载任务失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleSubmitTask = async (values: any) => {
    try {
      // 如果选择了模型，从模型中获取推荐规格
      let resourceType = values.resource_type
      let memoryGB = values.memory_gb
      let cores = values.cores
      let flavorId = values.flavor_id
      const labels: Record<string, string> = {}
      
      // 关联项目
      if (values.project_id) {
        labels.project_id = values.project_id
      }
      
      // 关联模型
      if (values.model_id) {
        const selectedModel = models.find(m => m.id === values.model_id)
        if (selectedModel) {
          labels.model_id = selectedModel.id
          // 如果模型有推荐规格，使用模型的规格
          if (selectedModel.resource_flavor_id && !flavorId) {
            flavorId = selectedModel.resource_flavor_id
          }
        }
      }
      
      // 如果选择了规格，从规格中获取资源信息
      if (flavorId) {
        const selectedFlavor = flavors.find(f => f.id === flavorId)
        if (selectedFlavor) {
          resourceType = selectedFlavor.type
          memoryGB = selectedFlavor.memory_gb
          cores = selectedFlavor.cores
          labels.flavor_id = flavorId
        }
      }
      
      if (values.placement_policy) {
        labels.placement_policy = values.placement_policy
      }
      const request: SubmitTaskRequest = {
        name: values.name,
        type: values.type,
        priority: values.priority,
        inference_backend: values.inference_backend || undefined,
        queue: values.queue || undefined,
        gang_group: values.gang_group || undefined,
        min_member: values.min_member || undefined,
        min_count: values.min_count || undefined,
        expected_duration_seconds: values.expected_duration_seconds || undefined,
        resource_req: {
          task_id: `task-${Date.now()}`,
          task_type: values.type,
          resource_type: resourceType,
          share_mode: values.share_mode || undefined,
          mig_profile: values.mig_profile || undefined,
          count: values.count || 1,
          memory_gb: memoryGB || values.memory_gb || 20,
          cores: cores || values.cores || 50,
          priority: values.priority,
          timeout: `${values.timeout || 3600}s`,
          labels: labels,
        },
        labels: labels,
      }
      await tasksApi.submitTask(request)
      message.success('任务提交成功')
      setCreateModalVisible(false)
      form.resetFields()
      loadTasks()
    } catch (error: any) {
      message.error('提交任务失败: ' + error.message)
    }
  }

  const handleCancelTask = async (taskId: string) => {
    try {
      await tasksApi.cancelTask(taskId)
      message.success('任务已取消')
      loadTasks()
    } catch (error: any) {
      message.error('取消任务失败: ' + error.message)
    }
  }

  const columns = [
    {
      title: '任务ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '任务名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => (
        <Tag color={type === 'inference' ? 'blue' : 'green'}>{type}</Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          running: 'success',
          pending: 'processing',
          scheduled: 'default',
          completed: 'default',
          failed: 'error',
          cancelled: 'warning',
        }
        const textMap: Record<string, string> = {
          running: '运行中',
          pending: '待处理',
          scheduled: '已调度',
          completed: '已完成',
          failed: '失败',
          cancelled: '已取消',
        }
        return <Tag color={colorMap[status]}>{textMap[status] || status}</Tag>
      },
    },
    {
      title: '队列',
      dataIndex: 'queue',
      key: 'queue',
      render: (v: string) => v || 'default',
    },
    {
      title: '落位',
      dataIndex: 'nodes',
      key: 'nodes',
      render: (nodes: string[]) => (nodes && nodes.length ? nodes.join(',') : '-'),
    },
    {
      title: 'Gang',
      dataIndex: 'gang_group',
      key: 'gang_group',
      render: (v: string) => v || '-',
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
    },
    {
      title: '推理后端',
      dataIndex: 'inference_backend',
      key: 'inference_backend',
      width: 110,
      render: (v: string) => (v === 'nvidia_nim' ? <Tag color="green">NIM</Tag> : v ? <Tag>{v}</Tag> : '-'),
    },
    {
      title: '资源类型',
      dataIndex: ['resource_req', 'resource_type'],
      key: 'resource_type',
      render: (type: string) => <Tag>{(type || 'gpu').toUpperCase()}</Tag>,
    },
    {
      title: '资源数量',
      dataIndex: ['resource_req', 'count'],
      key: 'count',
    },
    {
      title: '内存 (GB)',
      dataIndex: ['resource_req', 'memory_gb'],
      key: 'memory_gb',
    },
    {
      title: '核心数',
      dataIndex: ['resource_req', 'cores'],
      key: 'cores',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (time: string) => new Date(time).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Task) => (
        <Space>
          {record.status === 'running' || record.status === 'pending' ? (
            <Button
              size="small"
              danger
              icon={<StopOutlined />}
              onClick={() => handleCancelTask(record.id)}
            >
              取消
            </Button>
          ) : null}
        </Space>
      ),
    },
  ]

  const safeTasks = Array.isArray(tasks) ? tasks : []
  const taskStats = {
    total: safeTasks.length,
    running: safeTasks.filter(t => t?.status === 'running').length,
    pending: safeTasks.filter(t => t?.status === 'pending').length,
    completed: safeTasks.filter(t => t?.status === 'completed').length,
    failed: safeTasks.filter(t => t?.status === 'failed').length,
  }

  return (
    <div className="tasks-container">
      <div className="tasks-header">
        <Title level={2} style={{ margin: 0 }}>
          <ThunderboltOutlined /> 任务管理
        </Title>
        <Text type="secondary">管理和监控推理与训练任务</Text>
      </div>

      {/* 任务统计 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="总任务数"
              value={taskStats.total}
              suffix="个"
              prefix={<ThunderboltOutlined style={{ color: '#1890ff' }} />}
              valueStyle={{ color: '#1890ff', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="运行中"
              value={taskStats.running}
              suffix="个"
              prefix={<CheckCircleOutlined style={{ color: '#52c41a' }} />}
              valueStyle={{ color: '#52c41a', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="待处理"
              value={taskStats.pending}
              suffix="个"
              prefix={<ClockCircleOutlined style={{ color: '#faad14' }} />}
              valueStyle={{ color: '#faad14', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="已完成"
              value={taskStats.completed}
              suffix="个"
              valueStyle={{ color: '#722ed1', fontSize: 28 }}
            />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                失败: {taskStats.failed} 个
              </Text>
            </div>
          </Card>
        </Col>
      </Row>

      <Card
        title="任务列表"
        extra={
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => {
                const t = activeTab === 'inference' ? 'inference' : activeTab === 'training' ? 'training' : undefined
                loadTasks(t as any)
              }}
            >
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
              提交任务
            </Button>
          </Space>
        }
        className="tasks-table-card"
        bordered={false}
      >
        <Tabs
          activeKey={activeTab}
          onChange={(key) => {
            setActiveTab(key)
            const t = key === 'inference' ? 'inference' : key === 'training' ? 'training' : undefined
            loadTasks(t as any)
          }}
          items={[
            { key: 'all', label: '全部任务' },
            { key: 'inference', label: '推理任务' },
            { key: 'training', label: '训练任务' },
          ]}
        />
        <Table
          columns={columns}
          dataSource={safeTasks}
          rowKey="id"
          loading={loading}
        />
      </Card>

      <Modal
        title="提交任务"
        open={createModalVisible}
        onOk={() => form.submit()}
        onCancel={() => {
          setCreateModalVisible(false)
          form.resetFields()
        }}
        width={720}
      >
        <Form form={form} onFinish={handleSubmitTask} layout="vertical">
          <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
            <Input placeholder="例如：DeepSeek-R1 推理服务" />
          </Form.Item>
          <Form.Item name="type" label="任务类型" rules={[{ required: true }]}>
            <Select placeholder="选择任务类型">
              <Select.Option value="inference">推理</Select.Option>
              <Select.Option value="training">训练</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="inference_backend" label="推理后端（可选）">
            <Select placeholder="默认 GPU 直连" allowClear>
              <Select.Option value="gpu">GPU 直连</Select.Option>
              <Select.Option value="nvidia_nim">NVIDIA NIM</Select.Option>
            </Select>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
              选 NVIDIA NIM 时请求将转发至已配置的 NIM 服务（需在 系统配置 → NVIDIA NIM 中配置）
            </Text>
          </Form.Item>

          {/* 项目选择 */}
          <Form.Item name="project_id" label="关联项目（可选）">
            <Select
              placeholder="选择项目"
              allowClear
              showSearch
              optionFilterProp="children"
            >
              {projects.map((project) => (
                <Select.Option key={project.id} value={project.id}>
                  {project.name}
                </Select.Option>
              ))}
            </Select>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
              将任务关联到指定项目，便于管理和统计
            </Text>
          </Form.Item>

          {/* 模型选择 */}
          <Form.Item name="model_id" label="选择模型（可选）">
            <Select
              placeholder="从模型市场选择模型"
              allowClear
              showSearch
              optionFilterProp="children"
              onChange={(value) => {
                if (value) {
                  const model = models.find(m => m.id === value)
                  if (model) {
                    // 自动填充推荐规格
                    form.setFieldsValue({
                      flavor_id: model.resource_flavor_id,
                    })
                  }
                }
              }}
            >
              {models.map((model) => (
                <Select.Option key={model.id} value={model.id}>
                  {model.name} ({model.version}) - {model.type}
                </Select.Option>
              ))}
            </Select>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
              选择模型后会自动填充推荐资源规格
            </Text>
          </Form.Item>
          
          {/* 资源规格选择 */}
          <Form.Item name="flavor_id" label="资源规格（可选）">
            <Select
              placeholder="选择资源规格，或手动填写资源信息"
              allowClear
              showSearch
              optionFilterProp="children"
              onChange={(value) => {
                if (value) {
                  const flavor = flavors.find(f => f.id === value)
                  if (flavor) {
                    // 自动填充资源信息
                    form.setFieldsValue({
                      resource_type: flavor.type,
                      memory_gb: flavor.memory_gb,
                      cores: flavor.cores,
                    })
                  }
                }
              }}
            >
              {flavors.map((flavor) => (
                <Select.Option key={flavor.id} value={flavor.id}>
                  {flavor.name} ({flavor.model} - {flavor.memory_gb}GB - {flavor.cores}核心)
                </Select.Option>
              ))}
            </Select>
            <Text type="secondary" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
              选择规格后会自动填充资源类型、内存和核心数
            </Text>
          </Form.Item>
          
          <Form.Item name="resource_type" label="资源类型" rules={[{ required: true }]}>
            <Select placeholder="选择资源类型">
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="count" label="资源数量" rules={[{ required: true }]} initialValue={1}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="memory_gb" label="内存 (GB)" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} placeholder="选择规格后自动填充" />
          </Form.Item>
          <Form.Item name="cores" label="核心数" rules={[{ required: true }]}>
            <InputNumber min={1} max={100} style={{ width: '100%' }} placeholder="选择规格后自动填充" />
          </Form.Item>
          <Form.Item name="share_mode" label="GPU/NPU 使用方式">
            <Select allowClear placeholder="默认按核心数推断：100=整卡，否则 vGPU 切分">
              <Select.Option value="exclusive">整卡独占</Select.Option>
              <Select.Option value="shared">vGPU/vNPU 切分</Select.Option>
              <Select.Option value="mig">MIG 固定切片</Select.Option>
              <Select.Option value="timeslice">分时复用（可超卖）</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="mig_profile" label="MIG 规格" extra="仅 share_mode=mig 时生效，如 1g.10gb">
            <Select allowClear>
              <Select.Option value="1g.10gb">1g.10gb</Select.Option>
              <Select.Option value="2g.20gb">2g.20gb</Select.Option>
              <Select.Option value="3g.40gb">3g.40gb</Select.Option>
              <Select.Option value="7g.80gb">7g.80gb</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="queue" label="调度队列">
            <Select allowClear placeholder="默认 default，也可填 team1 / training">
              <Select.Option value="default">default</Select.Option>
              <Select.Option value="team1">team1</Select.Option>
              <Select.Option value="team2">team2</Select.Option>
              <Select.Option value="training">training</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="gang_group" label="Gang 组（分布式训练）">
            <Input placeholder="同组任务全部可调度后一起启动" />
          </Form.Item>
          <Form.Item name="min_count" label="弹性最少卡数">
            <InputNumber min={0} style={{ width: '100%' }} placeholder="0 表示不降级" />
          </Form.Item>
          <Form.Item name="placement_policy" label="放置策略">
            <Select allowClear placeholder="默认按任务类型">
              <Select.Option value="binpack">Binpack 装箱</Select.Option>
              <Select.Option value="spread">负载均衡</Select.Option>
              <Select.Option value="topology">拓扑优先（NVLink/RDMA）</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="expected_duration_seconds" label="预计运行秒数（演示自动完成）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="priority" label="优先级" initialValue={5}>
            <InputNumber min={1} max={10} style={{ width: '100%' }} />
            <Text type="secondary" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
              1-10，数字越大优先级越高；推理可抢占低优训练
            </Text>
          </Form.Item>
          <Form.Item name="timeout" label="超时时间 (秒)" initialValue={3600}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Tasks
