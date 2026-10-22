import { Card, Row, Col, Typography, Space, Tag, Divider } from 'antd'
import {
  ApartmentOutlined,
  DatabaseOutlined,
  ThunderboltOutlined,
  CloudServerOutlined,
  NodeIndexOutlined,
  PartitionOutlined,
  ClusterOutlined,
  RocketOutlined,
} from '@ant-design/icons'
import './Architecture.css'

const { Title, Text, Paragraph } = Typography

const LAYERS = [
  {
    key: 'fabric',
    title: 'GPU Fabric',
    subtitle: '高速互联',
    icon: <NodeIndexOutlined />,
    desc: '节点内 NVLink/NVSwitch，节点间 InfiniBand/RDMA，使数万 GPU 像一台机器一样通信。',
    items: ['NVLink / NVSwitch', 'InfiniBand / RDMA', 'Spine-Leaf 拓扑'],
    color: '#1677ff',
  },
  {
    key: 'pool',
    title: 'GPU Pool',
    subtitle: '资源池',
    icon: <DatabaseOutlined />,
    desc: '按类型/规格池化（A100/H100/RTX 等），支持按需申请、MIG 切分、多租户。',
    items: ['Node Pool', 'MIG 单卡多实例', 'Training / Inference / Dev 池'],
    color: '#52c41a',
  },
  {
    key: 'runtime',
    title: 'AI Runtime',
    subtitle: '训练与推理调度',
    icon: <ThunderboltOutlined />,
    desc: '训练（DeepSpeed/Megatron/ZeRO）、推理（vLLM/TensorRT-LLM）、调度（K8s/Slurm/Ray）。',
    items: ['TP + PP + DP + EP', 'ZeRO 分片', '多模型路由'],
    color: '#722ed1',
  },
]

const PARALLEL_TYPES = [
  { key: 'DP', name: 'Data Parallel', short: '数据并行', tip: '每卡一份模型，梯度 All-Reduce' },
  { key: 'TP', name: 'Tensor Parallel', short: '张量并行', tip: '单层权重切分到多卡' },
  { key: 'PP', name: 'Pipeline Parallel', short: '流水并行', tip: '按层切到多卡流水执行' },
  { key: 'EP', name: 'Expert Parallel', short: '专家并行', tip: 'MoE 专家分布到多卡' },
]

const POOL_TYPES = [
  { label: 'Training', value: 'training', color: 'blue', desc: 'H100 / A100 / H200 大规模训练' },
  { label: 'Inference', value: 'inference', color: 'green', desc: 'RTX / L40S 高吞吐推理' },
  { label: 'Dev', value: 'dev', color: 'orange', desc: 'Spark / 开发与原型、小规模微调' },
]

