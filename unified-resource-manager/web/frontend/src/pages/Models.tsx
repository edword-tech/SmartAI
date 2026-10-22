import { useEffect, useState } from 'react'
import { Card, Table, Button, Space, Tag, Modal, Form, Input, Select, message, Typography, Tabs, InputNumber, Checkbox, Row, Col, Radio } from 'antd'
import { PlusOutlined, ReloadOutlined, DeleteOutlined, AppstoreOutlined, SearchOutlined } from '@ant-design/icons'
import { modelsApi, Model, CreateModelRequest } from '../api/models'
import { flavorsApi, ResourceFlavor } from '../api/flavors'
import { projectsApi, Project } from '../api/projects'
import './Models.css'

const { Title, Text } = Typography
const { TextArea } = Input

// 模型类型分类
const MODEL_TYPES = [
  { key: 'all', label: '全部模型' },
  { key: 'llm', label: '大语言模型' },
  { key: 'embedding', label: 'Embedding 模型' },
  { key: 'rerank', label: 'Rerank 模型' },
  { key: 'cv', label: '视觉模型' },
  { key: 'asr', label: '语音识别' },
  { key: 'audio', label: '音频模型' },
  { key: 'video', label: '视频模型' },
  { key: 'custom', label: '自定义模型' },
]

// 模型能力选项
const MODEL_ABILITIES = [
  { value: 'generate', label: '生成' },
  { value: 'chat', label: '对话' },
  { value: 'vision', label: '视觉' },
  { value: 'tools', label: '工具调用' },
  { value: 'reasoning', label: '推理' },
  { value: 'audio', label: '音频' },
  { value: 'omni', label: '全能' },
  { value: 'hybrid', label: '混合' },
]

// 模型格式选项
const MODEL_FORMATS = [
  { value: 'pytorch', label: 'PyTorch' },
  { value: 'gguf', label: 'GGUF' },
  { value: 'gptq', label: 'GPTQ' },
  { value: 'awq', label: 'AWQ' },
  { value: 'fp8', label: 'FP8' },
  { value: 'mlx', label: 'MLX' },
]

