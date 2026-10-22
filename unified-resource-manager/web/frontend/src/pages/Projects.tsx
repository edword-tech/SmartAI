import { useEffect, useState } from 'react'
import { Card, Table, Button, Space, Tag, Modal, Form, Input, message, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined, DeleteOutlined, FolderOutlined, EditOutlined } from '@ant-design/icons'
import { projectsApi, Project, CreateProjectRequest } from '../api/projects'
import './Projects.css'

const { Title, Text } = Typography
const { TextArea } = Input

const Projects: React.FC = () => {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(false)
  const [createVisible, setCreateVisible] = useState(false)
  const [editVisible, setEditVisible] = useState(false)
  const [editingProject, setEditingProject] = useState<Project | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    loadProjects()
  }, [])

  const loadProjects = async () => {
    setLoading(true)
    try {
      const data = await projectsApi.listProjects()
      setProjects(data)
    } catch (error: any) {
      message.error('加载项目失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = async (values: any) => {
    const req: CreateProjectRequest = {
      name: values.name,
      description: values.description,
      tenant_id: values.tenant_id,
      owner_id: values.owner_id,
    }
    try {
      await projectsApi.createProject(req)
      message.success('项目创建成功')
      setCreateVisible(false)
      form.resetFields()
      loadProjects()
    } catch (error: any) {
      message.error('创建项目失败: ' + error.message)
    }
  }

  const handleEdit = (project: Project) => {
    setEditingProject(project)
    form.setFieldsValue({
      name: project.name,
      description: project.description,
      tenant_id: project.tenant_id,
      owner_id: project.owner_id,
    })
    setEditVisible(true)
  }

  const handleUpdate = async (values: any) => {
    if (!editingProject) return
    try {
      await projectsApi.updateProject(editingProject.id, values)
      message.success('项目更新成功')
      setEditVisible(false)
      setEditingProject(null)
      form.resetFields()
      loadProjects()
    } catch (error: any) {
      message.error('更新项目失败: ' + error.message)
    }
  }

  const handleDelete = async (record: Project) => {
    try {
      await projectsApi.deleteProject(record.id)
      message.success('项目已删除')
      loadProjects()
    } catch (error: any) {
      message.error('删除项目失败: ' + error.message)
    }
  }

  const columns = [
    {
      title: '项目名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => (
        <Space>
          <FolderOutlined />
          <Text strong>{text}</Text>
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '租户ID',
      dataIndex: 'tenant_id',
      key: 'tenant_id',
      width: 150,
      render: (id: string) => id ? <Tag>{id}</Tag> : '-',
    },
    {
      title: '所有者ID',
      dataIndex: 'owner_id',
      key: 'owner_id',
      width: 150,
      render: (id: string) => id ? <Tag>{id}</Tag> : '-',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (time: string) => time ? new Date(time).toLocaleString() : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_: any, record: Project) => (
        <Space>
          <Button
            size="small"
            icon={<EditOutlined />}
            onClick={() => handleEdit(record)}
          >
            编辑
          </Button>
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
    <div className="projects-container">
      <div className="projects-header">
        <Title level={2} style={{ margin: 0 }}>
          <FolderOutlined /> 项目管理
        </Title>
        <Text type="secondary">组织和管理 AI 项目，关联模型、任务和资源</Text>
      </div>

      <Card
        title="项目列表"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadProjects}>
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateVisible(true)}>
              创建项目
            </Button>
          </Space>
        }
        className="projects-table-card"
        bordered={false}
      >
        <Table
          columns={columns}
          dataSource={projects}
          rowKey="id"
          loading={loading}
          locale={{ emptyText: '暂无项目' }}
        />
      </Card>

      {/* 创建项目表单 */}
      <Modal
        title="创建项目"
        open={createVisible}
        onOk={() => form.submit()}
        onCancel={() => {
          setCreateVisible(false)
          form.resetFields()
        }}
        width={600}
      >
        <Form form={form} onFinish={handleCreate} layout="vertical">
          <Form.Item name="name" label="项目名称" rules={[{ required: true }]}>
            <Input placeholder="例如：DeepSeek-R1 推理服务" />
          </Form.Item>
          <Form.Item name="description" label="项目描述">
            <TextArea rows={3} placeholder="描述项目的用途和目标" />
          </Form.Item>
          <Form.Item name="tenant_id" label="租户ID（可选）">
            <Input placeholder="关联的租户ID" />
          </Form.Item>
          <Form.Item name="owner_id" label="所有者ID（可选）">
            <Input placeholder="项目所有者ID" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 编辑项目表单 */}
      <Modal
        title="编辑项目"
        open={editVisible}
        onOk={() => form.submit()}
        onCancel={() => {
          setEditVisible(false)
          setEditingProject(null)
          form.resetFields()
        }}
        width={600}
      >
        <Form form={form} onFinish={handleUpdate} layout="vertical">
          <Form.Item name="name" label="项目名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="项目描述">
            <TextArea rows={3} />
          </Form.Item>
          <Form.Item name="tenant_id" label="租户ID">
            <Input />
          </Form.Item>
          <Form.Item name="owner_id" label="所有者ID">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Projects