const Architecture: React.FC = () => {
  return (
    <div className="architecture-page">
      <div className="architecture-header page-header">
        <Title level={2} style={{ margin: 0 }}>
          <ApartmentOutlined /> AI 超算与 GPU 池化架构总览
        </Title>
        <Text type="secondary">
          三层架构：GPU Fabric → GPU Pool → AI Runtime；GPU 作为可切分、可组合、可调度的计算池
        </Text>
      </div>

      {/* 三层架构 */}
      <Card
        className="architecture-card layers-card"
        title={
          <Space>
            <ClusterOutlined />
            <span>三层架构</span>
          </Space>
        }
        bordered={false}
      >
        <Row gutter={[24, 24]}>
          {LAYERS.map((layer) => (
            <Col xs={24} md={8} key={layer.key}>
              <Card
                size="small"
                className="layer-block"
                style={{ borderLeft: `4px solid ${layer.color}` }}
              >
                <Space align="start" style={{ marginBottom: 12 }}>
                  <span className="layer-icon" style={{ color: layer.color }}>
                    {layer.icon}
                  </span>
                  <div>
                    <Title level={5} style={{ margin: 0 }}>
                      {layer.title}
                    </Title>
                    <Tag color="default">{layer.subtitle}</Tag>
                  </div>
                </Space>
                <Paragraph type="secondary" style={{ fontSize: 13, marginBottom: 12 }}>
                  {layer.desc}
                </Paragraph>
                <div className="layer-items">
                  {layer.items.map((item) => (
                    <Tag key={item} style={{ marginBottom: 4 }}>
                      {item}
                    </Tag>
                  ))}
                </div>
              </Card>
            </Col>
          ))}
        </Row>
      </Card>

      {/* 池类型与并行策略 */}
      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
        <Col xs={24} lg={12}>
          <Card
            className="architecture-card"
            title={
              <Space>
                <DatabaseOutlined />
                <span>GPU 池类型（企业常见划分）</span>
              </Space>
            }
            bordered={false}
          >
            <div className="pool-types-list">
              {POOL_TYPES.map((p) => (
                <div key={p.value} className="pool-type-item">
                  <Tag color={p.color}>{p.label}</Tag>
                  <Text type="secondary" style={{ fontSize: 13 }}>
                    {p.desc}
                  </Text>
                </div>
              ))}
            </div>
            <Divider style={{ margin: '12px 0' }} />
            <Text type="secondary" style={{ fontSize: 12 }}>
              Training cluster（H100/A100）、Inference cluster（RTX/L40S）、Dev cluster（Spark）由调度系统统一管理。
            </Text>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card
            className="architecture-card"
            title={
              <Space>
                <PartitionOutlined />
                <span>混合并行（多卡组成大卡）</span>
              </Space>
            }
            bordered={false}
          >
            <div className="parallel-list">
              {PARALLEL_TYPES.map((p) => (
                <div key={p.key} className="parallel-item">
                  <Tag color="blue">{p.key}</Tag>
                  <div>
                    <Text strong style={{ fontSize: 13 }}>{p.short}</Text>
                    <div style={{ fontSize: 12, color: '#8c8c8c' }}>{p.tip}</div>
                  </div>
                </div>
              ))}
            </div>
            <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
              组合使用即 Hybrid Parallel，支撑千亿/万亿参数训练（如 DP×TP×PP×EP）。
            </Text>
          </Card>
        </Col>
      </Row>

      {/* 拓扑示意 */}
      <Card
        className="architecture-card topology-card"
        title={
          <Space>
            <CloudServerOutlined />
            <span>集群拓扑概念</span>
          </Space>
        }
        bordered={false}
        style={{ marginTop: 24 }}
      >
        <pre className="topology-diagram">
{`        Spine（核心交换）
             │
  ┌──────────┼──────────┐
  │          │          │
Leaf 1    Leaf 2     Leaf N
  │          │          │
Rack       Rack       Rack
  │
8×GPU/节点；节点内 NVLink，节点间 IB/RDMA`}
        </pre>
        <Row gutter={16} style={{ marginTop: 16 }}>
          <Col span={12}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              <strong>Rack</strong> → GPU Node × N（每节点通常 8 GPU）<br />
              <strong>Pod</strong> → 约 32 nodes / 256 GPU（一个训练域）<br />
              <strong>SuperCluster</strong> → 10～100 pods，1 万～10 万 GPU
            </Text>
          </Col>
          <Col span={12}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              <strong>调度</strong>：K8s / Slurm；RunAI / Volcano 做 GPU 池化与队列。<br />
              <strong>训练栈</strong>：DeepSpeed + Megatron + ZeRO；TP + PP + DP（+ EP）。
            </Text>
          </Col>
        </Row>
      </Card>

      {/* 核心思想 */}
      <Card
        className="architecture-card core-idea-card"
        bordered={false}
        style={{ marginTop: 24 }}
      >
        <Space align="start">
          <RocketOutlined style={{ fontSize: 24, color: '#1677ff' }} />
          <div>
            <Title level={5} style={{ margin: 0 }}>核心思想</Title>
            <Paragraph style={{ marginBottom: 0 }}>
              把 GPU 当成<strong>「可切分、可组合、可调度的计算池」</strong>，而不是固定服务器里的卡。
              池化方式：GPU Node Pool（整机/整卡）、MIG（单卡物理切分）、GPU Virtualization（vGPU/细粒度共享）。
            </Paragraph>
          </div>
        </Space>
      </Card>
    </div>
  )
}

export default Architecture
