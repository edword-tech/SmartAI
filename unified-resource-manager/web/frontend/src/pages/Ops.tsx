import { useEffect, useState } from 'react'
import {
  Card, Row, Col, Statistic, Table, Tag, Button, Space, Switch, Select,
  message, Typography, Tabs, Input,
} from 'antd'
import { AlertOutlined, ExperimentOutlined, MedicineBoxOutlined, ReloadOutlined } from '@ant-design/icons'
import {
  opsApi, OpsOverview, DeviceTelemetry, OpsEvent, OpsRule, DeviceCondition,
  BenchmarkRun, DiagnosisReport, ProbeReport, ProbeResult, Readiness,
} from '../api/ops'
import { nodesApi, Node } from '../api/nodes'

const { Title, Text, Paragraph } = Typography

const levelColor: Record<string, string> = { info: 'blue', warning: 'orange', critical: 'red', pass: 'green', warn: 'orange', fail: 'red' }

const OpsPage: React.FC = () => {
  const [overview, setOverview] = useState<OpsOverview | null>(null)
  const [telemetry, setTelemetry] = useState<DeviceTelemetry[]>([])
  const [events, setEvents] = useState<OpsEvent[]>([])
  const [rules, setRules] = useState<OpsRule[]>([])
  const [conditions, setConditions] = useState<DeviceCondition[]>([])
  const [runs, setRuns] = useState<BenchmarkRun[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [report, setReport] = useState<DiagnosisReport | null>(null)
  const [probe, setProbe] = useState<ProbeReport | null>(null)
  const [readiness, setReadiness] = useState<Readiness[]>([])
  const [probing, setProbing] = useState(false)
  const [question, setQuestion] = useState('')
  const [nodeId, setNodeId] = useState<string>()
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const [o, t, e, r, c, b, n, ready] = await Promise.all([
        opsApi.overview(),
        opsApi.listTelemetry(),
        opsApi.listEvents({ limit: '80' }),
        opsApi.listRules(),
        opsApi.listConditions(),
        opsApi.listRuns(),
        nodesApi.listNodes(),
        opsApi.readiness(),
      ])
      setReadiness(Array.isArray(ready) ? ready : [])
      const last = (o as any)?.last_probe
      if (last?.results) setProbe(last)
      setOverview(o)
      setTelemetry(Array.isArray(t) ? t : [])
      setEvents(Array.isArray(e) ? e : [])
      setRules(Array.isArray(r) ? r : [])
      setConditions(Array.isArray(c) ? c : [])
      setRuns(Array.isArray(b) ? b : [])
      setNodes(Array.isArray(n) ? n : [])
    } catch (err: any) {
      message.error('加载运维数据失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    const timer = setInterval(load, 10000)
    return () => clearInterval(timer)
  }, [])

  const inject = async (id: string, kind: string) => {
    const fault: Record<string, number> = kind === 'xid79' ? { xid: 79 }
      : kind === 'temp' ? { temperature_add: 55 }
      : kind === 'dbe' ? { ecc_dbe: 1 }
      : { nvlink_crc_errors: 80 }
    try {
      await opsApi.injectFault(id, fault)
      message.success('已注入故障，自愈引擎将在下一采集周期处置')
      setTimeout(load, 1200)
    } catch (e: any) {
      message.error(e.message)
    }
  }

  return (
    <div className="page-header">
      <Title level={2} style={{ margin: 0 }}><AlertOutlined /> 集群运维</Title>
      <Text type="secondary">遥测感知 · 故障自愈 · GPU/RDMA 压测基线 · 规则+LLM 诊断</Text>

      <Row gutter={[16, 16]} style={{ margin: '24px 0' }}>
        <Col xs={12} lg={4}><Card bordered={false}><Statistic title="已检测故障" value={overview?.self_heal?.faults_detected ?? 0} /></Card></Col>
        <Col xs={12} lg={4}><Card bordered={false}><Statistic title="自动处置" value={overview?.self_heal?.auto_remediated ?? 0} valueStyle={{ color: '#52c41a' }} /></Card></Col>
        <Col xs={12} lg={4}><Card bordered={false}><Statistic title="自愈率" value={Math.round((overview?.self_heal?.self_heal_rate ?? 0) * 100)} suffix="%" /></Card></Col>
        <Col xs={12} lg={4}><Card bordered={false}><Statistic title="待人工 reset" value={overview?.self_heal?.pending_manual ?? 0} valueStyle={{ color: '#cf1322' }} /></Card></Col>
        <Col xs={12} lg={4}><Card bordered={false}><Statistic title="隔离设备" value={overview?.self_heal?.isolated ?? 0} /></Card></Col>
        <Col xs={12} lg={4}><Card bordered={false}><Statistic title="数据源" value={overview?.telemetry_source || '-'} /></Card></Col>
      </Row>

      <Card bordered={false}>
        <Tabs items={[
          {
            key: 'probe',
            label: 'GPU/NPU 调测',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size={16}>
                <Row gutter={16}>
                  {readiness.map((r) => (
                    <Col span={12} key={r.resource_type}>
                      <Card size="small" title={r.resource_type.toUpperCase()}>
                        <Tag color={r.ready ? 'green' : 'red'}>{r.ready ? '可调测' : '不可用'}</Tag>
                        <div style={{ marginTop: 8 }}>
                          设备 {r.total} · 空闲 {r.available} · 预留 {r.debug_reserved}（就绪 {r.debug_reserved_ready}）
                        </div>
                        <Text type="secondary">{r.message}</Text>
                      </Card>
                    </Col>
                  ))}
                </Row>
                <Space>
                  <Button type="primary" loading={probing} onClick={async () => {
                    setProbing(true)
                    try {
                      const rep = await opsApi.runProbe()
                      setProbe(rep)
                      message.success(rep.summary)
                      const ready = await opsApi.readiness()
                      setReadiness(Array.isArray(ready) ? ready : [])
                    } catch (err: any) {
                      message.error(err.message)
                    } finally {
                      setProbing(false)
                    }
                  }}>一键调测 GPU + NPU</Button>
                  <Button onClick={async () => { setProbe(await opsApi.runProbe(['gpu'])) }}>只测 GPU</Button>
                  <Button onClick={async () => { setProbe(await opsApi.runProbe(['npu'])) }}>只测 NPU</Button>
                </Space>
                {probe && (
                  <>
                    <Text>{probe.summary}</Text>
                    <Table
                      rowKey="resource_type"
                      pagination={false}
                      dataSource={probe.results}
                      expandable={{
                        expandedRowRender: (r: ProbeResult) => (
                          <Table
                            size="small"
                            rowKey="name"
                            pagination={false}
                            dataSource={r.checks}
                            columns={[
                              { title: '步骤', dataIndex: 'name' },
                              { title: '结果', dataIndex: 'status', render: (v: string) => <Tag color={levelColor[v] || 'default'}>{v}</Tag> },
                              { title: '详情', dataIndex: 'detail' },
                            ]}
                          />
                        ),
                      }}
                      columns={[
                        { title: '类型', dataIndex: 'resource_type', render: (v: string) => v.toUpperCase() },
                        { title: '结论', dataIndex: 'status', render: (v: string) => <Tag color={levelColor[v]}>{v}</Tag> },
                        { title: '预留卡', dataIndex: 'reserved_id' },
                        { title: '实测卡', dataIndex: 'allocated_id' },
                        { title: '节点', dataIndex: 'node_id' },
                        { title: '型号', dataIndex: 'model' },
                        { title: '耗时', dataIndex: 'latency_ms', render: (v: number) => `${v?.toFixed(1)} ms` },
                        { title: '错误', dataIndex: 'error' },
                      ]}
                    />
                  </>
                )}
              </Space>
            ),
          },
          {
            key: 'telemetry',
            label: '设备遥测',
            children: (
              <Table
                rowKey="resource_id"
                loading={loading}
                dataSource={telemetry}
                columns={[
                  { title: '设备', dataIndex: 'resource_id' },
                  { title: '型号', dataIndex: 'model' },
                  { title: '利用率', dataIndex: 'utilization_pct', render: (v: number) => `${(v || 0).toFixed(0)}%` },
                  { title: '温度', dataIndex: 'temperature_c', render: (v: number) => <span style={{ color: v >= 85 ? '#cf1322' : undefined }}>{v?.toFixed(1)}°C</span> },
                  { title: '功耗', render: (_: unknown, r: DeviceTelemetry) => `${r.power_w?.toFixed(0)} / ${r.power_limit_w} W` },
                  { title: 'XID', dataIndex: 'xid', render: (v: number) => v ? <Tag color="red">{v}</Tag> : '-' },
                  { title: 'ECC DBE', dataIndex: 'ecc_dbe_total' },
                  { title: 'NVLink CRC', dataIndex: 'nvlink_crc_errors_total' },
                  {
                    title: '演练',
                    render: (_: unknown, r: DeviceTelemetry) => (
                      <Space>
                        <Button size="small" onClick={() => inject(r.resource_id, 'temp')}>过温</Button>
                        <Button size="small" danger onClick={() => inject(r.resource_id, 'xid79')}>掉卡</Button>
                      </Space>
                    ),
                  },
                ]}
              />
            ),
          },
          {
            key: 'events',
            label: '运维事件',
            children: (
              <Table
                rowKey="id"
                dataSource={events}
                columns={[
                  { title: '时间', dataIndex: 'time', render: (t: string) => t ? new Date(t).toLocaleString('zh-CN') : '-' },
                  { title: '级别', dataIndex: 'level', render: (v: string) => <Tag color={levelColor[v]}>{v}</Tag> },
                  { title: '标题', dataIndex: 'title' },
                  { title: '对象', render: (_: unknown, r: OpsEvent) => r.resource_id || r.node_id },
                  { title: '说明', dataIndex: 'message' },
                  { title: '动作', dataIndex: 'actions', render: (a: string[]) => (a || []).join('；') },
                ]}
              />
            ),
          },
          {
            key: 'rules',
            label: '自愈规则',
            children: (
              <>
                <Table
                  rowKey="id"
                  dataSource={rules}
                  pagination={false}
                  columns={[
                    { title: '规则', dataIndex: 'name' },
                    { title: '级别', dataIndex: 'severity', render: (v: string) => <Tag color={levelColor[v]}>{v}</Tag> },
                    { title: '启用', dataIndex: 'enabled', render: (v: boolean, r: OpsRule) => (
                      <Switch checked={v} onChange={async (on) => { await opsApi.updateRule(r.id, { enabled: on }); load() }} />
                    ) },
                    { title: '自动处置', dataIndex: 'auto_remediate', render: (v: boolean, r: OpsRule) => (
                      <Switch checked={v} onChange={async (on) => { await opsApi.updateRule(r.id, { auto_remediate: on }); load() }} />
                    ) },
                    { title: '动作', dataIndex: 'action' },
                  ]}
                />
                <Title level={5} style={{ marginTop: 24 }}>当前故障条件</Title>
                <Table
                  rowKey="resource_id"
                  dataSource={conditions}
                  pagination={false}
                  locale={{ emptyText: '无活动故障' }}
                  columns={[
                    { title: '设备', dataIndex: 'resource_id' },
                    { title: '原因', dataIndex: 'reason' },
                    { title: '隔离', dataIndex: 'isolated', render: (v: boolean) => v ? <Tag color="red">是</Tag> : '否' },
                    { title: '需 reset', dataIndex: 'needs_reset', render: (v: boolean) => v ? <Tag color="red">是</Tag> : '否' },
                    {
                      title: '操作',
                      render: (_: unknown, r: DeviceCondition) => r.needs_reset ? (
                        <Button size="small" type="primary" onClick={async () => { await opsApi.resetDevice(r.resource_id, 'web'); load() }}>确认 reset</Button>
                      ) : null,
                    },
                  ]}
                />
              </>
            ),
          },
          {
            key: 'bench',
            label: '压测与验收',
            children: (
              <>
                <Space style={{ marginBottom: 12 }}>
                  <Select
                    style={{ width: 280 }}
                    placeholder="选择节点"
                    value={nodeId}
                    onChange={setNodeId}
                    options={nodes.map((n) => ({ value: n.id, label: `${n.name} (${n.id})` }))}
                  />
                  <Button type="primary" icon={<ExperimentOutlined />} disabled={!nodeId} onClick={async () => {
                    if (!nodeId) return
                    const run = await opsApi.runBenchmark(nodeId, 'benchmark')
                    message.success(`压测完成：${run.verdict}`)
                    load()
                  }}>跑压测</Button>
                  <Button disabled={!nodeId} onClick={async () => {
                    if (!nodeId) return
                    const run = await opsApi.runBenchmark(nodeId, 'acceptance')
                    message.info(`验收 ${run.verdict === 'fail' ? '未通过，节点已隔离' : '通过'}`)
                    load()
                  }}>交付验收</Button>
                  <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
                </Space>
                <Table
                  rowKey="id"
                  dataSource={runs}
                  expandable={{
                    expandedRowRender: (run: BenchmarkRun) => (
                      <Table
                        size="small"
                        rowKey="id"
                        pagination={false}
                        dataSource={run.results}
                        columns={[
                          { title: '项', dataIndex: 'test' },
                          { title: '对象', dataIndex: 'target' },
                          { title: '实测', render: (_: unknown, r) => `${r.value} ${r.unit}` },
                          { title: '基线', render: (_: unknown, r) => `${r.expected} ${r.unit}` },
                          { title: '比例', dataIndex: 'ratio', render: (v: number) => `${Math.round((v || 0) * 100)}%` },
                          { title: '结果', dataIndex: 'status', render: (v: string) => <Tag color={levelColor[v]}>{v}</Tag> },
                          { title: '归因', dataIndex: 'attribution' },
                        ]}
                      />
                    ),
                  }}
                  columns={[
                    { title: '运行', dataIndex: 'id', width: 220 },
                    { title: '节点', dataIndex: 'node_id' },
                    { title: '类型', dataIndex: 'kind' },
                    { title: '结论', dataIndex: 'verdict', render: (v: string) => <Tag color={levelColor[v]}>{v}</Tag> },
                    { title: '摘要', dataIndex: 'summary', render: (s: string[]) => s?.[0] },
                  ]}
                />
              </>
            ),
          },
          {
            key: 'diag',
            label: '故障诊断',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size={16}>
                <Space wrap>
                  <Select
                    allowClear
                    style={{ width: 260 }}
                    placeholder="限定节点（可选）"
                    value={nodeId}
                    onChange={setNodeId}
                    options={nodes.map((n) => ({ value: n.id, label: n.name }))}
                  />
                  <Input
                    style={{ width: 360 }}
                    placeholder="补充问题，例如：这台 H100 训练带宽为什么掉了一半？"
                    value={question}
                    onChange={(e) => setQuestion(e.target.value)}
                  />
                  <Button type="primary" icon={<MedicineBoxOutlined />} onClick={async () => {
                    const rep = await opsApi.diagnose({ node_id: nodeId, question })
                    setReport(rep)
                  }}>生成诊断</Button>
                </Space>
                {report && (
                  <Card size="small" title={`${report.scope} · ${report.summary}`}>
                    <Table
                      size="small"
                      rowKey={(r) => r.target + r.symptom}
                      pagination={false}
                      dataSource={report.findings}
                      columns={[
                        { title: '级别', dataIndex: 'severity', render: (v: string) => <Tag color={levelColor[v]}>{v}</Tag> },
                        { title: '对象', dataIndex: 'target' },
                        { title: '症状', dataIndex: 'symptom' },
                        { title: '根因', dataIndex: 'root_cause' },
                        { title: '建议', dataIndex: 'suggestion' },
                      ]}
                    />
                    {report.llm_analysis && (
                      <Paragraph style={{ marginTop: 16, whiteSpace: 'pre-wrap' }}>
                        <Text strong>LLM（{report.llm_model}）分析：</Text><br />{report.llm_analysis}
                      </Paragraph>
                    )}
                    {report.llm_error && <Text type="secondary">LLM 未启用或调用失败：{report.llm_error}</Text>}
                  </Card>
                )}
              </Space>
            ),
          },
        ]} />
      </Card>
    </div>
  )
}

export default OpsPage
