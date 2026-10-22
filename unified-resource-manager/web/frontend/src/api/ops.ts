import client from './client'

export interface DeviceTelemetry {
  resource_id: string
  node_id: string
  device_id: string
  model: string
  type: string
  timestamp: string
  utilization_pct: number
  memory_used_gb: number
  memory_total_gb: number
  temperature_c: number
  power_w: number
  power_limit_w: number
  ecc_sbe_total: number
  ecc_dbe_total: number
  xid?: number
  pcie_replay_total: number
  nvlink_crc_errors_total: number
  throttle?: string[]
  source: string
}

export interface OpsEvent {
  id: string
  time: string
  level: 'info' | 'warning' | 'critical'
  category: string
  rule_id?: string
  node_id?: string
  resource_id?: string
  title: string
  message: string
  actions?: string[]
  affected_tasks?: string[]
}

export interface OpsRule {
  id: string
  name: string
  description: string
  severity: string
  enabled: boolean
  auto_remediate: boolean
  threshold?: number
  unit?: string
  action: string
}

export interface DeviceCondition {
  resource_id: string
  node_id: string
  rule_id: string
  reason: string
  isolated: boolean
  degraded: boolean
  needs_reset: boolean
  since: string
}

export interface SelfHealStats {
  faults_detected: number
  auto_remediated: number
  auto_recovered: number
  manual_resets: number
  tasks_rescheduled: number
  pending_manual: number
  isolated: number
  degraded: number
  self_heal_rate: number
  mttr_seconds: number
}

export interface OpsOverview {
  telemetry_source: string
  self_heal: SelfHealStats
  conditions: DeviceCondition[]
  llm_enabled: boolean
  llm_model?: string
  scheduler?: Record<string, number>
  cluster?: Record<string, number>
}

export interface BenchmarkResult {
  id: string
  node_id: string
  target: string
  resource_id?: string
  model?: string
  test: string
  metric: string
  unit: string
  value: number
  expected: number
  ratio: number
  status: 'pass' | 'warn' | 'fail'
  attribution?: string
}

export interface BenchmarkRun {
  id: string
  node_id: string
  kind: string
  tests: string[]
  status: string
  verdict: 'pass' | 'warn' | 'fail'
  started_at: string
  finished_at: string
  results: BenchmarkResult[]
  summary: string[]
}

export interface DiagnosisFinding {
  severity: string
  target: string
  symptom: string
  root_cause: string
  suggestion: string
}

export interface DiagnosisReport {
  generated_at: string
  scope: string
  findings: DiagnosisFinding[]
  summary: string
  llm_used: boolean
  llm_model?: string
  llm_analysis?: string
  llm_error?: string
}

export interface ProbeCheck {
  name: string
  status: 'pass' | 'fail' | 'skip'
  detail?: string
}

export interface ProbeResult {
  resource_type: 'gpu' | 'npu'
  status: 'pass' | 'fail' | 'skip'
  ready: boolean
  reserved_id?: string
  allocated_id?: string
  node_id?: string
  model?: string
  checks: ProbeCheck[]
  error?: string
  latency_ms: number
}

export interface ProbeReport {
  id: string
  started_at: string
  finished_at: string
  status: 'pass' | 'fail'
  results: ProbeResult[]
  summary: string
}

export interface Readiness {
  resource_type: 'gpu' | 'npu'
  ready: boolean
  total: number
  available: number
  debug_reserved: number
  debug_reserved_ready: number
  message?: string
}

export const opsApi = {
  overview: (): Promise<OpsOverview> => client.get('/ops/overview'),
  listTelemetry: (nodeId?: string): Promise<DeviceTelemetry[]> =>
    client.get('/ops/telemetry', { params: nodeId ? { node_id: nodeId } : undefined }),
  listEvents: (params?: Record<string, string>): Promise<OpsEvent[]> =>
    client.get('/ops/events', { params }),
  listRules: (): Promise<OpsRule[]> => client.get('/ops/rules'),
  updateRule: (id: string, body: { enabled?: boolean; auto_remediate?: boolean; threshold?: number }): Promise<OpsRule> =>
    client.put(`/ops/rules/${id}`, body),
  listConditions: (): Promise<DeviceCondition[]> => client.get('/ops/conditions'),
  resetDevice: (id: string, operator?: string): Promise<void> =>
    client.post(`/ops/devices/${id}/reset`, { operator }),
  injectFault: (id: string, fault: Record<string, number | string>): Promise<void> =>
    client.post(`/ops/devices/${id}/fault`, fault),
  clearFault: (id: string): Promise<void> => client.delete(`/ops/devices/${id}/fault`),
  listRuns: (nodeId?: string): Promise<BenchmarkRun[]> =>
    client.get('/ops/benchmark/runs', { params: nodeId ? { node_id: nodeId } : undefined }),
  runBenchmark: (nodeId: string, kind = 'benchmark'): Promise<BenchmarkRun> =>
    client.post('/ops/benchmark/runs', { node_id: nodeId, kind }),
  diagnose: (body: { node_id?: string; resource_id?: string; question?: string }): Promise<DiagnosisReport> =>
    client.post('/ops/diagnose', body),
  readiness: (): Promise<Readiness[]> => client.get('/ops/readiness'),
  lastProbe: (): Promise<ProbeReport | { message: string }> => client.get('/ops/probe'),
  runProbe: (types?: Array<'gpu' | 'npu'>): Promise<ProbeReport> =>
    client.post('/ops/probe', types ? { types } : {}),
}
