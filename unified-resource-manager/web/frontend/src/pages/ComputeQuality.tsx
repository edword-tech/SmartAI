import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Modal, Form, Input, InputNumber, Switch, Select, message, Tabs, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined, AppstoreOutlined } from '@ant-design/icons'
import { computeQualityApi, SlicingConfig, ExclusiveConfig, AllocationStrategy } from '../api/compute'
import './ComputeQuality.css'

const { Title, Text } = Typography

const ComputeQuality: React.FC = () => {
  const [activeTab, setActiveTab] = useState<string>('slicing')
  const [slicingConfigs, setSlicingConfigs] = useState<SlicingConfig[]>([])
  const [exclusiveConfigs, setExclusiveConfigs] = useState<ExclusiveConfig[]>([])
  const [allocationStrategies, setAllocationStrategies] = useState<AllocationStrategy[]>([])
  const [slicingVisible, setSlicingVisible] = useState(false)
  const [exclusiveVisible, setExclusiveVisible] = useState(false)
  const [strategyVisible, setStrategyVisible] = useState(false)
  const [slicingForm] = Form.useForm()
  const [exclusiveForm] = Form.useForm()
  const [strategyForm] = Form.useForm()

  useEffect(() => {
    loadAll()
  }, [])

  const loadAll = async () => {
    await Promise.all([
      loadSlicingConfigs(),
      loadExclusiveConfigs(),
      loadAllocationStrategies(),
    ])
  }

  const loadSlicingConfigs = async () => {
    try {
      const data = await computeQualityApi.listSlicingConfigs()
      setSlicingConfigs(Array.isArray(data) ? data : [])
    } catch (error: any) {
      message.error('加载切分配置失败: ' + (error?.message ?? String(error)))
    }
  }

  const loadExclusiveConfigs = async () => {
    try {
      const data = await computeQualityApi.listExclusiveConfigs()
      setExclusiveConfigs(Array.isArray(data) ? data : [])
    } catch (error: any) {
      message.error('加载独占配置失败: ' + (error?.message ?? String(error)))
    }
  }

  const loadAllocationStrategies = async () => {
    try {
      const data = await computeQualityApi.listAllocationStrategies()
      setAllocationStrategies(Array.isArray(data) ? data : [])
    } catch (error: any) {
      message.error('加载分配策略失败: ' + (error?.message ?? String(error)))
    }
  }

  const handleCreateSlicing = async (values: any) => {
    try {
      await computeQualityApi.createSlicingConfig({
        id: `slice-${Date.now()}`,
        name: values.name,
        resource_type: values.resource_type,
        slice_size: {
          memory_gb: values.memory_gb,
          cores: values.cores,
        },
        max_slices: values.max_slices,
        isolation: values.isolation || false,
      })
      message.success('切分配置创建成功')
      setSlicingVisible(false)
      slicingForm.resetFields()
      loadSlicingConfigs()
    } catch (error: any) {
      message.error('创建切分配置失败: ' + error.message)
    }
  }

  const handleCreateExclusive = async (values: any) => {
    try {
      await computeQualityApi.createExclusiveConfig({
        id: `exclusive-${Date.now()}`,
        name: values.name,
        resource_type: values.resource_type,
        priority: values.priority,
        max_duration: values.max_duration,
      })
      message.success('独占配置创建成功')
      setExclusiveVisible(false)
      exclusiveForm.resetFields()
      loadExclusiveConfigs()
    } catch (error: any) {
      message.error('创建独占配置失败: ' + error.message)
    }
  }

  const handleCreateStrategy = async (values: any) => {
    try {
      await computeQualityApi.createAllocationStrategy({
        id: `strategy-${Date.now()}`,
        name: values.name,
        strategy: values.strategy,
        min_quality: values.min_quality,
        weight: values.weight ? JSON.parse(values.weight) : undefined,
      })
      message.success('分配策略创建成功')
      setStrategyVisible(false)
      strategyForm.resetFields()
      loadAllocationStrategies()
    } catch (error: any) {
      message.error('创建分配策略失败: ' + error.message)
    }
  }

  const slicingColumns = [
    {
      title: '配置ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '资源类型',
      dataIndex: 'resource_type',
      key: 'resource_type',
      render: (type: string) => <Tag color={type === 'gpu' ? 'blue' : 'green'}>{type.toUpperCase()}</Tag>,
    },
    {
      title: '切分大小',
      key: 'slice_size',
      render: (_: any, record: SlicingConfig) => (
        <span>{record.slice_size.memory_gb}GB / {record.slice_size.cores}核心</span>
      ),
    },
    {
      title: '最大切分数',
      dataIndex: 'max_slices',
      key: 'max_slices',
    },
    {
      title: '隔离',
      dataIndex: 'isolation',
      key: 'isolation',
      render: (isolation: boolean) => <Tag color={isolation ? 'green' : 'default'}>{isolation ? '是' : '否'}</Tag>,
    },
  ]

  const exclusiveColumns = [
    {
      title: '配置ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '资源类型',
      dataIndex: 'resource_type',
      key: 'resource_type',
      render: (type: string) => <Tag color={type === 'gpu' ? 'blue' : 'green'}>{type.toUpperCase()}</Tag>,
    },
    {
      title: '优先级阈值',
      dataIndex: 'priority',
      key: 'priority',
    },
    {
      title: '最大独占时长',
      dataIndex: 'max_duration',
      key: 'max_duration',
    },
  ]

  const strategyColumns = [
    {
      title: '策略ID',
      dataIndex: 'id',
      key: 'id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '策略类型',
      dataIndex: 'strategy',
      key: 'strategy',
      render: (strategy: string) => {
        const colorMap: Record<string, string> = {
          binpack: 'blue',
          spread: 'green',
          fair: 'orange',
        }
        return <Tag color={colorMap[strategy]}>{strategy}</Tag>
      },
    },
    {
      title: '最低质量',
      dataIndex: 'min_quality',
      key: 'min_quality',
      render: (quality: number) => `${(quality * 100).toFixed(1)}%`,
    },
  ]

  return (
    <div className="compute-quality-container">
      <div className="compute-quality-header">
        <Title level={2} style={{ margin: 0 }}>
          <AppstoreOutlined /> 算力质量
        </Title>
        <Text type="secondary">管理和控制算力资源的服务质量</Text>
      </div>

      <Card className="quality-tabs-card" bordered={false}>
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            {
              key: 'slicing',
              label: '切分配置',
              children: (
                <Card
                  title="资源切分配置"
                  extra={
                    <Space>
                      <Button icon={<ReloadOutlined />} onClick={loadSlicingConfigs}>
                        刷新
                      </Button>
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setSlicingVisible(true)}>
                        创建切分配置
                      </Button>
                    </Space>
                  }
                  bordered={false}
                >
                  <Table
                    columns={slicingColumns}
                    dataSource={slicingConfigs}
                    rowKey="id"
                    locale={{ emptyText: '暂无切分配置' }}
                  />
                </Card>
              ),
            },
            {
              key: 'exclusive',
              label: '独占配置',
              children: (
                <Card
                  title="资源独占配置"
                  extra={
                    <Space>
                      <Button icon={<ReloadOutlined />} onClick={loadExclusiveConfigs}>
                        刷新
                      </Button>
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setExclusiveVisible(true)}>
                        创建独占配置
                      </Button>
                    </Space>
                  }
                  bordered={false}
                >
                  <Table
                    columns={exclusiveColumns}
                    dataSource={exclusiveConfigs}
                    rowKey="id"
                    locale={{ emptyText: '暂无独占配置' }}
                  />
                </Card>
              ),
            },
            {
              key: 'strategy',
              label: '分配策略',
              children: (
                <Card
                  title="资源分配策略"
                  extra={
                    <Space>
                      <Button icon={<ReloadOutlined />} onClick={loadAllocationStrategies}>
                        刷新
                      </Button>
                      <Button type="primary" icon={<PlusOutlined />} onClick={() => setStrategyVisible(true)}>
                        创建分配策略
                      </Button>
                    </Space>
                  }
                  bordered={false}
                >
                  <Table
                    columns={strategyColumns}
                    dataSource={allocationStrategies}
                    rowKey="id"
                    locale={{ emptyText: '暂无分配策略' }}
                  />
                </Card>
              ),
            },
          ]}
        />
      </Card>

      {/* 创建切分配置弹窗 */}
      <Modal
        title="创建切分配置"
        open={slicingVisible}
        onOk={() => slicingForm.submit()}
        onCancel={() => {
          setSlicingVisible(false)
          slicingForm.resetFields()
        }}
        width={600}
      >
        <Form form={slicingForm} onFinish={handleCreateSlicing} layout="vertical">
          <Form.Item name="name" label="配置名称" rules={[{ required: true }]}>
            <Input placeholder="例如：GPU 20GB 切分" />
          </Form.Item>
          <Form.Item name="resource_type" label="资源类型" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="memory_gb" label="内存大小 (GB)" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="cores" label="核心数" rules={[{ required: true }]}>
            <InputNumber min={1} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="max_slices" label="最大切分数" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="isolation" label="隔离" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      {/* 创建独占配置弹窗 */}
      <Modal
        title="创建独占配置"
        open={exclusiveVisible}
        onOk={() => exclusiveForm.submit()}
        onCancel={() => {
          setExclusiveVisible(false)
          exclusiveForm.resetFields()
        }}
        width={600}
      >
        <Form form={exclusiveForm} onFinish={handleCreateExclusive} layout="vertical">
          <Form.Item name="name" label="配置名称" rules={[{ required: true }]}>
            <Input placeholder="例如：高优先级独占" />
          </Form.Item>
          <Form.Item name="resource_type" label="资源类型" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="priority" label="优先级阈值" rules={[{ required: true }]}>
            <InputNumber min={1} max={10} style={{ width: '100%' }} />
            <Text type="secondary" style={{ fontSize: 12 }}>只有优先级 {'>='} 此值的任务才能独占资源</Text>
          </Form.Item>
          <Form.Item name="max_duration" label="最大独占时长" rules={[{ required: true }]}>
            <Input placeholder="例如：24h" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 创建分配策略弹窗 */}
      <Modal
        title="创建分配策略"
        open={strategyVisible}
        onOk={() => strategyForm.submit()}
        onCancel={() => {
          setStrategyVisible(false)
          strategyForm.resetFields()
        }}
        width={600}
      >
        <Form form={strategyForm} onFinish={handleCreateStrategy} layout="vertical">
          <Form.Item name="name" label="策略名称" rules={[{ required: true }]}>
            <Input placeholder="例如：高质量分配策略" />
          </Form.Item>
          <Form.Item name="strategy" label="策略类型" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="binpack">Binpack（装箱）</Select.Option>
              <Select.Option value="spread">Spread（分散）</Select.Option>
              <Select.Option value="fair">Fair（公平）</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="min_quality" label="最低质量要求" rules={[{ required: true }]}>
            <InputNumber min={0} max={1} step={0.1} style={{ width: '100%' }} />
            <Text type="secondary" style={{ fontSize: 12 }}>0-1之间，例如 0.8 表示80%</Text>
          </Form.Item>
          <Form.Item name="weight" label="资源权重（JSON格式，可选）">
            <Input.TextArea rows={3} placeholder='{"gpu": 1.0, "npu": 0.8}' />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default ComputeQuality
