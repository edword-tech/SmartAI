import { useEffect, useState } from 'react'
import {
  Card, Row, Col, Statistic, Table, Tag, Button, Space, Switch, InputNumber,
  message, Typography, Tabs, Form, Input, Modal, Progress,
} from 'antd'
import { ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import {
  schedulerApi, SchedulerStats, SchedulerConfig, Queue, FragmentationReport, DefragPlan,
} from '../api/scheduler'

const { Title, Text } = Typography

const SchedulerPage: React.FC = () => {
  const [stats, setStats] = useState<SchedulerStats | null>(null)
  const [config, setConfig] = useState<SchedulerConfig | null>(null)
  const [queues, setQueues] = useState<Queue[]>([])
  const [frag, setFrag] = useState<FragmentationReport | null>(null)
  const [plan, setPlan] = useState<DefragPlan | null>(null)
  const [loading, setLoading] = useState(false)
  const [queueOpen, setQueueOpen] = useState(false)
  const [form] = Form.useForm()

  const load = async () => {
    setLoading(true)
    try {
      const [s, c, q, f] = await Promise.all([
        schedulerApi.getStats(),
        schedulerApi.getConfig(),
        schedulerApi.listQueues(),
        schedulerApi.getFragmentation(),
      ])
      setStats(s)
      setConfig(c)
      setQueues(Array.isArray(q) ? q : [])
      setFrag(f)
    } catch (e: any) {
      message.error('加载调度器失败: ' + e.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    const t = setInterval(load, 8000)
    return () => clearInterval(t)
  }, [])

  const saveConfig = async () => {
    if (!config) return
    try {
      const next = await schedulerApi.updateConfig(config)
      setConfig(next)
      message.success('调度配置已更新')
    } catch (e: any) {
      message.error(e.message)
    }
  }

  const runDefrag = async (execute: boolean) => {
    try {
      const p = execute ? await schedulerApi.executeDefrag() : await schedulerApi.planDefrag()
      setPlan(p)
      message.success(execute ? `已迁移 ${p.moves?.length || 0} 个实例` : '已生成整理计划')
      load()
    } catch (e: any) {
      message.error(e.message)
    }
  }

  const createQueue = async (v: any) => {
    try {
      await schedulerApi.upsertQueue({
        name: v.name,
        tenant_id: v.tenant_id,
        weight: v.weight || 1,
        reclaimable: v.reclaimable !== false,
        guaranteed: { gpu: v.g_gpu || 0, npu: v.g_npu || 0, memory_gb: v.g_mem || 0 },
        capability: { gpu: v.c_gpu || 0, npu: v.c_npu || 0, memory_gb: v.c_mem || 0 },
      })
      message.success('队列已保存')
      setQueueOpen(false)
      form.resetFields()
      load()
    } catch (e: any) {
      message.error(e.message)
    }
  }

  return (
    <div className="page-header">
      <Title level={2} style={{ margin: 0 }}><ThunderboltOutlined /> 智能调度</Title>
      <Text type="secondary">K8s Scheduler Framework 风格：队列 / Gang / 拓扑 / 抢占 / 超卖 / 碎片整理</Text>

      <Row gutter={[16, 16]} style={{ margin: '24px 0' }}>
        {[
          ['已调度', stats?.scheduled ?? 0, '#1677ff'],
          ['运行中', stats?.running_tasks ?? 0, '#52c41a'],
          ['排队中', stats?.pending_tasks ?? 0, '#faad14'],
          ['抢占次数', stats?.preemptions ?? 0, '#722ed1'],
          ['故障重调度', stats?.reschedules ?? 0, '#eb2f96'],
          ['周期耗时 ms', Number(stats?.last_cycle_millis ?? 0).toFixed(2), '#13c2c2'],
        ].map(([title, value, color]) => (
          <Col xs={12} md={8} lg={4} key={String(title)}>
            <Card bordered={false}><Statistic title={title as string} value={value as number} valueStyle={{ color: color as string }} /></Card>
          </Col>
        ))}
      </Row>

      <Card bordered={false}>
        <Tabs
          items={[
            {
              key: 'queues',
              label: '调度队列',
              children: (
                <>
                  <Space style={{ marginBottom: 12 }}>
                    <Button type="primary" onClick={() => setQueueOpen(true)}>新建队列</Button>
                    <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
                  </Space>
                  <Table
                    rowKey="name"
                    loading={loading}
                    dataSource={queues}
                    pagination={false}
                    columns={[
                      { title: '队列', dataIndex: 'name' },
                      { title: '租户', dataIndex: 'tenant_id', render: (v) => v || '-' },
                      { title: '权重', dataIndex: 'weight' },
                      { title: '运行/排队', render: (_: unknown, r: Queue) => `${r.running} / ${r.pending}` },
                      { title: '已分配 GPU', render: (_: unknown, r: Queue) => r.allocated?.gpu ?? 0 },
                      { title: '保障/上限 GPU', render: (_: unknown, r: Queue) => `${r.guaranteed?.gpu || '∞'} / ${r.capability?.gpu || '∞'}` },
                      { title: '主导份额', render: (_: unknown, r: Queue) => (r.dominant_share || 0).toFixed(3) },
                      { title: '可回收', dataIndex: 'reclaimable', render: (v: boolean) => <Tag color={v ? 'blue' : 'default'}>{v ? '是' : '否'}</Tag> },
                      {
                        title: '操作',
                        render: (_: unknown, r: Queue) => r.name !== 'default' ? (
                          <Button size="small" danger onClick={async () => { await schedulerApi.deleteQueue(r.name); load() }}>删除</Button>
                        ) : null,
                      },
                    ]}
                  />
                </>
              ),
            },
            {
              key: 'config',
              label: '插件与策略',
              children: config && (
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                  <Row gutter={16}>
                    <Col span={6}><Switch checked={config.enable_preemption} onChange={(v) => setConfig({ ...config, enable_preemption: v })} /> 抢占</Col>
                    <Col span={6}><Switch checked={config.enable_gang} onChange={(v) => setConfig({ ...config, enable_gang: v })} /> Gang 调度</Col>
                    <Col span={6}><Switch checked={config.enable_elastic} onChange={(v) => setConfig({ ...config, enable_elastic: v })} /> 弹性扩缩</Col>
                    <Col span={6}><Switch checked={config.enable_backfill} onChange={(v) => setConfig({ ...config, enable_backfill: v })} /> 回填</Col>
                  </Row>
                  <Space>
                    算力超卖
                    <InputNumber min={1} max={8} step={0.1} value={config.core_overcommit} onChange={(v) => setConfig({ ...config, core_overcommit: Number(v) })} />
                    饥饿阈值(秒)
                    <InputNumber min={0} value={config.starvation_seconds} onChange={(v) => setConfig({ ...config, starvation_seconds: Number(v) })} />
                  </Space>
                  <Table
                    rowKey="name"
                    pagination={false}
                    dataSource={Object.entries(config.scores || {}).map(([name, p]) => ({ name, ...p }))}
                    columns={[
                      { title: '打分插件', dataIndex: 'name' },
                      { title: '启用', dataIndex: 'enabled', render: (v: boolean, r: any) => (
                        <Switch checked={v} onChange={(on) => setConfig({
                          ...config,
                          scores: { ...config.scores, [r.name]: { ...config.scores[r.name], enabled: on } },
                        })} />
                      ) },
                      { title: '权重', dataIndex: 'weight', render: (v: number, r: any) => (
                        <InputNumber min={0} max={10} value={v} onChange={(n) => setConfig({
                          ...config,
                          scores: { ...config.scores, [r.name]: { ...config.scores[r.name], weight: Number(n) } },
                        })} />
                      ) },
                    ]}
                  />
                  <Button type="primary" onClick={saveConfig}>保存配置</Button>
                </Space>
              ),
            },
            {
              key: 'defrag',
              label: '碎片整理',
              children: (
                <>
                  <Space style={{ marginBottom: 16 }}>
                    <Button onClick={() => runDefrag(false)}>生成计划</Button>
                    <Button type="primary" onClick={() => runDefrag(true)}>执行整理</Button>
                  </Space>
                  <Row gutter={16}>
                    {(frag?.stats || []).map((st) => (
                      <Col span={12} key={st.resource_type}>
                        <Card size="small" title={`${st.resource_type.toUpperCase()} 碎片率`}>
                          <Progress percent={Math.round((st.fragmentation_ratio || 0) * 100)} />
                          <Text type="secondary">空闲 {st.idle_devices} / 部分占用 {st.partial_devices} / 打满 {st.full_devices}，碎片显存 {st.fragmented_memory_gb} GB</Text>
                        </Card>
                      </Col>
                    ))}
                  </Row>
                  {plan && (
                    <Table
                      style={{ marginTop: 16 }}
                      rowKey={(r) => `${r.task_id}-${r.from}`}
                      dataSource={plan.moves || []}
                      pagination={false}
                      columns={[
                        { title: '任务', dataIndex: 'task_id' },
                        { title: '从', dataIndex: 'from' },
                        { title: '到', dataIndex: 'to' },
                      ]}
                    />
                  )}
                </>
              ),
            },
          ]}
        />
      </Card>

      <Modal title="新建队列" open={queueOpen} onOk={() => form.submit()} onCancel={() => setQueueOpen(false)}>
        <Form form={form} layout="vertical" onFinish={createQueue}>
          <Form.Item name="name" label="队列名" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="tenant_id" label="归属租户"><Input /></Form.Item>
          <Form.Item name="weight" label="权重" initialValue={1}><InputNumber min={1} style={{ width: '100%' }} /></Form.Item>
          <Form.Item name="g_gpu" label="保障 GPU"><InputNumber min={0} style={{ width: '100%' }} /></Form.Item>
          <Form.Item name="c_gpu" label="上限 GPU（0 不限）"><InputNumber min={0} style={{ width: '100%' }} /></Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default SchedulerPage
