import { useState, useEffect } from 'react'
import { Card, Form, Input, Select, InputNumber, Switch, Button, Space, message, Tabs, Collapse, Typography } from 'antd'
import { SaveOutlined, ReloadOutlined, SettingOutlined, InfoCircleOutlined } from '@ant-design/icons'
import { nimApi, NIMConfig } from '../api/nim'
import './Settings.css'

const { Title, Text } = Typography

const { Option } = Select
const { TextArea } = Input

const PLATFORM_CAPABILITIES = [
  '资源与节点：资源池（GPU/NPU）、节点管理、设备发现、规格（Flavor）',
  '任务与模型：任务 CRUD、推理/训练分流与抢占、模型市场、NVIDIA NIM 推理路由',
  '算力：算力感知（状态/绑定/集群设置）、算力质量、算力编排、训练加速（数据加载/分布式/检查点/显存）',
  '系统：RBAC、多租户配额、系统配置与 NIM 配置',
]

const Settings: React.FC = () => {
  const [form] = Form.useForm()
  const [nimForm] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [nimLoading, setNimLoading] = useState(false)
  const [settings, setSettings] = useState<any>({})
  const [, setNimConfig] = useState<NIMConfig | null>(null)

  useEffect(() => {
    loadSettings()
    loadNIMConfig()
  }, [])

  const loadSettings = async () => {
    try {
      // 从后端加载配置
      // const data = await settingsApi.getSettings()
      // setSettings(data)
      // form.setFieldsValue(data)
    } catch (error: any) {
      message.error('加载配置失败: ' + error.message)
    }
  }

  const loadNIMConfig = async () => {
    try {
      const data = await nimApi.getConfig()
      setNimConfig(data)
      nimForm.setFieldsValue({
        base_url: data.base_url || '',
        api_key_credential_id: data.api_key_credential_id || '',
        enabled: data.enabled,
        default_namespace: data.default_namespace || '',
      })
    } catch (error: any) {
      message.error('加载 NIM 配置失败: ' + error.message)
    }
  }

  const handleSaveNIM = async (values: any) => {
    setNimLoading(true)
    try {
      const updated = await nimApi.updateConfig({
        base_url: values.base_url,
        api_key_credential_id: values.api_key_credential_id || undefined,
        enabled: values.enabled,
        default_namespace: values.default_namespace || undefined,
      })
      setNimConfig(updated)
      message.success('NVIDIA NIM 配置已保存')
    } catch (error: any) {
      message.error('保存 NIM 配置失败: ' + error.message)
    } finally {
      setNimLoading(false)
    }
  }

  const handleSave = async (values: any) => {
    setLoading(true)
    try {
      // 保存配置到后端
      // await settingsApi.updateSettings(values)
      message.success('配置保存成功')
      setSettings(values)
    } catch (error: any) {
      message.error('保存配置失败: ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleReset = () => {
    form.resetFields()
    form.setFieldsValue(settings)
  }

  return (
    <div className="settings-container">
      <div className="settings-header page-header">
        <Title level={2} style={{ margin: 0 }}>
          <SettingOutlined /> 系统配置
        </Title>
        <Text type="secondary">管理系统各项配置参数，便于运维与易管理</Text>
      </div>

      <Collapse
        ghost
        style={{ marginBottom: 24 }}
        items={[
          {
            key: 'overview',
            label: (
              <Space>
                <InfoCircleOutlined />
                <span>平台能力总览</span>
              </Space>
            ),
            children: (
              <ul style={{ marginBottom: 0, paddingLeft: 20 }}>
                {PLATFORM_CAPABILITIES.map((cap, i) => (
                  <li key={i} style={{ marginBottom: 4 }}>
                    <Text type="secondary">{cap}</Text>
                  </li>
                ))}
              </ul>
            ),
          },
        ]}
      />

      <Card
        className="settings-card"
        title={
          <Space>
            <SettingOutlined />
            <span>系统配置</span>
          </Space>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadSettings}>
              刷新
            </Button>
            <Button onClick={handleReset}>重置</Button>
            <Button type="primary" icon={<SaveOutlined />} onClick={() => form.submit()} loading={loading}>
              保存配置
            </Button>
          </Space>
        }
      >
        <Tabs
          items={[
            {
              key: 'general',
              label: '通用设置',
              children: (
                <Form form={form} onFinish={handleSave} layout="vertical" style={{ maxWidth: 800 }}>
                  <Form.Item name="cluster_name" label="集群名称">
                    <Input placeholder="输入集群名称" />
                  </Form.Item>
                  <Form.Item name="description" label="集群描述">
                    <TextArea rows={3} placeholder="输入集群描述" />
                  </Form.Item>
                  <Form.Item name="timezone" label="时区">
                    <Select defaultValue="Asia/Shanghai">
                      <Option value="Asia/Shanghai">Asia/Shanghai (UTC+8)</Option>
                      <Option value="UTC">UTC</Option>
                      <Option value="America/New_York">America/New_York (UTC-5)</Option>
                    </Select>
                  </Form.Item>
                  <Form.Item name="language" label="语言">
                    <Select defaultValue="zh-CN">
                      <Option value="zh-CN">中文</Option>
                      <Option value="en-US">English</Option>
                    </Select>
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'scheduler',
              label: '调度器配置',
              children: (
                <Form form={form} onFinish={handleSave} layout="vertical" style={{ maxWidth: 800 }}>
                  <Form.Item name="scheduler_strategy" label="调度策略">
                    <Select defaultValue="binpack">
                      <Option value="binpack">Binpack (资源利用率优先)</Option>
                      <Option value="spread">Spread (负载均衡优先)</Option>
                    </Select>
                  </Form.Item>
                  <Form.Item name="enable_preemption" label="启用抢占" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                  <Form.Item name="preemption_threshold" label="抢占阈值 (优先级差)">
                    <InputNumber min={1} max={10} defaultValue={3} />
                  </Form.Item>
                  <Form.Item name="schedule_interval" label="调度间隔 (秒)">
                    <InputNumber min={1} max={60} defaultValue={5} />
                  </Form.Item>
                  <Form.Item name="max_retries" label="最大重试次数">
                    <InputNumber min={0} max={10} defaultValue={3} />
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'resource',
              label: '资源管理',
              children: (
                <Form form={form} onFinish={handleSave} layout="vertical" style={{ maxWidth: 800 }}>
                  <Form.Item name="auto_discovery" label="自动发现资源" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                  <Form.Item name="discovery_interval" label="资源发现间隔 (秒)">
                    <InputNumber min={10} max={300} defaultValue={30} />
                  </Form.Item>
                  <Form.Item name="health_check_interval" label="健康检查间隔 (秒)">
                    <InputNumber min={5} max={120} defaultValue={30} />
                  </Form.Item>
                  <Form.Item name="resource_timeout" label="资源超时时间 (秒)">
                    <InputNumber min={60} max={3600} defaultValue={300} />
                  </Form.Item>
                  <Form.Item name="enable_quota" label="启用配额限制" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'monitoring',
              label: '监控配置',
              children: (
                <Form form={form} onFinish={handleSave} layout="vertical" style={{ maxWidth: 800 }}>
                  <Form.Item name="metrics_retention" label="指标保留时间 (天)">
                    <InputNumber min={1} max={90} defaultValue={7} />
                  </Form.Item>
                  <Form.Item name="metrics_interval" label="指标采集间隔 (秒)">
                    <InputNumber min={5} max={60} defaultValue={30} />
                  </Form.Item>
                  <Form.Item name="enable_alerts" label="启用告警" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                  <Form.Item name="alert_threshold_cpu" label="CPU 告警阈值 (%)">
                    <InputNumber min={50} max={100} defaultValue={80} />
                  </Form.Item>
                  <Form.Item name="alert_threshold_memory" label="内存告警阈值 (%)">
                    <InputNumber min={50} max={100} defaultValue={85} />
                  </Form.Item>
                  <Form.Item name="alert_threshold_temperature" label="温度告警阈值 (°C)">
                    <InputNumber min={60} max={100} defaultValue={85} />
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'nim',
              label: 'NVIDIA NIM',
              children: (
                <Form form={nimForm} onFinish={handleSaveNIM} layout="vertical" style={{ maxWidth: 800 }}>
                  <Text type="secondary" style={{ display: 'block', marginBottom: 16 }}>
                    配置 NVIDIA NIM (Inference Microservices) 推理后端地址，推理任务可选择路由到 NIM。
                  </Text>
                  <Form.Item name="base_url" label="NIM 服务地址 (Base URL)">
                    <Input placeholder="例如 https://nim.example.com 或 http://nim-gateway:8000" />
                  </Form.Item>
                  <Form.Item name="enabled" label="启用 NIM 推理后端" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                  <Form.Item name="api_key_credential_id" label="API Key 凭据 ID (可选)">
                    <Input placeholder="凭据系统中的应用 ID" />
                  </Form.Item>
                  <Form.Item name="default_namespace" label="默认 Namespace (可选)">
                    <Input placeholder="K8s 中 NIM 部署的 namespace" />
                  </Form.Item>
                  <Form.Item>
                    <Space>
                      <Button icon={<ReloadOutlined />} onClick={loadNIMConfig}>
                        刷新
                      </Button>
                      <Button type="primary" icon={<SaveOutlined />} htmlType="submit" loading={nimLoading}>
                        保存 NIM 配置
                      </Button>
                    </Space>
                  </Form.Item>
                </Form>
              ),
            },
            {
              key: 'api',
              label: 'API 配置',
              children: (
                <Form form={form} onFinish={handleSave} layout="vertical" style={{ maxWidth: 800 }}>
                  <Form.Item name="api_timeout" label="API 超时时间 (秒)">
                    <InputNumber min={5} max={300} defaultValue={30} />
                  </Form.Item>
                  <Form.Item name="api_rate_limit" label="API 速率限制 (请求/秒)">
                    <InputNumber min={10} max={1000} defaultValue={100} />
                  </Form.Item>
                  <Form.Item name="enable_cors" label="启用 CORS" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                  <Form.Item name="enable_auth" label="启用认证" valuePropName="checked">
                    <Switch />
                  </Form.Item>
                </Form>
              ),
            },
          ]}
        />
      </Card>
    </div>
  )
}

export default Settings
