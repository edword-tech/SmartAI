import { useEffect, useState } from 'react'
import { Card, Button, Space, Modal, Form, Select, Input, InputNumber, message, Typography, Descriptions, Tag, Row, Col } from 'antd'
import { ReloadOutlined, RocketOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { computeOrchestrationApi, OrchestrationStrategies } from '../api/compute'
import './ComputeOrchestration.css'

const { Title, Text } = Typography

const ComputeOrchestration: React.FC = () => {
  const [strategies, setStrategies] = useState<OrchestrationStrategies | null>(null)
  const [distributeVisible, setDistributeVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    loadStrategies()
  }, [])

  const loadStrategies = async () => {
    try {
      const data = await computeOrchestrationApi.getStrategies()
      setStrategies(data)
    } catch (error: any) {
      message.error('加载调度策略失败: ' + error.message)
    }
  }

  const handleDistribute = async (values: any) => {
    try {
      const task = {
        name: values.name,
        type: values.type,
        priority: values.priority,
        resource_req: {
          task_id: `task-${Date.now()}`,
          task_type: values.type,
          resource_type: values.resource_type,
          count: values.count,
          memory_gb: values.memory_gb,
          cores: values.cores,
          priority: values.priority,
          timeout: `${values.timeout || 3600}s`,
        },
      }
      await computeOrchestrationApi.distributeTask(task, values.strategy)
      message.success('任务下发成功')
      setDistributeVisible(false)
      form.resetFields()
    } catch (error: any) {
      message.error('任务下发失败: ' + error.message)
    }
  }

  const handleOptimize = async () => {
    try {
      await computeOrchestrationApi.optimizeScheduling()
      message.success('调度优化完成')
    } catch (error: any) {
      message.error('调度优化失败: ' + error.message)
    }
  }

  return (
    <div className="compute-orchestration-container">
      <div className="compute-orchestration-header">
        <Title level={2} style={{ margin: 0 }}>
          <ThunderboltOutlined /> 算力编排
        </Title>
        <Text type="secondary">智能编排和调度算力资源</Text>
      </div>

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} lg={12}>
          <Card title="均衡调度策略" bordered={false} className="strategy-card">
            <Descriptions column={1} size="small">
              <Descriptions.Item label="策略类型">
                <Tag color="blue">Balanced（均衡）</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="说明">
                将任务分配到负载最低的节点，实现负载均衡
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="最小碎片调度策略" bordered={false} className="strategy-card">
            <Descriptions column={1} size="small">
              <Descriptions.Item label="策略类型">
                <Tag color="green">Min Fragment（最小碎片）</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="说明">
                优先使用碎片小的资源，最大化资源利用率
              </Descriptions.Item>
              <Descriptions.Item label="碎片阈值">
                {strategies?.min_fragment?.fragment_threshold
                  ? `${(strategies.min_fragment.fragment_threshold * 100).toFixed(1)}%`
                  : '10%'}
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="弹性调度策略" bordered={false} className="strategy-card">
            <Descriptions column={1} size="small">
              <Descriptions.Item label="策略类型">
                <Tag color="orange">Elastic（弹性）</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="说明">
                根据资源利用率自动触发扩容或缩容
              </Descriptions.Item>
              <Descriptions.Item label="扩容阈值">
                {strategies?.elastic?.scale_up_threshold
                  ? `${(strategies.elastic.scale_up_threshold * 100).toFixed(1)}%`
                  : '80%'}
              </Descriptions.Item>
              <Descriptions.Item label="缩容阈值">
                {strategies?.elastic?.scale_down_threshold
                  ? `${(strategies.elastic.scale_down_threshold * 100).toFixed(1)}%`
                  : '30%'}
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="操作" bordered={false} className="strategy-card">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Button
                type="primary"
                icon={<RocketOutlined />}
                onClick={() => setDistributeVisible(true)}
                block
                size="large"
              >
                下发任务
              </Button>
              <Button
                icon={<ThunderboltOutlined />}
                onClick={handleOptimize}
                block
                size="large"
              >
                优化调度
              </Button>
              <Button icon={<ReloadOutlined />} onClick={loadStrategies} block>
                刷新策略
              </Button>
            </Space>
          </Card>
        </Col>
      </Row>

      {/* 下发任务弹窗 */}
      <Modal
        title="下发任务"
        open={distributeVisible}
        onOk={() => form.submit()}
        onCancel={() => {
          setDistributeVisible(false)
          form.resetFields()
        }}
        width={700}
      >
        <Form form={form} onFinish={handleDistribute} layout="vertical">
          <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
            <Input placeholder="例如：分布式训练任务" />
          </Form.Item>
          <Form.Item name="type" label="任务类型" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="inference">推理</Select.Option>
              <Select.Option value="training">训练</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="strategy" label="调度策略" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="balanced">均衡调度</Select.Option>
              <Select.Option value="min_fragment">最小碎片调度</Select.Option>
              <Select.Option value="elastic">弹性调度</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="priority" label="优先级" rules={[{ required: true }]} initialValue={5}>
            <InputNumber min={1} max={10} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="resource_type" label="资源类型" rules={[{ required: true }]}>
            <Select>
              <Select.Option value="gpu">GPU</Select.Option>
              <Select.Option value="npu">NPU</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="count" label="资源数量" rules={[{ required: true }]} initialValue={1}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="memory_gb" label="内存 (GB)" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="cores" label="核心数" rules={[{ required: true }]}>
            <InputNumber min={1} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="timeout" label="超时时间 (秒)" initialValue={3600}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default ComputeOrchestration
