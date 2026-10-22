import { useEffect, useState } from 'react'
import { Table, Card, Tag, Button, Space, Modal, Form, Input, Select, InputNumber, message, Row, Col, Statistic, Progress, Typography, Tooltip, Drawer, Tabs } from 'antd'
import { PlusOutlined, ReloadOutlined, EyeOutlined, DatabaseOutlined, ThunderboltOutlined, ControlOutlined, RocketOutlined } from '@ant-design/icons'
import { poolsApi, PoolInfo, Resource, AllocationRecord, PoolSlot, DeploymentRecord } from '../api/pools'
import { computeQualityApi, SlicingConfig } from '../api/compute'
import { modelsApi } from '../api/models'
import ResourceDetailModal from '../components/ResourceDetailModal'
import './ResourcePools.css'

const { Title, Text } = Typography

const ResourcePools: React.FC = () => {
  const [pools, setPools] = useState<PoolInfo[]>([])
  const [resources, setResources] = useState<Record<string, Resource[]>>({})
  const [loading, setLoading] = useState(false)
  const [createModalVisible, setCreateModalVisible] = useState(false)
  const [detailModalVisible, setDetailModalVisible] = useState(false)
  const [selectedResource, setSelectedResource] = useState<Resource | null>(null)
  const [form] = Form.useForm()

  const [managePoolId, setManagePoolId] = useState<string | null>(null)
  const [poolAllocations, setPoolAllocations] = useState<AllocationRecord[]>([])
  const [poolSlots, setPoolSlots] = useState<PoolSlot[]>([])
  const [sliceConfigId, setSliceConfigId] = useState<string>('')
  const [slicingConfigs, setSlicingConfigs] = useState<SlicingConfig[]>([])
  const [models, setModels] = useState<{ id: string; name: string }[]>([])
  const [deployments, setDeployments] = useState<DeploymentRecord[]>([])
  const [deployForm] = Form.useForm()
  const [deploying, setDeploying] = useState(false)

  useEffect(() => {
    loadPools()
    const interval = setInterval(loadPools, 10000)
    return () => clearInterval(interval)
  }, [])

  const loadPools = async () => {
    setLoading(true)
    try {
      const data = await poolsApi.listPools()
      const list = Array.isArray(data) ? data : []
      setPools(list)
      for (const pool of list) {
        loadResources(pool.id)
      }
    } catch (error: any) {
      message.error('加载资源池失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const loadResources = async (poolId: string) => {
    try {
      const data = await poolsApi.listResources(poolId)
      setResources((prev) => ({ ...prev, [poolId]: Array.isArray(data) ? data : [] }))
    } catch (error) {
      console.error('Failed to load resources:', error)
    }
  }

  const handleCreatePool = async (values: {
    name: string
    type: 'gpu' | 'npu'
    nodes?: string[]
    purpose?: string
    gpu_model?: string
  }) => {
    try {
      const labels: Record<string, string> = {}
      if (values.purpose) labels.purpose = values.purpose
      if (values.gpu_model) labels.gpu_model = values.gpu_model
      const res = await poolsApi.createPool({
        name: values.name,
        type: values.type,
        nodes: Array.isArray(values.nodes) ? values.nodes : [],
        labels: Object.keys(labels).length > 0 ? labels : undefined,
      })
      message.success(res.pool_id ? `资源池创建成功，池 ID: ${res.pool_id}` : '资源池创建成功')
      setCreateModalVisible(false)
      form.resetFields()
      loadPools()
    } catch (error: any) {
      message.error('创建资源池失败: ' + (error?.message ?? String(error)))
    }
  }

  const handleViewResource = async (poolId: string, resourceId: string) => {
    try {
      const resource = await poolsApi.getResource(poolId, resourceId)
      setSelectedResource(resource)
      setDetailModalVisible(true)
    } catch (error: any) {
      message.error('获取资源详情失败: ' + error.message)
    }
  }

  const openManageDrawer = async (poolId: string) => {
    setManagePoolId(poolId)
    setPoolAllocations([])
    setPoolSlots([])
    setDeployments([])
    try {
      const [allocs, slots, depls, configs, modelList] = await Promise.all([
        poolsApi.listPoolAllocations(poolId),
        poolsApi.listPoolSlots(poolId),
        poolsApi.listDeployments(poolId),
        computeQualityApi.listSlicingConfigs().catch(() => []),
        modelsApi.listModels().catch(() => []),
      ])
      setPoolAllocations(Array.isArray(allocs) ? allocs : [])
      setPoolSlots(Array.isArray(slots) ? slots : [])
      setDeployments(Array.isArray(depls) ? depls : [])
      setSlicingConfigs(Array.isArray(configs) ? configs : [])
      setModels(Array.isArray(modelList) ? modelList.map((m: any) => ({ id: m.id, name: m.name || m.id })) : [])
    } catch (e: any) {
      message.error('加载管理数据失败: ' + (e?.message ?? String(e)))
    }
  }

  const loadSlotsWithSlice = async (sliceId?: string) => {
    if (!managePoolId) return
    const sid = sliceId !== undefined ? sliceId : sliceConfigId
    try {
      const slots = await poolsApi.listPoolSlots(managePoolId, sid || undefined)
      setPoolSlots(Array.isArray(slots) ? slots : [])
    } catch {
      setPoolSlots([])
    }
  }

  const handleDeployModel = async (values: any) => {
    if (!managePoolId) return
    setDeploying(true)
    try {
      await poolsApi.deployModelOnPool(managePoolId, {
        model_id: values.model_id,
        resource_req: {
          task_id: `deploy-${values.model_id}-${Date.now()}`,
          task_type: 'inference',
          resource_type: managePoolId.startsWith('npu') ? 'npu' : 'gpu',
          count: values.count ?? 1,
          memory_gb: values.memory_gb,
          cores: values.cores ?? 100,
          priority: 5,
        },
      })
      message.success('部署成功')
      deployForm.resetFields()
      const depls = await poolsApi.listDeployments(managePoolId)
      setDeployments(Array.isArray(depls) ? depls : [])
      loadPools()
    } catch (e: any) {
      message.error('部署失败: ' + (e?.message ?? String(e)))
    } finally {
      setDeploying(false)
    }
  }

  const managePool = managePoolId ? pools.find((p) => p.id === managePoolId) : null

  const poolColumns = [
    {
      title: '资源池名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: PoolInfo) => (
        <Space>
          <DatabaseOutlined style={{ color: record.type === 'gpu' ? '#1890ff' : '#52c41a' }} />
          <Text strong>{text}</Text>
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => (
        <Tag color={type === 'gpu' ? 'blue' : 'green'} style={{ fontSize: 12, padding: '2px 8px' }}>
          {type.toUpperCase()}
        </Tag>
      ),
    },
    {
      title: '用途 / 型号',
      key: 'labels',
      width: 160,
      render: (_: any, record: PoolInfo) => {
        const labels = record.labels || {}
        const purpose = labels.purpose
        const gpuModel = labels.gpu_model
        if (!purpose && !gpuModel) return <Text type="secondary">-</Text>
        return (
          <Space size={4} wrap>
            {purpose && (
              <Tag color={purpose === 'training' ? 'blue' : purpose === 'inference' ? 'green' : 'orange'}>
                {purpose === 'training' ? '训练' : purpose === 'inference' ? '推理' : purpose === 'dev' ? '开发' : purpose}
              </Tag>
            )}
            {gpuModel && <Tag>{gpuModel}</Tag>}
          </Space>
        )
      },
    },
    {
      title: '资源统计',
      key: 'resources',
      width: 200,
      render: (_: any, record: PoolInfo) => {
        const utilization = record.total_count > 0
          ? (record.allocated_count / record.total_count) * 100
          : 0
        return (
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>总数</Text>
              <Text strong>{record.total_count}</Text>
            </div>
            <Progress
              percent={Math.round(utilization)}
              size="small"
              strokeColor={utilization > 80 ? '#ff4d4f' : utilization > 60 ? '#faad14' : '#52c41a'}
              showInfo={false}
            />
            <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                可用: <Text style={{ color: '#52c41a' }}>{record.available_count}</Text>
              </Text>
              <Text type="secondary" style={{ fontSize: 12 }}>
                已分配: <Text style={{ color: '#1890ff' }}>{record.allocated_count}</Text>
              </Text>
            </div>
          </div>
        )
      },
    },
    {
      title: '内存使用',
      key: 'memory',
      width: 200,
      render: (_: any, record: PoolInfo) => {
        const memoryUtil = record.total_memory > 0
          ? (record.used_memory / record.total_memory) * 100
          : 0
        return (
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>总内存</Text>
              <Text strong>{record.total_memory} GB</Text>
            </div>
            <Progress
              percent={Math.round(memoryUtil)}
              size="small"
              strokeColor={memoryUtil > 80 ? '#ff4d4f' : memoryUtil > 60 ? '#faad14' : '#52c41a'}
              format={() => `${record.used_memory}/${record.total_memory} GB`}
            />
          </div>
        )
      },
    },
    {
      title: '节点数',
      dataIndex: 'nodes',
      key: 'nodes',
      width: 100,
      align: 'center' as const,
      render: (nodes: string[]) => (
        <Tag color="default">{(nodes || []).length}</Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 220,
      render: (_: any, record: PoolInfo) => (
        <Space>
          <Tooltip title="池内分配/槽位/部署大模型">
            <Button
              size="small"
              icon={<ControlOutlined />}
              onClick={() => openManageDrawer(record.id)}
            >
              管理
            </Button>
          </Tooltip>
          <Tooltip title="刷新资源">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => loadResources(record.id)} />
          </Tooltip>
          <Tooltip title="查看资源">
            <Button size="small" type="primary" icon={<EyeOutlined />} onClick={() => loadResources(record.id)} />
          </Tooltip>
        </Space>
      ),
    },
  ]

  const resourceColumns = [
    {
      title: '资源ID',
      dataIndex: 'id',
      key: 'id',
      width: 150,
      render: (text: string) => <Text code style={{ fontSize: 12 }}>{text}</Text>,
    },
    {
      title: '节点',
      dataIndex: 'node_id',
      key: 'node_id',
      width: 120,
    },
    {
      title: '设备ID',
      dataIndex: 'device_id',
      key: 'device_id',
      width: 100,
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      width: 100,
      render: (state: string) => {
        const colorMap: Record<string, string> = {
          available: 'success',
          allocated: 'processing',
          maintenance: 'warning',
          error: 'error',
        }
        const textMap: Record<string, string> = {
          available: '可用',
          allocated: '已分配',
          maintenance: '维护中',
          error: '错误',
        }
        return <Tag color={colorMap[state]}>{textMap[state]}</Tag>
      },
    },
    {
      title: '内存',
      key: 'memory',
      width: 150,
      render: (_: any, record: Resource) => {
        const memoryUtil = record.total_memory > 0
          ? (record.used_memory / record.total_memory) * 100
          : 0
        return (
          <div>
            <Text style={{ fontSize: 12 }}>
              {record.used_memory}/{record.total_memory} GB
            </Text>
            <Progress
              percent={Math.round(memoryUtil)}
              size="small"
              strokeColor="#1890ff"
              showInfo={false}
              style={{ marginTop: 4 }}
            />
          </div>
        )
      },
    },
    {
      title: '核心数',
      key: 'cores',
      width: 120,
      render: (_: any, record: Resource) => {
        const coresUtil = record.total_cores > 0
          ? (record.used_cores / record.total_cores) * 100
          : 0
        return (
          <div>
            <Text style={{ fontSize: 12 }}>
              {record.used_cores}/{record.total_cores}
            </Text>
            <Progress
              percent={Math.round(coresUtil)}
              size="small"
              strokeColor="#52c41a"
              showInfo={false}
              style={{ marginTop: 4 }}
            />
          </div>
        )
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_: any, _record: Resource) => <span>-</span>,
    },
  ]

  const totalStats = {
    pools: pools.length,
    totalResources: pools.reduce((sum, p) => sum + p.total_count, 0),
    availableResources: pools.reduce((sum, p) => sum + p.available_count, 0),
    totalMemory: pools.reduce((sum, p) => sum + p.total_memory, 0),
    usedMemory: pools.reduce((sum, p) => sum + p.used_memory, 0),
  }

  return (
    <div className="resource-pools-container">
      <div className="pools-header">
        <Title level={2} style={{ margin: 0 }}>
          <DatabaseOutlined /> 资源池管理
        </Title>
        <Text type="secondary">可切分、可组合、可调度的 GPU/NPU 资源池</Text>
      </div>

      {/* 统计卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="资源池总数"
              value={totalStats.pools}
              suffix="个"
              prefix={<DatabaseOutlined style={{ color: '#1890ff' }} />}
              valueStyle={{ color: '#1890ff', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="总资源数"
              value={totalStats.totalResources}
              suffix="个"
              prefix={<ThunderboltOutlined style={{ color: '#52c41a' }} />}
              valueStyle={{ color: '#52c41a', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="可用资源"
              value={totalStats.availableResources}
              suffix="个"
              valueStyle={{ color: '#faad14', fontSize: 28 }}
            />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                利用率: {totalStats.totalResources > 0
                  ? ((totalStats.totalResources - totalStats.availableResources) / totalStats.totalResources * 100).toFixed(1)
                  : 0}%
              </Text>
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="总内存"
              value={totalStats.totalMemory}
              suffix="GB"
              valueStyle={{ color: '#722ed1', fontSize: 28 }}
            />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary" style={{ fontSize: 12 }}>
                已用: {totalStats.usedMemory} GB
              </Text>
            </div>
          </Card>
        </Col>
      </Row>

      <Card
        title="资源池列表"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadPools}>
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
              创建资源池
            </Button>
          </Space>
        }
        className="pools-table-card"
        bordered={false}
      >
        <Table
          columns={poolColumns}
          dataSource={pools}
          rowKey="id"
          loading={loading}
          expandable={{
            expandedRowRender: (record) => {
              const poolResources = resources[record.id] || []
              const columnsWithPoolId = resourceColumns.map(col => {
                if (col.key === 'action') {
                  return {
                    ...col,
                    render: (_: any, r: Resource) => (
                      <Button
                        size="small"
                        type="link"
                        icon={<EyeOutlined />}
                        onClick={() => handleViewResource(record.id, r.id)}
                      >
                        详情
                      </Button>
                    ),
                  }
                }
                return col
              })
              return (
                <div style={{ padding: '16px 0' }}>
                  <Title level={5} style={{ marginBottom: 16 }}>
                    资源列表 ({poolResources.length} 个)
                  </Title>
                  <Table
                    columns={columnsWithPoolId}
                    dataSource={poolResources}
                    rowKey="id"
                    size="small"
                    pagination={false}
                  />
                </div>
              )
            },
            rowExpandable: () => true,
          }}
        />
      </Card>

      <Modal
        title="创建资源池"
        open={createModalVisible}
        onOk={() => form.submit()}
        onCancel={() => {
          setCreateModalVisible(false)
          form.resetFields()
        }}
        width={600}
      >
        <Form form={form} onFinish={handleCreatePool} layout="vertical">
          <Form.Item name="name" label="资源池名称" rules={[{ required: true }]}>
            <Input placeholder="如：H100 训练池" />
          </Form.Item>
          <Form.Item name="type" label="资源类型" rules={[{ required: true }]}>
            <Select placeholder="选择资源类型">
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="purpose" label="用途（池类型）">
            <Select placeholder="可选：Training / Inference / Dev" allowClear>
              <Select.Option value="training">训练 (Training)</Select.Option>
              <Select.Option value="inference">推理 (Inference)</Select.Option>
              <Select.Option value="dev">开发 (Dev)</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="gpu_model" label="GPU 型号">
            <Select placeholder="可选：与架构文档对应" allowClear>
              <Select.Option value="A100">A100</Select.Option>
              <Select.Option value="H100">H100</Select.Option>
              <Select.Option value="H200">H200</Select.Option>
              <Select.Option value="H20">H20</Select.Option>
              <Select.Option value="RTX">RTX / L40S</Select.Option>
              <Select.Option value="Spark">Spark (Dev)</Select.Option>
              <Select.Option value="其他">其他</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="nodes" label="节点列表">
            <Select mode="tags" placeholder="输入节点名称，按回车添加" />
          </Form.Item>
        </Form>
      </Modal>

      <ResourceDetailModal
        visible={detailModalVisible}
        resource={selectedResource}
        onClose={() => {
          setDetailModalVisible(false)
          setSelectedResource(null)
        }}
      />

      <Drawer
        title={managePool ? `池管理：${managePool.name}` : '池管理'}
        placement="right"
        width={640}
        open={!!managePoolId}
        onClose={() => { setManagePoolId(null); setSliceConfigId('') }}
      >
        {managePool && (
          <Tabs
            items={[
              {
                key: 'overview',
                label: '剩余/已分配',
                children: (
                  <div>
                    <Row gutter={16} style={{ marginBottom: 16 }}>
                      <Col span={8}>
                        <Statistic title="总资源" value={managePool.total_count} suffix="卡" />
                      </Col>
                      <Col span={8}>
                        <Statistic title="剩余可分配" value={managePool.available_count} suffix="卡" valueStyle={{ color: '#52c41a' }} />
                      </Col>
                      <Col span={8}>
                        <Statistic title="已分配" value={managePool.allocated_count} suffix="卡" valueStyle={{ color: '#1890ff' }} />
                      </Col>
                    </Row>
                    <Progress
                      percent={managePool.total_count ? Math.round((managePool.allocated_count / managePool.total_count) * 100) : 0}
                      status="active"
                      format={(p) => `已用 ${p}%`}
                    />
                    <Text type="secondary" style={{ display: 'block', marginTop: 8 }}>
                      总内存 {managePool.total_memory} GB，已用 {managePool.used_memory} GB
                    </Text>
                  </div>
                ),
              },
              {
                key: 'allocations',
                label: '已分配列表',
                children: (
                  <Table
                    size="small"
                    dataSource={poolAllocations}
                    rowKey="allocation_id"
                    columns={[
                      { title: '分配ID', dataIndex: 'allocation_id', key: 'allocation_id', ellipsis: true, render: (t: string) => <Text code style={{ fontSize: 11 }}>{t}</Text> },
                      { title: '任务ID', dataIndex: 'task_id', key: 'task_id', ellipsis: true },
                      { title: '资源数', key: 'count', render: (_: any, r: AllocationRecord) => r.resource_ids?.length ?? 0 },
                      { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: (t: string) => t ? new Date(t).toLocaleString('zh-CN') : '-' },
                    ]}
                    pagination={false}
                    locale={{ emptyText: '暂无已分配记录' }}
                  />
                ),
              },
              {
                key: 'slots',
                label: '槽位（GPU 分割）',
                children: (
                  <div>
                    <Space style={{ marginBottom: 12 }}>
                      <span>切分配置：</span>
                      <Select
                        placeholder="不选则按整卡"
                        allowClear
                        style={{ width: 200 }}
                        value={sliceConfigId || undefined}
                        onChange={(v) => { setSliceConfigId(v || ''); loadSlotsWithSlice(v || '') }}
                      >
                        {slicingConfigs.map((c) => (
                          <Select.Option key={c.id} value={c.id}>{c.name} ({c.slice_size?.memory_gb}GB/{c.slice_size?.cores}核)</Select.Option>
                        ))}
                      </Select>
                      <Button size="small" onClick={loadSlotsWithSlice}>刷新槽位</Button>
                    </Space>
                    <Table
                      size="small"
                      dataSource={poolSlots}
                      rowKey="slot_id"
                      columns={[
                        { title: '槽位ID', dataIndex: 'slot_id', key: 'slot_id', ellipsis: true },
                        { title: '节点', dataIndex: 'node_id', key: 'node_id', width: 120 },
                        { title: '显存(GB)', dataIndex: 'memory_gb', key: 'memory_gb', width: 90 },
                        { title: '核心', dataIndex: 'cores', key: 'cores', width: 70 },
                        { title: '状态', dataIndex: 'state', key: 'state', width: 90, render: (s: string) => <Tag color={s === 'available' ? 'green' : 'blue'}>{s === 'available' ? '可用' : s}</Tag> },
                      ]}
                      pagination={{ pageSize: 8 }}
                      locale={{ emptyText: '暂无槽位（可选切分配置后刷新）' }}
                    />
                  </div>
                ),
              },
              {
                key: 'deploy',
                label: '部署大模型',
                children: (
                  <div>
                    <Form form={deployForm} layout="vertical" onFinish={handleDeployModel}>
                      <Form.Item name="model_id" label="模型" rules={[{ required: true }]}>
                        <Select placeholder="选择要部署的模型" showSearch optionFilterProp="label">
                          {models.map((m) => (
                            <Select.Option key={m.id} value={m.id} label={m.name}>{m.name} ({m.id})</Select.Option>
                          ))}
                        </Select>
                      </Form.Item>
                      <Form.Item name="count" label="卡数" initialValue={1}>
                        <InputNumber min={1} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name="memory_gb" label="单卡显存(GB)">
                        <InputNumber min={0} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name="cores" label="核心数(0-100)">
                        <InputNumber min={0} max={100} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item>
                        <Button type="primary" htmlType="submit" loading={deploying} icon={<RocketOutlined />}>
                          部署到当前池
                        </Button>
                      </Form.Item>
                    </Form>
                    <Title level={5} style={{ marginTop: 24 }}>本池部署记录</Title>
                    <Table
                      size="small"
                      dataSource={deployments}
                      rowKey="id"
                      columns={[
                        { title: '部署ID', dataIndex: 'id', key: 'id', ellipsis: true },
                        { title: '模型', dataIndex: 'model_id', key: 'model_id' },
                        { title: '分配ID', dataIndex: 'allocation_id', key: 'allocation_id', ellipsis: true },
                        { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag>{s}</Tag> },
                      ]}
                      pagination={false}
                      locale={{ emptyText: '暂无部署' }}
                    />
                  </div>
                ),
              },
            ]}
          />
        )}
      </Drawer>
    </div>
  )
}

export default ResourcePools
