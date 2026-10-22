import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Modal, Form, Input, InputNumber, Switch, Select, message, Tabs, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined, ThunderboltOutlined, RocketOutlined } from '@ant-design/icons'
import { trainingAccelerationApi, DataLoadingConfig, DistributedTrainingConfig, CheckpointConfig, MemoryOptimizationConfig } from '../api/compute'
import { tasksApi, Task } from '../api/tasks'
import './TrainingAcceleration.css'

const { Title, Text } = Typography

const TrainingAcceleration: React.FC = () => {
  const [activeTab, setActiveTab] = useState<string>('data-loading')
  const [tasks, setTasks] = useState<Task[]>([])
  const [dataLoadingConfigs, setDataLoadingConfigs] = useState<DataLoadingConfig[]>([])
  const [distributedConfigs, setDistributedConfigs] = useState<DistributedTrainingConfig[]>([])
  const [checkpointConfigs, setCheckpointConfigs] = useState<CheckpointConfig[]>([])
  const [memoryConfigs, setMemoryConfigs] = useState<MemoryOptimizationConfig[]>([])
  const [loading, setLoading] = useState(false)
  const [dataLoadingVisible, setDataLoadingVisible] = useState(false)
  const [distributedVisible, setDistributedVisible] = useState(false)
  const [checkpointVisible, setCheckpointVisible] = useState(false)
  const [memoryVisible, setMemoryVisible] = useState(false)
  const [dataLoadingForm] = Form.useForm()
  const [distributedForm] = Form.useForm()
  const [checkpointForm] = Form.useForm()
  const [memoryForm] = Form.useForm()

  useEffect(() => {
    loadTasks()
    loadAllConfigs()
  }, [])

  const loadAllConfigs = async () => {
    setLoading(true)
    try {
      const [dataLoading, distributed, checkpoint, memory] = await Promise.all([
        trainingAccelerationApi.listDataLoadingConfigs(),
        trainingAccelerationApi.listDistributedConfigs(),
        trainingAccelerationApi.listCheckpointConfigs(),
        trainingAccelerationApi.listMemoryOptimizationConfigs(),
      ])
      setDataLoadingConfigs(Array.isArray(dataLoading) ? dataLoading : [])
      setDistributedConfigs(Array.isArray(distributed) ? distributed : [])
      setCheckpointConfigs(Array.isArray(checkpoint) ? checkpoint : [])
      setMemoryConfigs(Array.isArray(memory) ? memory : [])
    } catch (e) {
      console.error('加载加速配置失败:', e)
    } finally {
      setLoading(false)
    }
  }

  const loadTasks = async () => {
    try {
      const data = await tasksApi.listTasks({})
      setTasks(Array.isArray(data) ? data : [])
    } catch (error: any) {
      console.error('加载任务失败:', error)
    }
  }

  const handleOptimizeTask = async (taskId: string) => {
    try {
      await trainingAccelerationApi.optimizeTask(taskId)
      message.success('任务优化成功')
    } catch (error: any) {
      message.error('任务优化失败: ' + error.message)
    }
  }

  const handleCreateDataLoading = async (values: any) => {
    try {
      await trainingAccelerationApi.configureDataLoading({
        id: `data-loading-${Date.now()}`,
        task_id: values.task_id,
        prefetch_size: values.prefetch_size,
        num_workers: values.num_workers,
        pin_memory: values.pin_memory || false,
        async_loading: values.async_loading || false,
        cache_strategy: values.cache_strategy || 'memory',
      })
      message.success('数据加载配置创建成功')
      setDataLoadingVisible(false)
      dataLoadingForm.resetFields()
      loadAllConfigs()
    } catch (error: any) {
      message.error('创建数据加载配置失败: ' + error.message)
    }
  }

  const handleCreateDistributed = async (values: any) => {
    try {
      await trainingAccelerationApi.configureDistributedTraining({
        id: `distributed-${Date.now()}`,
        task_id: values.task_id,
        strategy: values.strategy,
        num_nodes: values.num_nodes,
        num_gpus_per_node: values.num_gpus_per_node,
        communication_backend: values.communication_backend,
        mixed_precision: values.mixed_precision || false,
        gradient_checkpointing: values.gradient_checkpointing || false,
      })
      message.success('分布式训练配置创建成功')
      setDistributedVisible(false)
      distributedForm.resetFields()
      loadAllConfigs()
    } catch (error: any) {
      message.error('创建分布式训练配置失败: ' + error.message)
    }
  }

  const handleCreateCheckpoint = async (values: any) => {
    try {
      await trainingAccelerationApi.configureCheckpoint({
        id: `checkpoint-${Date.now()}`,
        task_id: values.task_id,
        interval: values.interval,
        max_checkpoints: values.max_checkpoints,
        auto_resume: values.auto_resume || false,
        fault_prediction: values.fault_prediction || false,
        storage_path: values.storage_path,
      })
      message.success('检查点配置创建成功')
      setCheckpointVisible(false)
      checkpointForm.resetFields()
      loadAllConfigs()
    } catch (error: any) {
      message.error('创建检查点配置失败: ' + error.message)
    }
  }

  const handleCreateMemory = async (values: any) => {
    try {
      await trainingAccelerationApi.configureMemoryOptimization({
        id: `memory-${Date.now()}`,
        task_id: values.task_id,
        memory_efficient_attention: values.memory_efficient_attention || false,
        gradient_accumulation: values.gradient_accumulation || 1,
        activation_checkpointing: values.activation_checkpointing || false,
        offload_strategy: values.offload_strategy || 'none',
        max_memory_usage: values.max_memory_usage,
      })
      message.success('显存优化配置创建成功')
      setMemoryVisible(false)
      memoryForm.resetFields()
      loadAllConfigs()
    } catch (error: any) {
      message.error('创建显存优化配置失败: ' + error.message)
    }
  }

  const taskColumns = [
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
      render: (type: string) => <Tag color={type === 'inference' ? 'blue' : 'green'}>{type}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          running: 'success',
          pending: 'processing',
          completed: 'default',
          failed: 'error',
        }
        return <Tag color={colorMap[status]}>{status}</Tag>
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Task) => (
        <Button
          size="small"
          icon={<RocketOutlined />}
          onClick={() => handleOptimizeTask(record.id)}
          disabled={record.status !== 'running' && record.status !== 'pending'}
        >
          优化
        </Button>
      ),
    },
  ]

  return (
    <div className="training-acceleration-container">
      <div className="training-acceleration-header">
        <Title level={2} style={{ margin: 0 }}>
          <ThunderboltOutlined /> 训练加速
        </Title>
        <Text type="secondary">优化训练和推理性能</Text>
      </div>

      {/* 功能说明 */}
      <Card size="small" style={{ marginBottom: 24 }} bordered={false}>
        <Title level={5} style={{ marginTop: 0 }}>功能说明</Title>
        <Text type="secondary">
          为训练任务配置数据加载、分布式策略、检查点与显存优化后，在任务上点击「优化」即可将配置应用到该任务；
          配置会写入任务标签，供执行端使用。下方可创建并查看各类配置。
        </Text>
      </Card>

      <Card
        title="任务列表"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => { loadTasks(); loadAllConfigs(); }}>
              刷新
            </Button>
          </Space>
        }
        className="tasks-card"
        bordered={false}
        style={{ marginBottom: 24 }}
      >
        <Table
          columns={taskColumns}
          dataSource={Array.isArray(tasks) ? tasks : []}
          rowKey="id"
          loading={loading}
          locale={{ emptyText: '暂无任务，请先在「任务管理」提交训练任务' }}
        />
      </Card>

      <Card className="acceleration-tabs-card" bordered={false}>
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            {
              key: 'data-loading',
              label: '数据加载优化',
              children: (
                <>
                  <Card
                    title="数据加载优化配置"
                    extra={
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setDataLoadingVisible(true)}>
                        创建配置
                      </Button>
                    }
                    bordered={false}
                  >
                    <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      通过预取、多线程、固定内存、异步加载与缓存策略减少数据读取瓶颈。建议：预取 2～4，工作线程数等于或略小于 CPU 核数，开启固定内存与异步加载。
                    </Text>
                    <Table
                      size="small"
                      rowKey="id"
                      dataSource={dataLoadingConfigs}
                      columns={[
                        { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 180 },
                        { title: '任务ID', dataIndex: 'task_id', key: 'task_id', width: 120 },
                        { title: '预取', dataIndex: 'prefetch_size', key: 'prefetch_size', width: 80 },
                        { title: '工作线程', dataIndex: 'num_workers', key: 'num_workers', width: 90 },
                        { title: '固定内存', dataIndex: 'pin_memory', key: 'pin_memory', width: 90, render: (v: boolean) => v ? '是' : '否' },
                        { title: '缓存策略', dataIndex: 'cache_strategy', key: 'cache_strategy', width: 100 },
                      ]}
                      pagination={false}
                      locale={{ emptyText: '暂无配置，请点击「创建配置」' }}
                    />
                  </Card>
                </>
              ),
            },
            {
              key: 'distributed',
              label: '分布式训练',
              children: (
                <>
                  <Card
                    title="分布式训练配置"
                    extra={
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setDistributedVisible(true)}>
                        创建配置
                      </Button>
                    }
                    bordered={false}
                  >
                    <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      DDP/FSDP/DeepSpeed 等策略与节点数、每节点 GPU 数、通信后端（NCCL/Gloo）、混合精度与梯度检查点。多机时建议使用 NCCL，单机多卡可用 DDP 或 FSDP。
                    </Text>
                    <Table
                      size="small"
                      rowKey="id"
                      dataSource={distributedConfigs}
                      columns={[
                        { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 160 },
                        { title: '任务ID', dataIndex: 'task_id', key: 'task_id', width: 120 },
                        { title: '策略', dataIndex: 'strategy', key: 'strategy', width: 100 },
                        { title: '节点数', dataIndex: 'num_nodes', key: 'num_nodes', width: 80 },
                        { title: '每节点GPU', dataIndex: 'num_gpus_per_node', key: 'num_gpus_per_node', width: 100 },
                        { title: '通信后端', dataIndex: 'communication_backend', key: 'communication_backend', width: 100 },
                        { title: '混合精度', dataIndex: 'mixed_precision', key: 'mixed_precision', width: 90, render: (v: boolean) => v ? '是' : '否' },
                      ]}
                      pagination={false}
                      locale={{ emptyText: '暂无配置，请点击「创建配置」' }}
                    />
                  </Card>
                </>
              ),
            },
            {
              key: 'checkpoint',
              label: '检查点配置',
              children: (
                <>
                  <Card
                    title="检查点配置"
                    extra={
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setCheckpointVisible(true)}>
                        创建配置
                      </Button>
                    }
                    bordered={false}
                  >
                    <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      定期保存检查点、最大保留数量、存储路径、自动恢复与故障预测，便于断点续训与容错。
                    </Text>
                    <Table
                      size="small"
                      rowKey="id"
                      dataSource={checkpointConfigs}
                      columns={[
                        { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 160 },
                        { title: '任务ID', dataIndex: 'task_id', key: 'task_id', width: 120 },
                        { title: '间隔', dataIndex: 'interval', key: 'interval', width: 100, render: (v: string | number) => typeof v === 'number' ? `${v}ns` : String(v) },
                        { title: '最大检查点数', dataIndex: 'max_checkpoints', key: 'max_checkpoints', width: 110 },
                        { title: '存储路径', dataIndex: 'storage_path', key: 'storage_path', ellipsis: true },
                        { title: '自动恢复', dataIndex: 'auto_resume', key: 'auto_resume', width: 90, render: (v: boolean) => v ? '是' : '否' },
                      ]}
                      pagination={false}
                      locale={{ emptyText: '暂无配置，请点击「创建配置」' }}
                    />
                  </Card>
                </>
              ),
            },
            {
              key: 'memory',
              label: '显存优化',
              children: (
                <>
                  <Card
                    title="显存优化配置"
                    extra={
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setMemoryVisible(true)}>
                        创建配置
                      </Button>
                    }
                    bordered={false}
                  >
                    <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                      内存高效注意力、梯度累积、激活检查点、CPU/磁盘卸载与最大显存限制，用于大模型或显存紧张时的训练稳定性。
                    </Text>
                    <Table
                      size="small"
                      rowKey="id"
                      dataSource={memoryConfigs}
                      columns={[
                        { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 160 },
                        { title: '任务ID', dataIndex: 'task_id', key: 'task_id', width: 120 },
                        { title: '最大显存(GB)', dataIndex: 'max_memory_usage', key: 'max_memory_usage', width: 120 },
                        { title: '梯度累积', dataIndex: 'gradient_accumulation', key: 'gradient_accumulation', width: 90 },
                        { title: '卸载策略', dataIndex: 'offload_strategy', key: 'offload_strategy', width: 100 },
                        { title: '内存高效注意力', dataIndex: 'memory_efficient_attention', key: 'memory_efficient_attention', width: 120, render: (v: boolean) => v ? '是' : '否' },
                        { title: '激活检查点', dataIndex: 'activation_checkpointing', key: 'activation_checkpointing', width: 100, render: (v: boolean) => v ? '是' : '否' },
                      ]}
                      pagination={false}
                      locale={{ emptyText: '暂无配置，请点击「创建配置」' }}
                    />
                  </Card>
                </>
              ),
            },
          ]}
        />
      </Card>

      {/* 数据加载配置弹窗 */}
      <Modal
        title="配置数据加载优化"
        open={dataLoadingVisible}
        onOk={() => dataLoadingForm.submit()}
        onCancel={() => {
          setDataLoadingVisible(false)
          dataLoadingForm.resetFields()
        }}
        width={600}
      >
        <Form form={dataLoadingForm} onFinish={handleCreateDataLoading} layout="vertical">
          <Form.Item name="task_id" label="任务ID" rules={[{ required: true }]}>
            <Select showSearch placeholder="选择任务">
              {(Array.isArray(tasks) ? tasks : []).map(task => (
                <Select.Option key={task.id} value={task.id}>{task.name} ({task.id})</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="prefetch_size" label="预取大小" rules={[{ required: true }]} initialValue={4}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="num_workers" label="工作线程数" rules={[{ required: true }]} initialValue={4}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="pin_memory" label="固定内存" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <Form.Item name="async_loading" label="异步加载" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <Form.Item name="cache_strategy" label="缓存策略" initialValue="memory">
            <Select>
              <Select.Option value="memory">内存缓存</Select.Option>
              <Select.Option value="disk">磁盘缓存</Select.Option>
              <Select.Option value="none">无缓存</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      {/* 分布式训练配置弹窗 */}
      <Modal
        title="配置分布式训练"
        open={distributedVisible}
        onOk={() => distributedForm.submit()}
        onCancel={() => {
          setDistributedVisible(false)
          distributedForm.resetFields()
        }}
        width={600}
      >
        <Form form={distributedForm} onFinish={handleCreateDistributed} layout="vertical">
          <Form.Item name="task_id" label="任务ID" rules={[{ required: true }]}>
            <Select showSearch placeholder="选择任务">
              {(Array.isArray(tasks) ? tasks : []).map(task => (
                <Select.Option key={task.id} value={task.id}>{task.name} ({task.id})</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="strategy" label="分布式策略" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="ddp">DDP (DistributedDataParallel)</Select.Option>
              <Select.Option value="fsdp">FSDP (FullyShardedDataParallel)</Select.Option>
              <Select.Option value="deepspeed">DeepSpeed</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="num_nodes" label="节点数" rules={[{ required: true }]} initialValue={1}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="num_gpus_per_node" label="每节点GPU数" rules={[{ required: true }]} initialValue={1}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="communication_backend" label="通信后端" rules={[{ required: true }]} initialValue="nccl">
            <Select>
              <Select.Option value="nccl">NCCL</Select.Option>
              <Select.Option value="gloo">Gloo</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="mixed_precision" label="混合精度" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
          <Form.Item name="gradient_checkpointing" label="梯度检查点" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      {/* 检查点配置弹窗 */}
      <Modal
        title="配置检查点"
        open={checkpointVisible}
        onOk={() => checkpointForm.submit()}
        onCancel={() => {
          setCheckpointVisible(false)
          checkpointForm.resetFields()
        }}
        width={600}
      >
        <Form form={checkpointForm} onFinish={handleCreateCheckpoint} layout="vertical">
          <Form.Item name="task_id" label="任务ID" rules={[{ required: true }]}>
            <Select showSearch placeholder="选择任务">
              {(Array.isArray(tasks) ? tasks : []).map(task => (
                <Select.Option key={task.id} value={task.id}>{task.name} ({task.id})</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="interval" label="检查点间隔" rules={[{ required: true }]} initialValue="1h">
            <Input placeholder="例如：1h, 30m" />
          </Form.Item>
          <Form.Item name="max_checkpoints" label="最大检查点数" rules={[{ required: true }]} initialValue={5}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="storage_path" label="存储路径" rules={[{ required: true }]}>
            <Input placeholder="例如：/checkpoints" />
          </Form.Item>
          <Form.Item name="auto_resume" label="自动恢复" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
          <Form.Item name="fault_prediction" label="故障预测" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      {/* 显存优化配置弹窗 */}
      <Modal
        title="配置显存优化"
        open={memoryVisible}
        onOk={() => memoryForm.submit()}
        onCancel={() => {
          setMemoryVisible(false)
          memoryForm.resetFields()
        }}
        width={600}
      >
        <Form form={memoryForm} onFinish={handleCreateMemory} layout="vertical">
          <Form.Item name="task_id" label="任务ID" rules={[{ required: true }]}>
            <Select showSearch placeholder="选择任务">
              {(Array.isArray(tasks) ? tasks : []).map(task => (
                <Select.Option key={task.id} value={task.id}>{task.name} ({task.id})</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="max_memory_usage" label="最大显存使用 (GB)" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="memory_efficient_attention" label="内存高效注意力" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
          <Form.Item name="gradient_accumulation" label="梯度累积步数" initialValue={1}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="activation_checkpointing" label="激活检查点" valuePropName="checked" initialValue={false}>
            <Switch />
          </Form.Item>
          <Form.Item name="offload_strategy" label="卸载策略" initialValue="none">
            <Select>
              <Select.Option value="none">无</Select.Option>
              <Select.Option value="cpu">CPU卸载</Select.Option>
              <Select.Option value="disk">磁盘卸载</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default TrainingAcceleration
