import { useEffect, useState } from 'react'
import { Card, Table, Tag, Button, Space, message, Row, Col, Statistic, Typography, Tooltip, Modal, Descriptions } from 'antd'
import { ReloadOutlined, DatabaseOutlined, ClusterOutlined, EyeOutlined } from '@ant-design/icons'
import { nodesApi, Node } from '../api/nodes'
import './Nodes.css'

const { Title, Text } = Typography

const Nodes: React.FC = () => {
  const [nodes, setNodes] = useState<Node[]>([])
  const [loading, setLoading] = useState(false)
  const [discovering, setDiscovering] = useState<string | null>(null)
  const [detailNodeId, setDetailNodeId] = useState<string | null>(null)
  const [detailNode, setDetailNode] = useState<Node | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  useEffect(() => {
    loadNodes()
  }, [])

  const loadNodes = async () => {
    setLoading(true)
    try {
      const data = await nodesApi.listNodes()
      setNodes(data)
    } catch (error: any) {
      message.error('加载节点失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleDiscover = async (nodeId: string) => {
    setDiscovering(nodeId)
    try {
      await nodesApi.discoverNodeResources(nodeId)
      message.success('节点资源发现已触发')
      loadNodes()
    } catch (error: any) {
      message.error('发现节点资源失败: ' + (error?.message ?? String(error)))
    } finally {
      setDiscovering(null)
    }
  }

  const handleViewDetail = async (nodeId: string) => {
    setDetailNodeId(nodeId)
    setDetailNode(null)
    setDetailLoading(true)
    try {
      const node = await nodesApi.getNode(nodeId)
      setDetailNode(node)
    } catch (error: any) {
      message.error('获取节点详情失败: ' + (error?.message ?? String(error)))
      setDetailNodeId(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const columns = [
    {
      title: '节点名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Node) => (
        <Space>
          <ClusterOutlined />
          <Text strong>{text}</Text>
          <Tag color="default">{record.id}</Tag>
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type: string) => {
        const colorMap: Record<string, string> = {
          gpu: 'blue',
          npu: 'green',
          hybrid: 'purple',
        }
        const textMap: Record<string, string> = {
          gpu: 'GPU',
          npu: 'NPU',
          hybrid: '混合',
        }
        return <Tag color={colorMap[type]}>{textMap[type] || type}</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      width: 100,
      render: (state: string) => {
        const colorMap: Record<string, string> = {
          online: 'success',
          offline: 'default',
          maintenance: 'warning',
          error: 'error',
        }
        const textMap: Record<string, string> = {
          online: '在线',
          offline: '离线',
          maintenance: '维护中',
          error: '异常',
        }
        return <Tag color={colorMap[state]}>{textMap[state] || state}</Tag>
      },
    },
    {
      title: 'GPU',
      key: 'gpu',
      width: 140,
      render: (_: any, record: Node) => {
        const gpus = record.resources?.gpus || []
        const totalMemory = gpus.reduce((sum, g) => sum + g.total_memory, 0)
        return (
          <div>
            <Text>数量: {gpus.length}</Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              总显存: {totalMemory} GB
            </Text>
          </div>
        )
      },
    },
    {
      title: 'NPU',
      key: 'npu',
      width: 140,
      render: (_: any, record: Node) => {
        const npus = record.resources?.npus || []
        const totalMemory = npus.reduce((sum, n) => sum + n.total_memory, 0)
        return (
          <div>
            <Text>数量: {npus.length}</Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              总显存: {totalMemory} GB
            </Text>
          </div>
        )
      },
    },
    {
      title: '地址',
      dataIndex: 'address',
      key: 'address',
    },
    {
      title: '最后更新',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
      render: (time: string) => (time ? new Date(time).toLocaleString('zh-CN') : '-'),
    },
    {
      title: 'RDMA',
      key: 'rdma',
      width: 140,
      render: (_: unknown, record: Node) => record.network?.rdma_domain || '-',
    },
    {
      title: '隔离',
      dataIndex: 'cordoned',
      width: 80,
      render: (v: boolean) => v ? <Tag color="orange">cordon</Tag> : <Tag color="green">可调度</Tag>,
    },
    {
      title: '操作',
      key: 'action',
      width: 280,
      render: (_: any, record: Node) => (
        <Space wrap>
          <Tooltip title="查看节点详情">
            <Button
              size="small"
              type="link"
              icon={<EyeOutlined />}
              onClick={() => handleViewDetail(record.id)}
            >
              详情
            </Button>
          </Tooltip>
          <Tooltip title="刷新资源信息">
            <Button
              size="small"
              loading={discovering === record.id}
              onClick={() => handleDiscover(record.id)}
            >
              发现资源
            </Button>
          </Tooltip>
          <Button
            size="small"
            onClick={async () => {
              await nodesApi.cordon(record.id, !record.cordoned)
              message.success(record.cordoned ? '已解除隔离' : '节点已隔离')
              loadNodes()
            }}
          >
            {record.cordoned ? '解除隔离' : '隔离'}
          </Button>
          <Button
            size="small"
            danger
            onClick={async () => {
              const r = await nodesApi.drain(record.id)
              message.success(`已排空，迁移 ${r.migrated_tasks?.length || 0} 个任务`)
              loadNodes()
            }}
          >
            排空
          </Button>
        </Space>
      ),
    },
  ]

  const totalGPUs = nodes.reduce((sum, n) => sum + (n.resources?.gpus?.length || 0), 0)
  const totalNPUs = nodes.reduce((sum, n) => sum + (n.resources?.npus?.length || 0), 0)

  return (
    <div className="nodes-container">
      <div className="nodes-header">
        <Title level={2} style={{ margin: 0 }}>
          <ClusterOutlined /> 节点管理
        </Title>
        <Text type="secondary">查看和管理集群中的 GPU / NPU 节点</Text>
      </div>

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={8}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="节点总数"
              value={nodes.length}
              suffix="个"
              prefix={<DatabaseOutlined style={{ color: '#1890ff' }} />}
              valueStyle={{ color: '#1890ff', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={8}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="GPU 数量"
              value={totalGPUs}
              suffix="块"
              valueStyle={{ color: '#52c41a', fontSize: 28 }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={8}>
          <Card className="stat-card" bordered={false}>
            <Statistic
              title="NPU 数量"
              value={totalNPUs}
              suffix="块"
              valueStyle={{ color: '#722ed1', fontSize: 28 }}
            />
          </Card>
        </Col>
      </Row>

      <Card
        title="节点列表"
        extra={
          <Button icon={<ReloadOutlined />} onClick={loadNodes}>
            刷新
          </Button>
        }
        className="nodes-table-card"
        bordered={false}
      >
        <Table
          columns={columns}
          dataSource={nodes}
          rowKey="id"
          loading={loading}
          locale={{ emptyText: '暂无节点数据' }}
          expandable={{
            expandedRowRender: (record: Node) => {
              const gpus = record.resources?.gpus || []
              const npus = record.resources?.npus || []
              return (
                <div style={{ padding: '8px 16px' }}>
                  <Row gutter={16}>
                    <Col span={12}>
                      <Title level={5}>GPU 列表</Title>
                      {gpus.length === 0 ? (
                        <Text type="secondary">暂无 GPU 设备</Text>
                      ) : (
                        gpus.map((g) => (
                          <div key={g.device_id} style={{ marginBottom: 4 }}>
                            <Tag color="blue">{g.device_id}</Tag>
                            <Text type="secondary" style={{ marginLeft: 8 }}>
                              {g.model} / {g.total_memory} GB
                              {g.topology?.nvlink_group ? ` / NVLink ${g.topology.nvlink_group}` : ''}
                              {g.topology ? ` / NUMA ${g.topology.numa_node}` : ''}
                            </Text>
                          </div>
                        ))
                      )}
                    </Col>
                    <Col span={12}>
                      <Title level={5}>NPU 列表</Title>
                      {npus.length === 0 ? (
                        <Text type="secondary">暂无 NPU 设备</Text>
                      ) : (
                        npus.map((n) => (
                          <div key={n.device_id} style={{ marginBottom: 4 }}>
                            <Tag color="green">{n.device_id}</Tag>
                            <Text type="secondary" style={{ marginLeft: 8 }}>
                              {n.model} / {n.total_memory} GB / {n.total_cores} cores
                            </Text>
                          </div>
                        ))
                      )}
                    </Col>
                  </Row>
                </div>
              )
            },
          }}
        />
      </Card>

      <Modal
        title="节点详情"
        open={detailNodeId !== null}
        onCancel={() => { setDetailNodeId(null); setDetailNode(null) }}
        footer={null}
        width={560}
      >
        {detailLoading ? (
          <Text type="secondary">加载中...</Text>
        ) : detailNode ? (
          <Descriptions column={1} size="small">
            <Descriptions.Item label="节点 ID">{detailNode.id}</Descriptions.Item>
            <Descriptions.Item label="名称">{detailNode.name}</Descriptions.Item>
            <Descriptions.Item label="类型">
              <Tag color={detailNode.type === 'gpu' ? 'blue' : detailNode.type === 'npu' ? 'green' : 'purple'}>
                {detailNode.type === 'gpu' ? 'GPU' : detailNode.type === 'npu' ? 'NPU' : '混合'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="状态">{detailNode.state}</Descriptions.Item>
            <Descriptions.Item label="地址">{detailNode.address}</Descriptions.Item>
            <Descriptions.Item label="最后更新">
              {detailNode.updated_at ? new Date(detailNode.updated_at).toLocaleString('zh-CN') : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="GPU 数量">
              {detailNode.resources?.gpus?.length ?? 0} 块
            </Descriptions.Item>
            <Descriptions.Item label="NPU 数量">
              {detailNode.resources?.npus?.length ?? 0} 块
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </div>
  )
}

export default Nodes
