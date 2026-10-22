import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Row, Col, Statistic, Table, Tag, Spin, Progress, Typography, Space, Divider, Button } from 'antd'
import {
  DatabaseOutlined,
  ThunderboltOutlined,
  CheckCircleOutlined,
  DashboardOutlined,
  RiseOutlined,
  ClusterOutlined,
  AppstoreOutlined,
  EyeOutlined,
  SettingOutlined,
  RightOutlined,
} from '@ant-design/icons'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts'
import { poolsApi, PoolInfo } from '../api/pools'
import { tasksApi, Task } from '../api/tasks'
import './Dashboard.css'

const { Title, Text } = Typography

const QUICK_LINKS = [
  { path: '/architecture', icon: <RiseOutlined />, label: '架构总览', desc: '三层架构、GPU 池类型与并行策略' },
  { path: '/pools', icon: <DatabaseOutlined />, label: '资源池', desc: '可切分、可组合、可调度的 GPU/NPU 池' },
  { path: '/cluster-pools', icon: <ClusterOutlined />, label: '集群大池', desc: '多节点聚合成大池、子池切分与配额' },
  { path: '/tasks', icon: <ThunderboltOutlined />, label: '任务管理', desc: '提交与监控推理/训练任务' },
  { path: '/scheduler', icon: <ThunderboltOutlined />, label: '智能调度', desc: '队列、Gang、拓扑、碎片整理' },
  { path: '/ops', icon: <ClusterOutlined />, label: '集群运维', desc: '遥测、自愈、压测与诊断' },
  { path: '/nodes', icon: <ClusterOutlined />, label: '节点管理', desc: '集群节点与设备发现' },
  { path: '/models', icon: <AppstoreOutlined />, label: '模型市场', desc: '注册模型与 NIM 推理' },
  { path: '/compute/perception', icon: <EyeOutlined />, label: '算力感知', desc: '资源状态与绑定' },
  { path: '/settings', icon: <SettingOutlined />, label: '系统配置', desc: '集群与 NIM 配置' },
]

