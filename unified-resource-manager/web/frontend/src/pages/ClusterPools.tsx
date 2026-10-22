import { useEffect, useState } from 'react'
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Modal,
  Form,
  Input,
  Select,
  InputNumber,
  message,
  Tabs,
  Typography,
  Row,
  Col,
  Statistic,
  Progress,
} from 'antd'
import {
  PlusOutlined,
  ReloadOutlined,
  ClusterOutlined,
  ThunderboltOutlined,
  ApartmentOutlined,
} from '@ant-design/icons'
import { clusterPoolsApi } from '../api/clusterPools'
import type {
  ClusterPoolSpec,
  ClusterPoolInfo,
  SubPoolSpec,
  SubPoolUsage,
  AllocateRequest,
} from '../api/clusterPools'
import { nodesApi } from '../api/nodes'
import { poolsApi } from '../api/pools'
import './ClusterPools.css'

const { Title, Text } = Typography

const ClusterPools: React.FC = () => {
  const [activeTab, setActiveTab] = useState('cluster')
  const [clusterPools, setClusterPools] = useState<ClusterPoolSpec[]>([])
  const [clusterPoolInfos, setClusterPoolInfos] = useState<Record<string, ClusterPoolInfo>>({})
  const [subPools, setSubPools] = useState<SubPoolSpec[]>([])
  const [subPoolUsages, setSubPoolUsages] = useState<Record<string, SubPoolUsage>>({})
  const [nodes, setNodes] = useState<{ id: string; name: string }[]>([])
  const [pools, setPools] = useState<{ id: string; name: string }[]>([])
  const [loading, setLoading] = useState(false)
  const [createClusterVisible, setCreateClusterVisible] = useState(false)
  const [createSubVisible, setCreateSubVisible] = useState(false)
  const [allocateClusterId, setAllocateClusterId] = useState<string | null>(null)
  const [allocateSubId, setAllocateSubId] = useState<string | null>(null)
  const [clusterForm] = Form.useForm()
  const [subForm] = Form.useForm()
  const [allocForm] = Form.useForm()

  useEffect(() => {
    loadAll()
  }, [])

  const loadAll = async () => {
    setLoading(true)
    try {
      const [nodeList, poolList] = await Promise.all([
        nodesApi.listNodes().catch(() => []),
        poolsApi.listPools().catch(() => []),
      ])
      setNodes(Array.isArray(nodeList) ? nodeList.map((n: any) => ({ id: n.id, name: n.name || n.id })) : [])
      setPools(Array.isArray(poolList) ? poolList.map((p: any) => ({ id: p.id, name: p.name || p.id })) : [])
      await loadClusterPools()
      await loadSubPools()
    } catch (e) {
      message.error('加载失败: ' + (e as Error)?.message)
    } finally {
      setLoading(false)
    }
  }

  const loadClusterPools = async () => {
    try {
      const data = await clusterPoolsApi.listClusterPools()
      setClusterPools(Array.isArray(data) ? data : [])
      const infos: Record<string, ClusterPoolInfo> = {}
      for (const cp of Array.isArray(data) ? data : []) {
        try {
          const info = await clusterPoolsApi.getClusterPoolInfo(cp.id)
          infos[cp.id] = info
        } catch {
          // ignore
        }
      }
      setClusterPoolInfos(infos)
    } catch (e) {
      console.error(e)
    }
  }

  const loadSubPools = async () => {
    try {
      const data = await clusterPoolsApi.listSubPools()
      setSubPools(Array.isArray(data) ? data : [])
      const usages: Record<string, SubPoolUsage> = {}
      for (const sp of Array.isArray(data) ? data : []) {
        try {
          const u = await clusterPoolsApi.getSubPoolUsage(sp.id)
          usages[sp.id] = u
        } catch {
          // ignore
        }
      }
      setSubPoolUsages(usages)
    } catch (e) {
      console.error(e)
    }
  }

  const handleCreateClusterPool = async (values: any) => {
    try {
      const labels: Record<string, string> = {}
      if (values.purpose) labels.purpose = values.purpose
      if (values.gpu_model) labels.gpu_model = values.gpu_model
      await clusterPoolsApi.createClusterPool({
        name: values.name,
        type: values.type,
        description: values.description,
        node_ids: Array.isArray(values.node_ids) ? values.node_ids : [],
        labels: Object.keys(labels).length > 0 ? labels : undefined,
      })
      message.success('集群大池创建成功')
      setCreateClusterVisible(false)
      clusterForm.resetFields()
      loadClusterPools()
    } catch (e: any) {
      message.error('创建失败: ' + (e?.message ?? String(e)))
    }
  }

  const handleCreateSubPool = async (values: any) => {
    try {
      await clusterPoolsApi.createSubPool({
        name: values.name,
        description: values.description,
        parent_cluster_pool_id: values.parent_cluster_pool_id || undefined,
        parent_pool_id: values.parent_pool_id || undefined,
        quota_max_count: values.quota_max_count,
        quota_max_memory_gb: values.quota_max_memory_gb,
      })
      message.success('子池创建成功')
      setCreateSubVisible(false)
      subForm.resetFields()
      loadSubPools()
    } catch (e: any) {
      message.error('创建失败: ' + (e?.message ?? String(e)))
    }
  }

  const handleAllocate = async (values: any, clusterPoolId?: string, subPoolId?: string) => {
    const req: AllocateRequest = {
      task_id: values.task_id || `task-${Date.now()}`,
      task_type: values.task_type || 'inference',
      resource_type: values.resource_type || 'gpu',
      count: values.count ?? 1,
      memory_gb: values.memory_gb,
      cores: values.cores,
      priority: values.priority ?? 5,
      timeout: values.timeout ? `${values.timeout}s` : '24h',
    }
    try {
      if (subPoolId) {
        await clusterPoolsApi.allocateFromSubPool(subPoolId, req)
        message.success('从子池分配成功')
        setAllocateSubId(null)
        loadSubPools()
      } else if (clusterPoolId) {
        await clusterPoolsApi.allocateFromClusterPool(clusterPoolId, req)
        message.success('从大池分配成功')
        setAllocateClusterId(null)
        loadClusterPools()
      }
      allocForm.resetFields()
    } catch (e: any) {
      message.error('分配失败: ' + (e?.message ?? String(e)))
    }
  }

  const clusterColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (t: string, r: ClusterPoolSpec) => (
        <Space>
          <ClusterOutlined style={{ color: r.type === 'gpu' ? '#1890ff' : '#52c41a' }} />
          <Text strong>{t}</Text>
        </Space>
      ),
    },
    { title: 'ID', dataIndex: 'id', key: 'id', width: 180, ellipsis: true, render: (t: string) => <Text code style={{ fontSize: 12 }}>{t}</Text> },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 80,
      render: (t: string) => <Tag color={t === 'gpu' ? 'blue' : 'green'}>{t.toUpperCase()}</Tag>,
    },
    {
      title: '用途 / 型号',
      key: 'labels',
      width: 140,
      render: (_: any, r: ClusterPoolSpec) => {
        const labels = r.labels || {}
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
      title: '节点数',
      key: 'nodes',
      width: 80,
      render: (_: any, r: ClusterPoolSpec) => <Tag>{r.node_ids?.length ?? 0}</Tag>,
    },
    {
      title: '资源统计',
      key: 'stats',
      width: 200,
      render: (_: any, r: ClusterPoolSpec) => {
        const info = clusterPoolInfos[r.id]
        if (!info) return <Text type="secondary">-</Text>
        const util = info.total_count > 0 ? (info.allocated_count / info.total_count) * 100 : 0
        return (
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>
              可用 {info.available_count} / 共 {info.total_count} 卡
            </Text>
            <Progress percent={Math.round(util)} size="small" showInfo={false} style={{ marginTop: 4 }} />
          </div>
        )
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      render: (_: any, record: ClusterPoolSpec) => (
        <Space>
          <Button size="small" type="primary" onClick={() => setAllocateClusterId(record.id)}>
            分配
          </Button>
        </Space>
      ),
    },
  ]

  const subColumns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (t: string) => (
        <Space>
          <ApartmentOutlined />
          <Text strong>{t}</Text>
        </Space>
      ),
    },
    { title: 'ID', dataIndex: 'id', key: 'id', width: 140, ellipsis: true, render: (t: string) => <Text code style={{ fontSize: 12 }}>{t}</Text> },
    {
      title: '父池',
      key: 'parent',
      width: 160,
      render: (_: any, r: SubPoolSpec) => (
        <Text type="secondary">{r.parent_cluster_pool_id || r.parent_pool_id || '-'}</Text>
      ),
    },
    {
      title: '配额',
      key: 'quota',
      width: 120,
      render: (_: any, r: SubPoolSpec) => (
        <span>
          {r.quota_max_count != null ? `最多 ${r.quota_max_count} 卡` : '-'}
          {r.quota_max_memory_gb != null ? ` / ${r.quota_max_memory_gb}GB` : ''}
        </span>
      ),
    },
    {
      title: '已用',
      key: 'usage',
      width: 120,
      render: (_: any, r: SubPoolSpec) => {
        const u = subPoolUsages[r.id]
        if (!u) return <Text type="secondary">-</Text>
        return (
          <span>
            {u.count} 卡 / {u.memory_gb}GB
          </span>
        )
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: any, record: SubPoolSpec) => (
        <Button size="small" type="primary" onClick={() => setAllocateSubId(record.id)}>
          分配
        </Button>
      ),
    },
  ]

  return (
    <div className="cluster-pools-container">
      <div className="cluster-pools-header page-header">
        <Title level={2} style={{ margin: 0 }}>
          <ClusterOutlined /> 集群大池与子池
        </Title>
        <Text type="secondary">多节点 GPU 聚合成大池，可切分、可组合、可调度；大池内可切分子池并配额管理</Text>
      </div>

      <Card bordered={false}>
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          tabBarExtraContent={
            <Button icon={<ReloadOutlined />} onClick={loadAll}>
              刷新
            </Button>
          }
        >
          <Tabs.TabPane tab="集群大池" key="cluster">
            <div style={{ marginBottom: 16 }}>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateClusterVisible(true)}>
                创建大池
              </Button>
            </div>
            <Table
              columns={clusterColumns}
              dataSource={clusterPools}
              rowKey="id"
              loading={loading}
              pagination={{ pageSize: 10 }}
              locale={{ emptyText: '暂无集群大池，可点击「创建大池」添加' }}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab="子池" key="sub">
            <div style={{ marginBottom: 16 }}>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateSubVisible(true)}>
                创建子池
              </Button>
            </div>
            <Table
              columns={subColumns}
              dataSource={subPools}
              rowKey="id"
              loading={loading}
              pagination={{ pageSize: 10 }}
              locale={{ emptyText: '暂无子池，可点击「创建子池」添加' }}
            />
          </Tabs.TabPane>
        </Tabs>
      </Card>

      {/* 创建集群大池 */}
      <Modal
        title="创建集群大池"
        open={createClusterVisible}
        onOk={() => clusterForm.submit()}
        onCancel={() => { setCreateClusterVisible(false); clusterForm.resetFields() }}
        width={560}
      >
        <Form form={clusterForm} layout="vertical" onFinish={handleCreateClusterPool}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="如：H100 训练大池" />
          </Form.Item>
          <Form.Item name="type" label="资源类型" rules={[{ required: true }]} initialValue="gpu">
            <Select>
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="purpose" label="用途（池类型）">
            <Select placeholder="Training / Inference / Dev" allowClear>
              <Select.Option value="training">训练 (Training)</Select.Option>
              <Select.Option value="inference">推理 (Inference)</Select.Option>
              <Select.Option value="dev">开发 (Dev)</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="gpu_model" label="GPU 型号">
            <Select placeholder="可选" allowClear>
              <Select.Option value="A100">A100</Select.Option>
              <Select.Option value="H100">H100</Select.Option>
              <Select.Option value="H200">H200</Select.Option>
              <Select.Option value="RTX">RTX / L40S</Select.Option>
              <Select.Option value="Spark">Spark (Dev)</Select.Option>
              <Select.Option value="其他">其他</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="node_ids" label="纳入节点">
            <Select mode="multiple" placeholder="选择节点（可选多个）" optionFilterProp="label">
              {nodes.map((n) => (
                <Select.Option key={n.id} value={n.id} label={n.name}>
                  {n.name} ({n.id})
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 创建子池 */}
      <Modal
        title="创建子池"
        open={createSubVisible}
        onOk={() => subForm.submit()}
        onCancel={() => { setCreateSubVisible(false); subForm.resetFields() }}
        width={560}
      >
        <Form form={subForm} layout="vertical" onFinish={handleCreateSubPool}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="如：团队A子池" />
          </Form.Item>
          <Form.Item name="parent_cluster_pool_id" label="所属集群大池">
            <Select placeholder="选择大池（与物理池二选一）" allowClear>
              {clusterPools.map((cp) => (
                <Select.Option key={cp.id} value={cp.id}>{cp.name}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="parent_pool_id" label="所属物理池">
            <Select placeholder="选择物理池（与大池二选一）" allowClear>
              {pools.map((p) => (
                <Select.Option key={p.id} value={p.id}>{p.name}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="quota_max_count" label="配额-最大卡数">
            <InputNumber min={0} placeholder="0 表示不限制" style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="quota_max_memory_gb" label="配额-最大显存(GB)">
            <InputNumber min={0} placeholder="0 表示不限制" style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 从大池分配 */}
      <Modal
        title="从大池分配资源"
        open={!!allocateClusterId}
        onOk={() => allocForm.submit()}
        onCancel={() => { setAllocateClusterId(null); allocForm.resetFields() }}
        width={480}
      >
        <Form
          form={allocForm}
          layout="vertical"
          onFinish={(v) => allocateClusterId && handleAllocate(v, allocateClusterId)}
          initialValues={{ count: 1, priority: 5, task_type: 'inference', resource_type: 'gpu' }}
        >
          <Form.Item name="task_id" label="任务ID">
            <Input placeholder="可选，默认自动生成" />
          </Form.Item>
          <Form.Item name="task_type" label="任务类型">
            <Select>
              <Select.Option value="inference">推理</Select.Option>
              <Select.Option value="training">训练</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="resource_type" label="资源类型">
            <Select>
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="count" label="卡数" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="memory_gb" label="单卡显存(GB)">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="cores" label="核心数(0-100)">
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="priority" label="优先级">
            <InputNumber min={1} max={10} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="timeout" label="超时(秒)">
            <InputNumber min={0} placeholder="如 86400" style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 从子池分配 */}
      <Modal
        title="从子池分配资源"
        open={!!allocateSubId}
        onOk={() => allocForm.submit()}
        onCancel={() => { setAllocateSubId(null); allocForm.resetFields() }}
        width={480}
      >
        <Form
          form={allocForm}
          layout="vertical"
          onFinish={(v) => allocateSubId && handleAllocate(v, undefined, allocateSubId)}
          initialValues={{ count: 1, priority: 5, task_type: 'inference', resource_type: 'gpu' }}
        >
          <Form.Item name="task_id" label="任务ID">
            <Input placeholder="可选" />
          </Form.Item>
          <Form.Item name="task_type" label="任务类型">
            <Select>
              <Select.Option value="inference">推理</Select.Option>
              <Select.Option value="training">训练</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="resource_type" label="资源类型">
            <Select>
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="count" label="卡数" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="memory_gb" label="单卡显存(GB)">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="priority" label="优先级">
            <InputNumber min={1} max={10} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default ClusterPools
