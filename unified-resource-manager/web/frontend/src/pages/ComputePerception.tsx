import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, Modal, Form, Input, Switch, message, Row, Col, Statistic, Typography, Progress } from 'antd'
import { ReloadOutlined, SettingOutlined, LinkOutlined, DisconnectOutlined, EyeOutlined } from '@ant-design/icons'
import { computePerceptionApi, ResourceState, ClusterSettings } from '../api/compute'
import './ComputePerception.css'

const { Title, Text } = Typography

const ComputePerception: React.FC = () => {
  const [resourceStates, setResourceStates] = useState<ResourceState[]>([])
  const [loading, setLoading] = useState(false)
  const [settingsVisible, setSettingsVisible] = useState(false)
  const [bindVisible, setBindVisible] = useState(false)
  const [selectedResource, setSelectedResource] = useState<ResourceState | null>(null)
  const [clusterSettings, setClusterSettings] = useState<ClusterSettings | null>(null)
  const [form] = Form.useForm()
  const [bindForm] = Form.useForm()

  useEffect(() => {
    loadResourceStates()
    loadClusterSettings()
    const interval = setInterval(loadResourceStates, 10000) // 每10秒刷新
    return () => clearInterval(interval)
  }, [])

  const loadResourceStates = async () => {
    setLoading(true)
    try {
      const data = await computePerceptionApi.listResourceStates()
      setResourceStates(Array.isArray(data) ? data : [])
    } catch (error: any) {
      message.error('加载资源状态失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const loadClusterSettings = async () => {
    try {
      const settings = await computePerceptionApi.getClusterSettings()
      setClusterSettings(settings)
      form.setFieldsValue(settings)
    } catch (error: any) {
      console.error('加载集群设置失败:', error)
    }
  }

  const handleUpdateSettings = async (values: any) => {
    try {
      const updated = await computePerceptionApi.updateClusterSettings({
        ...clusterSettings!,
        ...values,
      })
      setClusterSettings(updated)
      message.success('集群设置更新成功')
      setSettingsVisible(false)
    } catch (error: any) {
      message.error('更新集群设置失败: ' + error.message)
    }
  }

  const handleBind = async (values: any) => {
    if (!selectedResource) return
    try {
      await computePerceptionApi.bindResource(selectedResource.resource_id, values.task_id)
      message.success('资源绑定成功')
      setBindVisible(false)
      bindForm.resetFields()
      loadResourceStates()
    } catch (error: any) {
      message.error('绑定资源失败: ' + error.message)
    }
  }

  const handleUnbind = async (resourceId: string) => {
    try {
      await computePerceptionApi.unbindResource(resourceId)
      message.success('资源解绑成功')
      loadResourceStates()
    } catch (error: any) {
      message.error('解绑资源失败: ' + error.message)
    }
  }

  const getHealthColor = (health: string) => {
    const colorMap: Record<string, string> = {
      healthy: 'success',
      warning: 'warning',
      critical: 'error',
      unknown: 'default',
    }
    return colorMap[health] || 'default'
  }

  const getHealthText = (health: string) => {
    const textMap: Record<string, string> = {
      healthy: '健康',
      warning: '警告',
      critical: '严重',
      unknown: '未知',
    }
    return textMap[health] || health
  }

  const safeStates = Array.isArray(resourceStates) ? resourceStates : []
  const stats = {
    total: safeStates.length,
    healthy: safeStates.filter(s => s?.health === 'healthy').length,
    warning: safeStates.filter(s => s?.health === 'warning').length,
    critical: safeStates.filter(s => s?.health === 'critical').length,
    avgUtilization: safeStates.length > 0
      ? safeStates.reduce((sum, s) => sum + (s?.utilization ?? 0), 0) / safeStates.length
      : 0,
  }

  const columns = [
    {
      title: '资源ID',
      dataIndex: 'resource_id',
      key: 'resource_id',
      width: 150,
    },
    {
      title: '节点ID',
      dataIndex: 'node_id',
      key: 'node_id',
      width: 150,
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => <Tag color={type === 'gpu' ? 'blue' : 'green'}>{type.toUpperCase()}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      width: 120,
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
        return <Tag color={colorMap[state]}>{textMap[state] || state}</Tag>
      },
    },
    {
      title: '健康状态',
      dataIndex: 'health',
      key: 'health',
      width: 120,
      render: (health: string) => (
        <Tag color={getHealthColor(health)}>{getHealthText(health)}</Tag>
      ),
    },
    {
      title: '利用率',
      key: 'utilization',
      width: 150,
      render: (_: any, record: ResourceState) => (
        <Progress percent={Math.round(record.utilization)} size="small" />
      ),
    },
    {
      title: '内存使用',
      key: 'memory_usage',
      width: 150,
      render: (_: any, record: ResourceState) => (
        <Progress percent={Math.round(record.memory_usage)} size="small" status={record.memory_usage > 90 ? 'exception' : 'normal'} />
      ),
    },
    {
      title: '核心使用',
      key: 'core_usage',
      width: 150,
      render: (_: any, record: ResourceState) => (
        <Progress percent={Math.round(record.core_usage)} size="small" />
      ),
    },
    {
      title: '温度',
      dataIndex: 'temperature',
      key: 'temperature',
      width: 100,
      render: (temp: number) => temp ? `${temp}°C` : '-',
    },
    {
      title: '功耗',
      dataIndex: 'power_usage',
      key: 'power_usage',
      width: 100,
      render: (power: number) => power ? `${power}W` : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_: any, record: ResourceState) => (
        <Space>
          <Button
            size="small"
            icon={<LinkOutlined />}
            onClick={() => {
              setSelectedResource(record)
              setBindVisible(true)
            }}
            disabled={record.state !== 'available'}
          >
            绑定
          </Button>
          {record.labels?.task_id && (
            <Button
              size="small"
              danger
              icon={<DisconnectOutlined />}
              onClick={() => handleUnbind(record.resource_id)}
            >
              解绑
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="compute-perception-container">
      <div className="compute-perception-header">
        <Title level={2} style={{ margin: 0 }}>
          <EyeOutlined /> 算力感知
        </Title>
        <Text type="secondary">实时监控和管理算力资源状态</Text>
      </div>

      {/* 功能说明 */}
      <Card size="small" style={{ marginBottom: 24 }} bordered={false}>
        <Title level={5} style={{ marginTop: 0 }}>功能说明</Title>
        <ul style={{ marginBottom: 0, paddingLeft: 20, color: 'rgba(0,0,0,0.65)' }}>
          <li><strong>资源状态监控</strong>：从资源池同步 GPU/NPU 的利用率、内存、核心、温度、功耗及健康状态，每 10 秒自动刷新。</li>
          <li><strong>资源绑定</strong>：将「可用」状态的资源绑定到指定任务 ID，便于任务与算力一一对应。</li>
          <li><strong>集群设置</strong>：配置集群 ID、名称、是否自动发现、健康检查间隔等。</li>
        </ul>
        {safeStates.length === 0 && (
          <Text type="secondary" style={{ display: 'block', marginTop: 12 }}>
            暂无资源状态时，请确保已创建资源池并有可用资源；算力感知会从资源池拉取并展示各资源当前状态。
          </Text>
        )}
      </Card>

      {/* 统计卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="总资源数"
              value={stats.total}
              suffix="个"
              valueStyle={{ color: '#1890ff', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="健康资源"
              value={stats.healthy}
              suffix="个"
              valueStyle={{ color: '#52c41a', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="警告资源"
              value={stats.warning}
              suffix="个"
              valueStyle={{ color: '#faad14', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="平均利用率"
              value={stats.avgUtilization.toFixed(1)}
              suffix="%"
              valueStyle={{ color: '#722ed1', fontSize: 28 }}
            />
          </Card>
        </Col>
      </Row>

      <Card
        title="资源状态监控"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadResourceStates}>
              刷新
            </Button>
            <Button icon={<SettingOutlined />} onClick={() => setSettingsVisible(true)}>
              集群设置
            </Button>
          </Space>
        }
        className="perception-table-card"
        bordered={false}
      >
        <Table
          columns={columns}
          dataSource={safeStates}
          rowKey="resource_id"
          loading={loading}
          locale={{
            emptyText: '暂无资源状态。请先在「资源池」中确保有池与资源，系统会从资源池同步并在此展示。',
          }}
        />
      </Card>

      {/* 集群设置弹窗 */}
      <Modal
        title="集群设置"
        open={settingsVisible}
        onOk={() => form.submit()}
        onCancel={() => setSettingsVisible(false)}
        width={600}
      >
        <Form form={form} onFinish={handleUpdateSettings} layout="vertical">
          <Form.Item name="cluster_id" label="集群ID">
            <Input disabled />
          </Form.Item>
          <Form.Item name="name" label="集群名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="auto_discovery" label="自动发现" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="health_check_interval" label="健康检查间隔">
            <Input placeholder="例如：10s" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 资源绑定弹窗 */}
      <Modal
        title="绑定资源"
        open={bindVisible}
        onOk={() => bindForm.submit()}
        onCancel={() => {
          setBindVisible(false)
          bindForm.resetFields()
        }}
        width={500}
      >
        <Form form={bindForm} onFinish={handleBind} layout="vertical">
          <Form.Item label="资源ID">
            <Input value={selectedResource?.resource_id} disabled />
          </Form.Item>
          <Form.Item name="task_id" label="任务ID" rules={[{ required: true }]}>
            <Input placeholder="输入要绑定的任务ID" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default ComputePerception
