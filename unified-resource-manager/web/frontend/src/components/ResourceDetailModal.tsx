import { useEffect, useState } from 'react'
import { Modal, Descriptions, Tag, Spin, Card, Row, Col, Statistic, Progress, Typography, Divider, Space } from 'antd'
import { DatabaseOutlined } from '@ant-design/icons'
import { Resource } from '../api/pools'

const { Text } = Typography

interface ResourceDetailModalProps {
  visible: boolean
  resource: Resource | null
  onClose: () => void
}

const ResourceDetailModal: React.FC<ResourceDetailModalProps> = ({
  visible,
  resource,
  onClose,
}) => {
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (visible && resource) {
      setLoading(false)
    }
  }, [visible, resource])

  if (!resource) return null

  const memoryUtil = resource.total_memory > 0
    ? (resource.used_memory / resource.total_memory) * 100
    : 0
  const coresUtil = resource.total_cores > 0
    ? (resource.used_cores / resource.total_cores) * 100
    : 0

  return (
    <Modal
      title={
        <Space>
          <DatabaseOutlined style={{ color: resource.type === 'gpu' ? '#1890ff' : '#52c41a' }} />
          <span>资源详情</span>
        </Space>
      }
      open={visible}
      onCancel={onClose}
      footer={null}
      width={900}
      styles={{
        body: { padding: '24px' },
      }}
    >
      <Spin spinning={loading}>
        <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
          <Col span={8}>
            <Card bordered={false} style={{ textAlign: 'center' }}>
              <Statistic
                title="内存使用率"
                value={memoryUtil}
                suffix="%"
                valueStyle={{ color: memoryUtil > 80 ? '#ff4d4f' : memoryUtil > 60 ? '#faad14' : '#52c41a' }}
              />
              <Progress
                percent={Math.round(memoryUtil)}
                strokeColor={memoryUtil > 80 ? '#ff4d4f' : memoryUtil > 60 ? '#faad14' : '#52c41a'}
                style={{ marginTop: 16 }}
              />
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
                {resource.used_memory} / {resource.total_memory} GB
              </Text>
            </Card>
          </Col>
          <Col span={8}>
            <Card bordered={false} style={{ textAlign: 'center' }}>
              <Statistic
                title="核心使用率"
                value={coresUtil}
                suffix="%"
                valueStyle={{ color: coresUtil > 80 ? '#ff4d4f' : coresUtil > 60 ? '#faad14' : '#52c41a' }}
              />
              <Progress
                percent={Math.round(coresUtil)}
                strokeColor={coresUtil > 80 ? '#ff4d4f' : coresUtil > 60 ? '#faad14' : '#52c41a'}
                style={{ marginTop: 16 }}
              />
              <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
                {resource.used_cores} / {resource.total_cores}
              </Text>
            </Card>
          </Col>
          <Col span={8}>
            <Card bordered={false} style={{ textAlign: 'center' }}>
              <Statistic
                title="资源状态"
                value={resource.state === 'available' ? '可用' : resource.state === 'allocated' ? '已分配' : resource.state}
                valueStyle={{ color: resource.state === 'available' ? '#52c41a' : resource.state === 'allocated' ? '#1890ff' : '#ff4d4f' }}
              />
              <Tag
                color={resource.state === 'available' ? 'success' : resource.state === 'allocated' ? 'processing' : 'error'}
                style={{ marginTop: 16, fontSize: 14, padding: '4px 12px' }}
              >
                {resource.state}
              </Tag>
            </Card>
          </Col>
        </Row>

        <Divider />

        <Descriptions column={2} bordered size="middle">
          <Descriptions.Item label="资源ID" span={2}>
            <Text code>{resource.id}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="资源类型">
            <Tag color={resource.type === 'gpu' ? 'blue' : 'green'} style={{ fontSize: 12 }}>
              {resource.type.toUpperCase()}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="节点ID">
            <Text>{resource.node_id}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="设备ID">
            <Text>{resource.device_id}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag
              color={
                resource.state === 'available'
                  ? 'success'
                  : resource.state === 'allocated'
                  ? 'processing'
                  : 'error'
              }
            >
              {resource.state === 'available' ? '可用' : resource.state === 'allocated' ? '已分配' : resource.state}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="总内存">
            <Text strong>{resource.total_memory} GB</Text>
          </Descriptions.Item>
          <Descriptions.Item label="已用内存">
            <Text>{resource.used_memory} GB</Text>
          </Descriptions.Item>
          <Descriptions.Item label="总核心数">
            <Text strong>{resource.total_cores}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="已用核心数">
            <Text>{resource.used_cores}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="创建时间" span={2}>
            {new Date(resource.created_at).toLocaleString('zh-CN')}
          </Descriptions.Item>
          <Descriptions.Item label="更新时间" span={2}>
            {new Date(resource.updated_at).toLocaleString('zh-CN')}
          </Descriptions.Item>
        </Descriptions>
      </Spin>
    </Modal>
  )
}

export default ResourceDetailModal