const Dashboard: React.FC = () => {
  const navigate = useNavigate()
  const [pools, setPools] = useState<PoolInfo[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    loadData()
    const interval = setInterval(loadData, 5000)
    return () => clearInterval(interval)
  }, [])

  const loadData = async () => {
    try {
      const [poolsData, tasksData] = await Promise.all([
        poolsApi.listPools(),
        tasksApi.listTasks(),
      ])
      setPools(Array.isArray(poolsData) ? poolsData : [])
      setTasks(Array.isArray(tasksData) ? tasksData : [])
    } catch (error) {
      console.error('Failed to load data:', error)
    } finally {
      setLoading(false)
    }
  }

  const safePools = Array.isArray(pools) ? pools : []
  const safeTasks = Array.isArray(tasks) ? tasks : []
  const totalResources = safePools.reduce((sum, pool) => sum + (pool?.total_count ?? 0), 0)
  const availableResources = safePools.reduce((sum, pool) => sum + (pool?.available_count ?? 0), 0)
  const allocatedResources = safePools.reduce((sum, pool) => sum + (pool?.allocated_count ?? 0), 0)
  const runningTasks = safeTasks.filter(t => t?.status === 'running').length
  const pendingTasks = safeTasks.filter(t => t?.status === 'pending').length
  const totalMemory = safePools.reduce((sum, pool) => sum + (pool?.total_memory ?? 0), 0)
  const usedMemory = safePools.reduce((sum, pool) => sum + (pool?.used_memory ?? 0), 0)
  const memoryUtilization = totalMemory > 0 ? (usedMemory / totalMemory) * 100 : 0
  const resourceUtilization = totalResources > 0 ? (allocatedResources / totalResources) * 100 : 0

  // 模拟历史数据
  const utilizationData = [
    { time: '00:00', utilization: 45, memory: 60 },
    { time: '04:00', utilization: 52, memory: 65 },
    { time: '08:00', utilization: 78, memory: 80 },
    { time: '12:00', utilization: 85, memory: 88 },
    { time: '16:00', utilization: 72, memory: 75 },
    { time: '20:00', utilization: 65, memory: 70 },
  ]

  const taskStatusData = [
    { name: '运行中', value: runningTasks, color: '#52c41a' },
    { name: '待处理', value: pendingTasks, color: '#1890ff' },
    { name: '已完成', value: safeTasks.filter(t => t?.status === 'completed').length, color: '#722ed1' },
    { name: '失败', value: safeTasks.filter(t => t?.status === 'failed').length, color: '#ff4d4f' },
  ]

  const poolTypeData = safePools.reduce((acc, pool) => {
    const type = pool?.type ? String(pool.type).toUpperCase() : 'UNKNOWN'
    acc[type] = (acc[type] || 0) + (pool?.total_count ?? 0)
    return acc
  }, {} as Record<string, number>)

  const poolTypeChartData = Object.entries(poolTypeData).map(([name, value]) => ({
    name,
    value,
  }))

  const taskColumns = [
    {
      title: '任务ID',
      dataIndex: 'id',
      key: 'id',
      width: 120,
      render: (text: string) => <Text code style={{ fontSize: 12 }}>{text.slice(0, 8)}...</Text>,
    },
    {
      title: '任务名称',
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => (
        <Tag color={type === 'inference' ? 'blue' : 'green'} style={{ margin: 0 }}>
          {type === 'inference' ? '推理' : '训练'}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => {
        const colorMap: Record<string, string> = {
          running: 'success',
          pending: 'processing',
          completed: 'default',
          failed: 'error',
          cancelled: 'warning',
        }
        return <Tag color={colorMap[status]}>{status}</Tag>
      },
    },
    {
      title: '资源',
      key: 'resource',
      width: 120,
      render: (_: any, record: Task) => {
        const req = record?.resource_req
        if (!req) return <Text type="secondary">-</Text>
        return (
          <Space size="small">
            <Tag>{(req.resource_type || 'gpu').toUpperCase()}</Tag>
            <Text type="secondary">{req.count ?? 0}x</Text>
          </Space>
        )
      },
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (time: string) => new Date(time).toLocaleString('zh-CN'),
    },
  ]

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="dashboard-container">
      <div className="dashboard-header">
        <Title level={2} style={{ margin: 0 }}>
          <DashboardOutlined /> 系统概览
        </Title>
        <Text type="secondary">GPU 计算池 · 实时监控集群资源与任务执行</Text>
      </div>

      {/* 核心指标卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card stat-card-primary" bordered={false}>
            <Statistic
              title={
                <Space>
                  <DatabaseOutlined />
                  <span>资源池</span>
                </Space>
              }
              value={safePools.length}
              suffix="个"
              valueStyle={{ color: '#1890ff', fontSize: 32, fontWeight: 'bold' }}
            />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary">GPU: {safePools.filter(p => p?.type === 'gpu').length} | NPU: {safePools.filter(p => p?.type === 'npu').length}</Text>
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card stat-card-success" bordered={false}>
            <Statistic
              title={
                <Space>
                  <ThunderboltOutlined />
                  <span>总资源数</span>
                </Space>
              }
              value={totalResources}
              suffix="个"
              valueStyle={{ color: '#52c41a', fontSize: 32, fontWeight: 'bold' }}
            />
            <div style={{ marginTop: 8 }}>
              <Progress
                percent={Math.round(resourceUtilization)}
                size="small"
                strokeColor="#52c41a"
                showInfo={false}
              />
              <Text type="secondary" style={{ fontSize: 12 }}>
                利用率: {resourceUtilization.toFixed(1)}%
              </Text>
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card stat-card-warning" bordered={false}>
            <Statistic
              title={
                <Space>
                  <CheckCircleOutlined />
                  <span>可用资源</span>
                </Space>
              }
              value={availableResources}
              suffix="个"
              valueStyle={{ color: '#faad14', fontSize: 32, fontWeight: 'bold' }}
            />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary">
                已分配: {allocatedResources} 个
              </Text>
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card stat-card-info" bordered={false}>
            <Statistic
              title={
                <Space>
                  <RiseOutlined />
                  <span>运行中任务</span>
                </Space>
              }
              value={runningTasks}
              suffix="个"
              valueStyle={{ color: '#722ed1', fontSize: 32, fontWeight: 'bold' }}
            />
            <div style={{ marginTop: 8 }}>
              <Text type="secondary">
                待处理: {pendingTasks} 个
              </Text>
            </div>
          </Card>
        </Col>
      </Row>

      {/* 架构总览入口 */}
      <Card
        size="small"
        className="chart-card"
        bordered={false}
        style={{ marginBottom: 24, background: 'linear-gradient(135deg, #f0f7ff 0%, #e6f4ff 100%)', border: '1px solid #91caff' }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 16 }}>
          <Space size="middle">
            <RiseOutlined style={{ fontSize: 28, color: '#1677ff' }} />
            <div>
              <Typography.Text strong style={{ fontSize: 15 }}>三层架构</Typography.Text>
              <div style={{ fontSize: 12, color: '#8c8c8c', marginTop: 4 }}>
                GPU Fabric（高速互联）→ GPU Pool（资源池）→ AI Runtime（训练/推理调度）
              </div>
            </div>
          </Space>
          <Button type="primary" size="small" onClick={() => navigate('/architecture')}>
            查看架构总览
          </Button>
        </div>
      </Card>

      {/* 快捷入口 */}
      <Card size="small" className="chart-card" bordered={false} style={{ marginBottom: 24 }}>
        <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 16 }}>
          快捷入口
        </Typography.Title>
        <Row gutter={[12, 12]}>
          {QUICK_LINKS.map((item) => (
            <Col xs={12} sm={8} md={6} lg={4} key={item.path}>
              <Card
                size="small"
                hoverable
                bordered={false}
                style={{
                  background: '#fafafa',
                  borderRadius: 8,
                  cursor: 'pointer',
                }}
                bodyStyle={{ padding: '12px 16px' }}
                onClick={() => navigate(item.path)}
              >
                <Space>
                  <span style={{ fontSize: 18, color: '#1677ff' }}>{item.icon}</span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 600, fontSize: 13 }}>{item.label}</div>
                    <Typography.Text type="secondary" style={{ fontSize: 11 }} ellipsis>
                      {item.desc}
                    </Typography.Text>
                  </div>
                  <RightOutlined style={{ fontSize: 12, color: '#bfbfbf' }} />
                </Space>
              </Card>
            </Col>
          ))}
        </Row>
      </Card>

      {/* 资源利用率趋势 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={16}>
          <Card
            title={
              <Space>
                <RiseOutlined />
                <span>资源利用率趋势</span>
              </Space>
            }
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={300}>
              <AreaChart data={utilizationData}>
                <defs>
                  <linearGradient id="colorUtilization" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#1890ff" stopOpacity={0.8} />
                    <stop offset="95%" stopColor="#1890ff" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="colorMemory" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#52c41a" stopOpacity={0.8} />
                    <stop offset="95%" stopColor="#52c41a" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="time" stroke="#8c8c8c" />
                <YAxis stroke="#8c8c8c" />
                <Tooltip
                  contentStyle={{
                    backgroundColor: 'rgba(255, 255, 255, 0.95)',
                    border: '1px solid #e8e8e8',
                    borderRadius: 4,
                  }}
                />
                <Legend />
                <Area
                  type="monotone"
                  dataKey="utilization"
                  stroke="#1890ff"
                  fillOpacity={1}
                  fill="url(#colorUtilization)"
                  name="资源利用率 (%)"
                />
                <Area
                  type="monotone"
                  dataKey="memory"
                  stroke="#52c41a"
                  fillOpacity={1}
                  fill="url(#colorMemory)"
                  name="内存使用率 (%)"
                />
              </AreaChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <DatabaseOutlined />
                <span>资源类型分布</span>
              </Space>
            }
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie
                  data={poolTypeChartData}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`}
                  outerRadius={80}
                  fill="#8884d8"
                  dataKey="value"
                >
                  {poolTypeChartData.map((_, index) => (
                    <Cell key={`cell-${index}`} fill={index === 0 ? '#1890ff' : '#52c41a'} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {/* 内存使用情况 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={12}>
          <Card
            title={
              <Space>
                <DatabaseOutlined />
                <span>内存使用情况</span>
              </Space>
            }
            className="chart-card"
            bordered={false}
          >
            <div style={{ padding: '20px 0' }}>
              <Row gutter={16}>
                <Col span={12}>
                  <Statistic
                    title="总内存"
                    value={totalMemory}
                    suffix="GB"
                    valueStyle={{ color: '#1890ff', fontSize: 24 }}
                  />
                </Col>
                <Col span={12}>
                  <Statistic
                    title="已用内存"
                    value={usedMemory}
                    suffix="GB"
                    valueStyle={{ color: '#52c41a', fontSize: 24 }}
                  />
                </Col>
              </Row>
              <Divider />
              <Progress
                percent={Math.round(memoryUtilization)}
                strokeColor={{
                  '0%': '#108ee9',
                  '100%': '#87d068',
                }}
                status={memoryUtilization > 80 ? 'exception' : 'active'}
              />
              <div style={{ marginTop: 16, textAlign: 'center' }}>
                <Text type="secondary">
                  内存利用率: <Text strong>{memoryUtilization.toFixed(1)}%</Text>
                </Text>
              </div>
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card
            title={
              <Space>
                <ThunderboltOutlined />
                <span>任务状态分布</span>
              </Space>
            }
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={200}>
              <PieChart>
                <Pie
                  data={taskStatusData}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ name, value }) => `${name}: ${value}`}
                  outerRadius={70}
                  fill="#8884d8"
                  dataKey="value"
                >
                  {taskStatusData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {/* 资源池概览和最近任务 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card
            title={
              <Space>
                <DatabaseOutlined />
                <span>资源池概览</span>
              </Space>
            }
            extra={<a href="/pools">查看全部 →</a>}
            className="overview-card"
            bordered={false}
          >
            {safePools.length === 0 ? (
              <div style={{ textAlign: 'center', padding: 40 }}>
                <Text type="secondary">暂无资源池</Text>
              </div>
            ) : (
              <div>
                {safePools.map((pool) => {
                  const poolUtilization = pool.total_count > 0
                    ? (pool.allocated_count / pool.total_count) * 100
                    : 0
                  const memoryUtil = pool.total_memory > 0
                    ? (pool.used_memory / pool.total_memory) * 100
                    : 0

                  return (
                    <Card
                      key={pool.id}
                      size="small"
                      style={{ marginBottom: 12, borderRadius: 8 }}
                      bodyStyle={{ padding: 16 }}
                    >
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                        <div>
                          <Title level={5} style={{ margin: 0 }}>
                            {pool.name}
                          </Title>
                          <Tag color={pool.type === 'gpu' ? 'blue' : 'green'} style={{ marginTop: 4 }}>
                            {pool.type.toUpperCase()}
                          </Tag>
                        </div>
                        <Text strong style={{ fontSize: 18 }}>
                          {pool.available_count}/{pool.total_count}
                        </Text>
                      </div>
                      <Row gutter={16}>
                        <Col span={12}>
                          <Progress
                            percent={Math.round(poolUtilization)}
                            size="small"
                            strokeColor="#1890ff"
                            format={() => `${pool.allocated_count}/${pool.total_count}`}
                          />
                          <Text type="secondary" style={{ fontSize: 12 }}>资源使用</Text>
                        </Col>
                        <Col span={12}>
                          <Progress
                            percent={Math.round(memoryUtil)}
                            size="small"
                            strokeColor="#52c41a"
                            format={() => `${Math.round(memoryUtil)}%`}
                          />
                          <Text type="secondary" style={{ fontSize: 12 }}>内存使用</Text>
                        </Col>
                      </Row>
                    </Card>
                  )
                })}
              </div>
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card
            title={
              <Space>
                <ThunderboltOutlined />
                <span>最近任务</span>
              </Space>
            }
            extra={<a href="/tasks">查看全部 →</a>}
            className="overview-card"
            bordered={false}
          >
            <Table
              columns={taskColumns}
              dataSource={safeTasks.slice(0, 5)}
              rowKey="id"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Dashboard