const Models: React.FC = () => {
  const [models, setModels] = useState<Model[]>([])
  const [filteredModels, setFilteredModels] = useState<Model[]>([])
  const [loading, setLoading] = useState(false)
  const [createVisible, setCreateVisible] = useState(false)
  const [activeTab, setActiveTab] = useState<string>('all')
  const [searchText, setSearchText] = useState('')
  const [form] = Form.useForm()
  const [flavors, setFlavors] = useState<ResourceFlavor[]>([])
  const [projects, setProjects] = useState<Project[]>([])

  useEffect(() => {
    loadModels()
    loadFlavors()
    loadProjects()
  }, [])

  useEffect(() => {
    filterModels()
  }, [models, activeTab, searchText])

  const loadModels = async () => {
    setLoading(true)
    try {
      const data = await modelsApi.listModels()
      setModels(Array.isArray(data) ? data : [])
    } catch (error: any) {
      message.error('加载模型失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const loadFlavors = async () => {
    try {
      const data = await flavorsApi.listFlavors()
      setFlavors(data)
    } catch (error) {
      console.error('Failed to load flavors:', error)
    }
  }

  const loadProjects = async () => {
    try {
      const data = await projectsApi.listProjects()
      setProjects(data)
    } catch (error) {
      console.error('Failed to load projects:', error)
    }
  }

  const filterModels = () => {
    let filtered = [...models]
    
    // 按类型过滤
    if (activeTab !== 'all') {
      filtered = filtered.filter(m => m.type === activeTab)
    }
    
    // 按搜索文本过滤
    if (searchText) {
      const lowerSearch = searchText.toLowerCase()
      filtered = filtered.filter(m => 
        m.name.toLowerCase().includes(lowerSearch) ||
        m.description?.toLowerCase().includes(lowerSearch) ||
        m.family?.toLowerCase().includes(lowerSearch)
      )
    }
    
    setFilteredModels(filtered)
  }

  const handleCreate = async (values: any) => {
    const req: CreateModelRequest = {
      name: values.name,
      version: values.version || 'v1.0.0',
      type: values.type,
      framework: values.framework,
      format: values.format,
      resource_flavor_id: values.resource_flavor_id,
      path: values.path,
      description: values.description,
      context_length: values.context_length,
      size: values.size,
      family: values.family,
      languages: values.languages,
      abilities: values.abilities,
      project_id: values.project_id,
      inference_backend: values.inference_backend || undefined,
      nim_model_id: values.nim_model_id || undefined,
    }
    try {
      await modelsApi.createModel(req)
      message.success('模型注册成功')
      setCreateVisible(false)
      form.resetFields()
      loadModels()
    } catch (error: any) {
      message.error('注册模型失败: ' + error.message)
    }
  }

  const handleDelete = async (record: Model) => {
    try {
      await modelsApi.deleteModel(record.id)
      message.success('模型已删除')
      loadModels()
    } catch (error: any) {
      message.error('删除模型失败: ' + error.message)
    }
  }

  const columns = [
    {
      title: '模型名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Model) => (
        <Space direction="vertical" size={0}>
          <Space>
            <AppstoreOutlined />
            <Text strong>{text}</Text>
            <Tag color="default">{record.version}</Tag>
          </Space>
          {record.family && <Text type="secondary" style={{ fontSize: 12 }}>{record.family}</Text>}
        </Space>
      ),
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 120,
      render: (type: string) => {
        const typeMap: Record<string, { color: string; label: string }> = {
          llm: { color: 'blue', label: 'LLM' },
          embedding: { color: 'green', label: 'Embedding' },
          rerank: { color: 'orange', label: 'Rerank' },
          cv: { color: 'purple', label: '视觉' },
          asr: { color: 'cyan', label: '语音' },
          audio: { color: 'magenta', label: '音频' },
          video: { color: 'red', label: '视频' },
          custom: { color: 'default', label: '自定义' },
        }
        const config = typeMap[type] || { color: 'default', label: type }
        return <Tag color={config.color}>{config.label}</Tag>
      },
    },
    {
      title: '能力',
      dataIndex: 'abilities',
      key: 'abilities',
      width: 200,
      render: (abilities: string[]) => (
        <Space wrap>
          {abilities?.slice(0, 3).map(ability => (
            <Tag key={ability} style={{ fontSize: 12 }}>{ability}</Tag>
          ))}
          {abilities && abilities.length > 3 && <Tag style={{ fontSize: 12 }}>+{abilities.length - 3}</Tag>}
        </Space>
      ),
    },
    {
      title: '上下文长度',
      dataIndex: 'context_length',
      key: 'context_length',
      width: 120,
      render: (length: number) => length ? `${length}K` : '-',
    },
    {
      title: '模型大小',
      dataIndex: 'size',
      key: 'size',
      width: 100,
      render: (size: number) => size ? `${size}B` : '-',
    },
    {
      title: '推荐规格',
      dataIndex: 'resource_flavor_id',
      key: 'resource_flavor_id',
      width: 200,
      render: (id: string) => <Tag color="cyan">{id}</Tag>,
    },
    {
      title: '推理后端',
      dataIndex: 'inference_backend',
      key: 'inference_backend',
      width: 120,
      render: (v: string) => (v === 'nvidia_nim' ? <Tag color="green">NVIDIA NIM</Tag> : v ? <Tag>{v}</Tag> : '-'),
    },
    {
      title: 'NIM 模型 ID',
      dataIndex: 'nim_model_id',
      key: 'nim_model_id',
      width: 140,
      ellipsis: true,
      render: (v: string) => v || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: any, record: Model) => (
        <Space>
          <Button
            size="small"
            danger
            icon={<DeleteOutlined />}
            onClick={() => handleDelete(record)}
          >
            删除
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div className="models-container">
      <div className="models-header">
        <Title level={2} style={{ margin: 0 }}>
          <AppstoreOutlined /> 模型市场
        </Title>
        <Text type="secondary">统一管理和注册可复用的 AI 模型</Text>
      </div>

      {/* 模型类型分类 Tab */}
      <Card className="models-tabs-card" bordered={false}>
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={MODEL_TYPES.map(type => ({
            key: type.key,
            label: type.label,
          }))}
        />
      </Card>

      <Card
        title={
          <Space>
            <span>模型列表</span>
            <Input
              placeholder="搜索模型名称、描述或家族"
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 300 }}
              allowClear
            />
            <Text type="secondary">共 {filteredModels.length} 个模型</Text>
          </Space>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadModels}>
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateVisible(true)}>
              注册模型
            </Button>
          </Space>
        }
        className="models-table-card"
        bordered={false}
      >
        <Table
          columns={columns}
          dataSource={filteredModels}
          rowKey="id"
          loading={loading}
          locale={{ emptyText: '暂无模型' }}
        />
      </Card>

      {/* 增强的模型注册表单 */}
      <Modal
        title="注册模型"
        open={createVisible}
        onOk={() => form.submit()}
        onCancel={() => {
          setCreateVisible(false)
          form.resetFields()
        }}
        width={800}
        okText="注册"
        cancelText="取消"
      >
        <Form form={form} onFinish={handleCreate} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="name" label="模型名称" rules={[{ required: true, message: '请输入模型名称' }]}>
                <Input placeholder="例如：DeepSeek-R1 或 custom-llm" />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  字母数字字符，可使用连字符和下划线，不能与内置模型名称重复
                </Text>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="version" label="版本" initialValue="v1.0.0">
                <Input placeholder="例如：v1.0.0" />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item name="description" label="模型描述（可选）">
            <TextArea rows={2} placeholder="描述模型的用途、特点等" />
          </Form.Item>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="type" label="模型类型" rules={[{ required: true }]}>
                <Select placeholder="选择模型类型">
                  <Select.Option value="llm">大语言模型 (LLM)</Select.Option>
                  <Select.Option value="embedding">Embedding 模型</Select.Option>
                  <Select.Option value="rerank">Rerank 模型</Select.Option>
                  <Select.Option value="cv">视觉模型</Select.Option>
                  <Select.Option value="asr">语音识别</Select.Option>
                  <Select.Option value="audio">音频模型</Select.Option>
                  <Select.Option value="video">视频模型</Select.Option>
                  <Select.Option value="custom">自定义模型</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="family" label="模型家族（可选）">
                <Input placeholder="例如：llama/gpt/qwen/deepseek" />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  可从内置模型中选择或输入自定义家族名称
                </Text>
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="framework" label="框架" rules={[{ required: true }]}>
                <Select placeholder="选择框架">
                  <Select.Option value="pytorch">PyTorch</Select.Option>
                  <Select.Option value="tensorflow">TensorFlow</Select.Option>
                  <Select.Option value="vllm">vLLM</Select.Option>
                  <Select.Option value="triton">Triton</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="format" label="模型格式">
                <Radio.Group>
                  {MODEL_FORMATS.map(format => (
                    <Radio key={format.value} value={format.value}>{format.label}</Radio>
                  ))}
                </Radio.Group>
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="context_length" label="上下文长度（K tokens）">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="例如：2048" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="size" label="模型大小（Billion parameters）">
                <InputNumber min={1} style={{ width: '100%' }} placeholder="例如：7" />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item name="languages" label="支持的语言">
            <Checkbox.Group>
              <Checkbox value="en">English</Checkbox>
              <Checkbox value="zh">中文</Checkbox>
              <Checkbox value="ja">日本語</Checkbox>
              <Checkbox value="ko">한국어</Checkbox>
              <Checkbox value="es">Español</Checkbox>
              <Checkbox value="fr">Français</Checkbox>
            </Checkbox.Group>
          </Form.Item>

          <Form.Item name="abilities" label="模型能力">
            <Checkbox.Group>
              {MODEL_ABILITIES.map(ability => (
                <Checkbox key={ability.value} value={ability.value}>{ability.label}</Checkbox>
              ))}
            </Checkbox.Group>
          </Form.Item>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="resource_flavor_id" label="推荐资源规格" rules={[{ required: true }]}>
                <Select placeholder="选择推荐资源规格" showSearch>
                  {flavors.map(flavor => (
                    <Select.Option key={flavor.id} value={flavor.id}>
                      {flavor.name} ({flavor.memory_gb}GB)
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="project_id" label="关联项目（可选）">
                <Select placeholder="选择项目" allowClear showSearch>
                  {projects.map(project => (
                    <Select.Option key={project.id} value={project.id}>
                      {project.name}
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
          </Row>

          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="inference_backend" label="推理后端（可选）">
                <Select placeholder="默认 GPU 直连" allowClear>
                  <Select.Option value="gpu">GPU 直连</Select.Option>
                  <Select.Option value="nvidia_nim">NVIDIA NIM</Select.Option>
                </Select>
                <Text type="secondary" style={{ fontSize: 12 }}>选 NIM 时由 NIM 服务承载推理</Text>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="nim_model_id" label="NIM 模型 ID（可选）">
                <Input placeholder="例如：meta/llama-3-70b-instruct" />
                <Text type="secondary" style={{ fontSize: 12 }}>当推理后端为 NVIDIA NIM 时填写</Text>
              </Form.Item>
            </Col>
          </Row>

          <Form.Item name="path" label="模型路径" rules={[{ required: true }]}>
            <Input placeholder="例如：/models/deepseek-r1 或 s3://bucket/path" />
            <Text type="secondary" style={{ fontSize: 12 }}>
              PyTorch 格式提供模型目录，GGUF 格式提供模型文件路径
            </Text>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Models
