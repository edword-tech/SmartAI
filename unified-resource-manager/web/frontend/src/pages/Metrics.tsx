import { useEffect, useState } from 'react'
import { Card, Row, Col, Select, DatePicker, Space, Typography, Statistic, Progress } from 'antd'
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, BarChart, Bar } from 'recharts'
import { poolsApi, PoolInfo } from '../api/pools'
import { LineChartOutlined, RiseOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import './Metrics.css'

const { RangePicker } = DatePicker
const { Title, Text } = Typography

const Metrics: React.FC = () => {
  const [pools, setPools] = useState<PoolInfo[]>([])
  const [selectedPool, setSelectedPool] = useState<string>('')
  const [timeRange, setTimeRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>([
    dayjs().subtract(24, 'hour'),
    dayjs(),
  ])

  useEffect(() => {
    loadPools()
  }, [])

  const loadPools = async () => {
    try {
      const data = await poolsApi.listPools()
      setPools(data)
      if (data.length > 0) {
        setSelectedPool(data[0].id)
      }
    } catch (error) {
      console.error('Failed to load pools:', error)
    }
  }

  // 模拟指标数据
  const utilizationData = Array.from({ length: 24 }, (_, i) => ({
    time: `${String(i).padStart(2, '0')}:00`,
    utilization: Math.floor(Math.random() * 30) + 50,
    memory: Math.floor(Math.random() * 25) + 60,
    temperature: Math.floor(Math.random() * 10) + 65,
    power: Math.floor(Math.random() * 50) + 200,
  }))

  const taskMetricsData = [
    { name: '00:00', inference: 12, training: 5 },
    { name: '04:00', inference: 15, training: 8 },
    { name: '08:00', inference: 28, training: 12 },
    { name: '12:00', inference: 35, training: 15 },
    { name: '16:00', inference: 32, training: 18 },
    { name: '20:00', inference: 25, training: 10 },
  ]

  const resourceDistribution = [
    { name: 'GPU', value: pools.filter(p => p.type === 'gpu').reduce((sum, p) => sum + p.total_count, 0) },
    { name: 'NPU', value: pools.filter(p => p.type === 'npu').reduce((sum, p) => sum + p.total_count, 0) },
  ]

  const selectedPoolData = pools.find(p => p.id === selectedPool)
  const poolUtilization = selectedPoolData && selectedPoolData.total_count > 0
    ? (selectedPoolData.allocated_count / selectedPoolData.total_count) * 100
    : 0
  const memoryUtil = selectedPoolData && selectedPoolData.total_memory > 0
    ? (selectedPoolData.used_memory / selectedPoolData.total_memory) * 100
    : 0

  return (
    <div className="metrics-container">
      <div className="metrics-header">
        <Title level={2} style={{ margin: 0 }}>
          <LineChartOutlined /> 监控指标
        </Title>
        <Text type="secondary">实时监控资源使用情况和性能指标</Text>
      </div>

      {/* 筛选器 */}
      <Card className="filter-card" bordered={false} style={{ marginBottom: 24 }}>
        <Space size="large">
          <div>
            <Text strong style={{ marginRight: 8 }}>资源池:</Text>
            <Select
              style={{ width: 200 }}
              value={selectedPool}
              onChange={setSelectedPool}
              placeholder="选择资源池"
            >
              {pools.map((pool) => (
                <Select.Option key={pool.id} value={pool.id}>
                  {pool.name} ({pool.type.toUpperCase()})
                </Select.Option>
              ))}
            </Select>
          </div>
          <div>
            <Text strong style={{ marginRight: 8 }}>时间范围:</Text>
            <RangePicker
              value={timeRange}
              onChange={(dates) => dates && setTimeRange(dates as [dayjs.Dayjs, dayjs.Dayjs])}
              showTime
              format="YYYY-MM-DD HH:mm"
            />
          </div>
        </Space>
      </Card>

      {/* 关键指标 */}
      {selectedPoolData && (
        <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
          <Col xs={24} sm={12} lg={6}>
            <Card className="metric-card" bordered={false}>
              <Statistic
                title="资源利用率"
                value={poolUtilization}
                suffix="%"
                prefix={<RiseOutlined />}
                valueStyle={{ color: poolUtilization > 80 ? '#ff4d4f' : poolUtilization > 60 ? '#faad14' : '#52c41a' }}
              />
              <Progress
                percent={Math.round(poolUtilization)}
                strokeColor={poolUtilization > 80 ? '#ff4d4f' : poolUtilization > 60 ? '#faad14' : '#52c41a'}
                style={{ marginTop: 16 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card className="metric-card" bordered={false}>
              <Statistic
                title="内存使用率"
                value={memoryUtil}
                suffix="%"
                prefix={<RiseOutlined />}
                valueStyle={{ color: memoryUtil > 80 ? '#ff4d4f' : memoryUtil > 60 ? '#faad14' : '#52c41a' }}
              />
              <Progress
                percent={Math.round(memoryUtil)}
                strokeColor={memoryUtil > 80 ? '#ff4d4f' : memoryUtil > 60 ? '#faad14' : '#52c41a'}
                style={{ marginTop: 16 }}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card className="metric-card" bordered={false}>
              <Statistic
                title="总资源数"
                value={selectedPoolData.total_count}
                suffix="个"
                valueStyle={{ color: '#1890ff' }}
              />
              <div style={{ marginTop: 16 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  可用: {selectedPoolData.available_count} | 已分配: {selectedPoolData.allocated_count}
                </Text>
              </div>
            </Card>
          </Col>
          <Col xs={24} sm={12} lg={6}>
            <Card className="metric-card" bordered={false}>
              <Statistic
                title="总内存"
                value={selectedPoolData.total_memory}
                suffix="GB"
                valueStyle={{ color: '#722ed1' }}
              />
              <div style={{ marginTop: 16 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  已用: {selectedPoolData.used_memory} GB
                </Text>
              </div>
            </Card>
          </Col>
        </Row>
      )}

      {/* 资源利用率趋势 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={16}>
          <Card
            title="资源利用率趋势"
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={350}>
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
            title="资源类型分布"
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={350}>
              <BarChart data={resourceDistribution}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="name" stroke="#8c8c8c" />
                <YAxis stroke="#8c8c8c" />
                <Tooltip />
                <Bar dataKey="value" fill="#1890ff" radius={[8, 8, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {/* 温度和功耗 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={12}>
          <Card
            title="温度趋势"
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={utilizationData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="time" stroke="#8c8c8c" />
                <YAxis stroke="#8c8c8c" />
                <Tooltip />
                <Legend />
                <Line
                  type="monotone"
                  dataKey="temperature"
                  stroke="#ff4d4f"
                  strokeWidth={2}
                  name="温度 (°C)"
                />
              </LineChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card
            title="功耗趋势"
            className="chart-card"
            bordered={false}
          >
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={utilizationData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis dataKey="time" stroke="#8c8c8c" />
                <YAxis stroke="#8c8c8c" />
                <Tooltip />
                <Legend />
                <Line
                  type="monotone"
                  dataKey="power"
                  stroke="#722ed1"
                  strokeWidth={2}
                  name="功耗 (W)"
                />
              </LineChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {/* 任务指标 */}
      <Card
        title="任务执行统计"
        className="chart-card"
        bordered={false}
      >
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={taskMetricsData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
            <XAxis dataKey="name" stroke="#8c8c8c" />
            <YAxis stroke="#8c8c8c" />
            <Tooltip />
            <Legend />
            <Bar dataKey="inference" fill="#1890ff" name="推理任务" radius={[8, 8, 0, 0]} />
            <Bar dataKey="training" fill="#52c41a" name="训练任务" radius={[8, 8, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </Card>
    </div>
  )
}

export default Metrics
