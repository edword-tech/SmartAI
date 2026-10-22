/*
 * Unified Resource Manager - Auto Remediation
 *
 * 故障自愈引擎：基于设备遥测的规则检测 → 自动处置（隔离 / 降级 / 任务重调度）→ 恢复后自动解除。
 * - 致命 XID（79 掉卡、48/94/95 ECC、74 NVLink 等）与 ECC DBE：设备置 failed + cordon + 重调度，需人工 reset
 * - 过温、NVLink CRC、PCIe replay、SBE 风暴：设备降级（调度打分降低），信号消失后自动恢复
 * - 节点心跳丢失：节点上任务整体重调度
 * 每条规则可单独开关、调阈值，或切换为仅告警（AutoRemediate=false）
 */

package remediation

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/ops/telemetry"
)

// Level 事件级别
type Level string

const (
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
)

// 规则 ID
const (
	RuleXIDFatal        = "xid-fatal"
	RuleXIDApplication  = "xid-application"
	RuleECCDBE          = "ecc-dbe"
	RuleECCSBEStorm     = "ecc-sbe-storm"
	RuleThermalWarning  = "thermal-warning"
	RuleThermalCritical = "thermal-critical"
	RuleNVLinkCRC       = "nvlink-crc"
	RulePCIeReplay      = "pcie-replay"
	RuleNodeHeartbeat   = "node-heartbeat"
	RuleAutoRecover     = "auto-recover"
)

// FatalXIDs 需要隔离并 reset 的 XID
var FatalXIDs = map[int]string{
	48:  "显存 DBE ECC 错误",
	63:  "显存行重映射挂起",
	64:  "显存行重映射失败",
	74:  "NVLink 错误",
	79:  "GPU 掉卡 (fallen off the bus)",
	92:  "SBE ECC 错误率过高",
	94:  "Contained ECC 错误",
	95:  "Uncontained ECC 错误",
	119: "GSP RPC 超时",
	120: "GSP 错误",
}

// ApplicationXIDs 通常由业务程序引起的 XID（不隔离设备，归因到任务）
var ApplicationXIDs = map[int]string{
	13: "图形引擎异常（多为越界访问）",
	31: "GPU 显存页错误（多为程序非法访问）",
	43: "GPU 停止处理（用户程序异常退出）",
	45: "被抢占/上下文清理",
}

// Event 运维事件
type Event struct {
	ID            string    `json:"id"`
	Time          time.Time `json:"time"`
	Level         Level     `json:"level"`
	Category      string    `json:"category"` // device / node / benchmark / manual / scheduler
	RuleID        string    `json:"rule_id,omitempty"`
	NodeID        string    `json:"node_id,omitempty"`
	ResourceID    string    `json:"resource_id,omitempty"`
	Title         string    `json:"title"`
	Message       string    `json:"message"`
	Actions       []string  `json:"actions,omitempty"`
	AffectedTasks []string  `json:"affected_tasks,omitempty"`
}

// Rule 自愈规则
type Rule struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Severity      Level   `json:"severity"`
	Enabled       bool    `json:"enabled"`
	AutoRemediate bool    `json:"auto_remediate"`
	Threshold     float64 `json:"threshold,omitempty"`
	Unit          string  `json:"unit,omitempty"`
	Action        string  `json:"action"`
}

// DefaultRules 默认规则集
func DefaultRules() []Rule {
	return []Rule{
		{ID: RuleXIDFatal, Name: "致命 XID", Description: "XID 48/63/64/74/79/92/94/95/119/120", Severity: LevelCritical, Enabled: true, AutoRemediate: true, Action: "设备置 failed + 隔离 + 任务重调度，等待人工 reset"},
		{ID: RuleXIDApplication, Name: "应用类 XID", Description: "XID 13/31/43/45，多由业务代码引起", Severity: LevelWarning, Enabled: true, AutoRemediate: false, Action: "记录事件并归因到当前任务，不隔离设备"},
		{ID: RuleECCDBE, Name: "显存 ECC 双比特错误", Description: "DBE 计数增长", Severity: LevelCritical, Enabled: true, AutoRemediate: true, Action: "设备置 failed + 隔离 + 任务重调度，等待人工 reset/返修"},
		{ID: RuleECCSBEStorm, Name: "ECC 单比特错误风暴", Description: "单次采样周期 SBE 增量超过阈值", Severity: LevelWarning, Enabled: true, AutoRemediate: true, Threshold: 100, Unit: "次/周期", Action: "设备降级（新任务避让），预测性维护"},
		{ID: RuleThermalWarning, Name: "温度告警", Description: "GPU 温度超过阈值", Severity: LevelWarning, Enabled: true, AutoRemediate: true, Threshold: 85, Unit: "°C", Action: "设备降级，温度恢复后自动解除"},
		{ID: RuleThermalCritical, Name: "温度严重", Description: "GPU 温度超过严重阈值", Severity: LevelCritical, Enabled: true, AutoRemediate: true, Threshold: 95, Unit: "°C", Action: "隔离设备并迁移任务，温度恢复后自动解除"},
		{ID: RuleNVLinkCRC, Name: "NVLink CRC 错误", Description: "单次采样周期 NVLink CRC 增量超过阈值", Severity: LevelWarning, Enabled: true, AutoRemediate: true, Threshold: 50, Unit: "次/周期", Action: "设备降级（多卡任务避让）"},
		{ID: RulePCIeReplay, Name: "PCIe Replay", Description: "单次采样周期 PCIe replay 增量超过阈值", Severity: LevelWarning, Enabled: true, AutoRemediate: true, Threshold: 100, Unit: "次/周期", Action: "设备降级，提示检查 PCIe 链路/Riser"},
		{ID: RuleNodeHeartbeat, Name: "节点心跳丢失", Description: "节点超过 3 个心跳周期未上报", Severity: LevelCritical, Enabled: true, AutoRemediate: true, Action: "节点上任务整体重调度（checkpoint 续训）"},
		{ID: RuleAutoRecover, Name: "自动恢复", Description: "非致命故障信号连续 N 个周期消失", Severity: LevelInfo, Enabled: true, AutoRemediate: true, Threshold: 3, Unit: "周期", Action: "恢复 healthy 并解除隔离"},
	}
}

// Actuator 自愈动作执行器（由调度器 + 设备账本实现）
type Actuator interface {
	CordonDevice(resourceID string, cordoned bool) error
	SetDeviceHealth(resourceID, health string) error
	RescheduleDevice(resourceID, reason string) []string
	RescheduleNode(nodeID, reason string) []string
}

// Stats 自愈统计
type Stats struct {
	FaultsDetected   int            `json:"faults_detected"`
	AutoRemediated   int            `json:"auto_remediated"`
	AutoRecovered    int            `json:"auto_recovered"`
	ManualResets     int            `json:"manual_resets"`
	TasksRescheduled int            `json:"tasks_rescheduled"`
	PendingManual    int            `json:"pending_manual"`
	Isolated         int            `json:"isolated"`
	Degraded         int            `json:"degraded"`
	SelfHealRate     float64        `json:"self_heal_rate"` // 无需人工介入即处置完成的故障占比
	MTTRSeconds      float64        `json:"mttr_seconds"`   // 设备故障从检测到恢复的平均时长
	EventsByLevel    map[Level]int  `json:"events_by_level"`
	EventsByRule     map[string]int `json:"events_by_rule"`
}

// DeviceCondition 设备当前的自愈状态
type DeviceCondition struct {
	ResourceID  string    `json:"resource_id"`
	NodeID      string    `json:"node_id"`
	RuleID      string    `json:"rule_id"`
	Reason      string    `json:"reason"`
	Isolated    bool      `json:"isolated"`
	Degraded    bool      `json:"degraded"`
	NeedsReset  bool      `json:"needs_reset"`
	Since       time.Time `json:"since"`
	CleanStreak int       `json:"clean_streak"`
}

type deviceState struct {
	initialized bool
	lastSBE     int64
	lastDBE     int64
	lastNVLink  int64
	lastReplay  int64
	lastXID     int
	cond        *DeviceCondition
}

// Engine 自愈引擎
type Engine struct {
	actuator Actuator
	now      func() time.Time

	evalMu  sync.Mutex // 串行化评估与处置
	devices map[string]*deviceState

	mu          sync.RWMutex // 保护规则、事件、统计
	rules       map[string]*Rule
	order       []string
	events      []Event
	maxEvents   int
	stats       Stats
	mttrSum     float64
	mttrCount   int
	subscribers []func(Event)
}

// NewEngine 创建自愈引擎
func NewEngine(actuator Actuator) *Engine {
	e := &Engine{
		actuator:  actuator,
		now:       time.Now,
		devices:   make(map[string]*deviceState),
		rules:     make(map[string]*Rule),
		maxEvents: 1000,
		stats:     Stats{EventsByLevel: map[Level]int{}, EventsByRule: map[string]int{}},
	}
	for _, r := range DefaultRules() {
		r := r
		e.rules[r.ID] = &r
		e.order = append(e.order, r.ID)
	}
	return e
}

// Subscribe 订阅事件
func (e *Engine) Subscribe(fn func(Event)) {
	e.mu.Lock()
	e.subscribers = append(e.subscribers, fn)
	e.mu.Unlock()
}

// Rules 返回规则列表
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Rule, 0, len(e.order))
	for _, id := range e.order {
		out = append(out, *e.rules[id])
	}
	return out
}

// RuleUpdate 规则更新（nil 字段不修改）
type RuleUpdate struct {
	Enabled       *bool    `json:"enabled,omitempty"`
	AutoRemediate *bool    `json:"auto_remediate,omitempty"`
	Threshold     *float64 `json:"threshold,omitempty"`
}

// UpdateRule 更新规则
func (e *Engine) UpdateRule(id string, u RuleUpdate) (Rule, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.rules[id]
	if !ok {
		return Rule{}, fmt.Errorf("rule %s not found", id)
	}
	if u.Enabled != nil {
		r.Enabled = *u.Enabled
	}
	if u.AutoRemediate != nil {
		r.AutoRemediate = *u.AutoRemediate
	}
	if u.Threshold != nil {
		if *u.Threshold < 0 {
			return Rule{}, fmt.Errorf("threshold must be >= 0")
		}
		r.Threshold = *u.Threshold
	}
	return *r, nil
}

func (e *Engine) rule(id string) Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return *e.rules[id]
}

// Record 记录事件
func (e *Engine) Record(ev Event) Event {
	if ev.ID == "" {
		ev.ID = uuid.New().String()
	}
	if ev.Time.IsZero() {
		ev.Time = e.now()
	}
	e.mu.Lock()
	e.events = append(e.events, ev)
	if len(e.events) > e.maxEvents {
		e.events = e.events[len(e.events)-e.maxEvents:]
	}
	e.stats.EventsByLevel[ev.Level]++
	if ev.RuleID != "" {
		e.stats.EventsByRule[ev.RuleID]++
	}
	subs := make([]func(Event), len(e.subscribers))
	copy(subs, e.subscribers)
	e.mu.Unlock()
	for _, fn := range subs {
		fn(ev)
	}
	return ev
}

// EventFilter 事件查询条件
type EventFilter struct {
	Level      Level
	Category   string
	NodeID     string
	ResourceID string
	Since      time.Time
	Limit      int
}

// Events 查询事件（新的在前）
func (e *Engine) Events(f EventFilter) []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := []Event{}
	for i := len(e.events) - 1; i >= 0; i-- {
		ev := e.events[i]
		if f.Level != "" && ev.Level != f.Level {
			continue
		}
		if f.Category != "" && ev.Category != f.Category {
			continue
		}
		if f.NodeID != "" && ev.NodeID != f.NodeID {
			continue
		}
		if f.ResourceID != "" && ev.ResourceID != f.ResourceID {
			continue
		}
		if !f.Since.IsZero() && ev.Time.Before(f.Since) {
			continue
		}
		out = append(out, ev)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out
}

// Stats 自愈统计
func (e *Engine) Stats() Stats {
	e.evalMu.Lock()
	var isolated, degraded, pending int
	for _, st := range e.devices {
		if st.cond == nil {
			continue
		}
		if st.cond.Isolated {
			isolated++
		}
		if st.cond.Degraded {
			degraded++
		}
		if st.cond.NeedsReset {
			pending++
		}
	}
	e.evalMu.Unlock()

	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.stats
	s.EventsByLevel = make(map[Level]int, len(e.stats.EventsByLevel))
	for k, v := range e.stats.EventsByLevel {
		s.EventsByLevel[k] = v
	}
	s.EventsByRule = make(map[string]int, len(e.stats.EventsByRule))
	for k, v := range e.stats.EventsByRule {
		s.EventsByRule[k] = v
	}
	s.Isolated, s.Degraded, s.PendingManual = isolated, degraded, pending
	if s.FaultsDetected > 0 {
		s.SelfHealRate = float64(s.FaultsDetected-s.ManualResets-pending) / float64(s.FaultsDetected)
		if s.SelfHealRate < 0 {
			s.SelfHealRate = 0
		}
	}
	if e.mttrCount > 0 {
		s.MTTRSeconds = e.mttrSum / float64(e.mttrCount)
	}
	return s
}

// Conditions 当前处于故障处置中的设备
func (e *Engine) Conditions() []DeviceCondition {
	e.evalMu.Lock()
	defer e.evalMu.Unlock()
	out := []DeviceCondition{}
	for _, st := range e.devices {
		if st.cond != nil {
			out = append(out, *st.cond)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ResourceID < out[j].ResourceID })
	return out
}

func (e *Engine) bump(fn func(s *Stats)) {
	e.mu.Lock()
	fn(&e.stats)
	e.mu.Unlock()
}

type finding struct {
	rule    string
	reason  string
	fatal   bool // 需要隔离 + reset
	isolate bool // 隔离并迁移任务（非 fatal 可自动恢复）
}

func (e *Engine) detect(st *deviceState, s *telemetry.DeviceTelemetry) []finding {
	var out []finding
	enabled := func(id string) (Rule, bool) {
		r := e.rule(id)
		return r, r.Enabled
	}

	if s.XID != 0 {
		if desc, ok := FatalXIDs[s.XID]; ok {
			if _, on := enabled(RuleXIDFatal); on {
				out = append(out, finding{rule: RuleXIDFatal, reason: fmt.Sprintf("XID %d: %s", s.XID, desc), fatal: true})
			}
		}
	}
	if s.ECCDoubleBit > st.lastDBE {
		if _, on := enabled(RuleECCDBE); on {
			out = append(out, finding{rule: RuleECCDBE, reason: fmt.Sprintf("ECC DBE 计数 %d → %d", st.lastDBE, s.ECCDoubleBit), fatal: true})
		}
	}
	if r, on := enabled(RuleThermalCritical); on && s.TemperatureC >= r.Threshold {
		out = append(out, finding{rule: RuleThermalCritical, reason: fmt.Sprintf("温度 %.1f°C ≥ %.0f°C", s.TemperatureC, r.Threshold), isolate: true})
	} else if r, on := enabled(RuleThermalWarning); on && s.TemperatureC >= r.Threshold {
		out = append(out, finding{rule: RuleThermalWarning, reason: fmt.Sprintf("温度 %.1f°C ≥ %.0f°C", s.TemperatureC, r.Threshold)})
	}
	if r, on := enabled(RuleECCSBEStorm); on && st.initialized && float64(s.ECCSingleBit-st.lastSBE) >= r.Threshold {
		out = append(out, finding{rule: RuleECCSBEStorm, reason: fmt.Sprintf("SBE 增量 %d", s.ECCSingleBit-st.lastSBE)})
	}
	if r, on := enabled(RuleNVLinkCRC); on && st.initialized && float64(s.NVLinkCRCErrors-st.lastNVLink) >= r.Threshold {
		out = append(out, finding{rule: RuleNVLinkCRC, reason: fmt.Sprintf("NVLink CRC 增量 %d", s.NVLinkCRCErrors-st.lastNVLink)})
	}
	if r, on := enabled(RulePCIeReplay); on && st.initialized && float64(s.PCIeReplay-st.lastReplay) >= r.Threshold {
		out = append(out, finding{rule: RulePCIeReplay, reason: fmt.Sprintf("PCIe replay 增量 %d", s.PCIeReplay-st.lastReplay)})
	}
	return out
}

// Evaluate 评估一批遥测样本并执行处置
func (e *Engine) Evaluate(samples map[string]*telemetry.DeviceTelemetry) {
	ids := make([]string, 0, len(samples))
	for id := range samples {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	e.evalMu.Lock()
	defer e.evalMu.Unlock()
	for _, id := range ids {
		e.evaluateDevice(samples[id])
	}
}

func (e *Engine) evaluateDevice(s *telemetry.DeviceTelemetry) {
	st, ok := e.devices[s.ResourceID]
	if !ok {
		st = &deviceState{}
		e.devices[s.ResourceID] = st
	}
	findings := e.detect(st, s)

	// 应用类 XID：只记录并归因，不影响设备状态
	if s.XID != 0 && s.XID != st.lastXID {
		if desc, ok := ApplicationXIDs[s.XID]; ok && e.rule(RuleXIDApplication).Enabled {
			e.Record(Event{Level: LevelWarning, Category: "device", RuleID: RuleXIDApplication, NodeID: s.NodeID, ResourceID: s.ResourceID,
				Title: fmt.Sprintf("应用类 XID %d", s.XID), Message: desc + "，归因到设备上运行的任务，设备不隔离"})
		}
	}

	st.initialized = true
	st.lastSBE, st.lastDBE, st.lastNVLink, st.lastReplay, st.lastXID = s.ECCSingleBit, s.ECCDoubleBit, s.NVLinkCRCErrors, s.PCIeReplay, s.XID

	if len(findings) == 0 {
		e.maybeRecover(st, s)
		return
	}

	// 取最严重的一条处置：fatal > isolate > degrade
	sort.SliceStable(findings, func(i, j int) bool { return severityRank(findings[i]) > severityRank(findings[j]) })
	f := findings[0]
	cond := st.cond
	if cond != nil {
		cond.CleanStreak = 0
		if severityRank(f) <= condRank(cond) {
			return // 已按同级或更高级别处置
		}
	}

	rule := e.rule(f.rule)
	level := rule.Severity
	ev := Event{Level: level, Category: "device", RuleID: f.rule, NodeID: s.NodeID, ResourceID: s.ResourceID,
		Title: rule.Name, Message: f.reason}
	if cond == nil {
		e.bump(func(st *Stats) { st.FaultsDetected++ })
		cond = &DeviceCondition{ResourceID: s.ResourceID, NodeID: s.NodeID, Since: e.now()}
	}
	cond.RuleID, cond.Reason = f.rule, f.reason

	if !rule.AutoRemediate {
		ev.Actions = []string{"仅告警（规则未开启自动处置）"}
		st.cond = cond
		e.Record(ev)
		return
	}

	switch {
	case f.fatal || f.isolate:
		health := pool.DeviceHealthFailed
		if !f.fatal {
			health = pool.DeviceHealthDegraded
		}
		if err := e.actuator.SetDeviceHealth(s.ResourceID, health); err == nil {
			ev.Actions = append(ev.Actions, "设备健康状态 → "+health)
		}
		if !cond.Isolated {
			if err := e.actuator.CordonDevice(s.ResourceID, true); err == nil {
				cond.Isolated = true
				ev.Actions = append(ev.Actions, "隔离设备 (cordon)")
			}
		}
		moved := e.actuator.RescheduleDevice(s.ResourceID, f.reason)
		if len(moved) > 0 {
			ev.AffectedTasks = moved
			ev.Actions = append(ev.Actions, fmt.Sprintf("重调度 %d 个任务", len(moved)))
		}
		cond.NeedsReset = cond.NeedsReset || f.fatal
		cond.Degraded = !f.fatal
		if cond.NeedsReset {
			ev.Actions = append(ev.Actions, "等待人工 reset / 返修")
		}
		e.bump(func(st *Stats) { st.AutoRemediated++; st.TasksRescheduled += len(moved) })
	default:
		if err := e.actuator.SetDeviceHealth(s.ResourceID, pool.DeviceHealthDegraded); err == nil {
			ev.Actions = append(ev.Actions, "设备降级 (degraded)，新任务避让")
		}
		cond.Degraded = true
		e.bump(func(st *Stats) { st.AutoRemediated++ })
	}
	st.cond = cond
	e.Record(ev)
}

func severityRank(f finding) int {
	switch {
	case f.fatal:
		return 3
	case f.isolate:
		return 2
	}
	return 1
}

func condRank(c *DeviceCondition) int {
	switch {
	case c.NeedsReset:
		return 3
	case c.Isolated:
		return 2
	case c.Degraded:
		return 1
	}
	return 0 // 仅告警
}

func (e *Engine) maybeRecover(st *deviceState, s *telemetry.DeviceTelemetry) {
	cond := st.cond
	if cond == nil || cond.NeedsReset {
		return
	}
	r := e.rule(RuleAutoRecover)
	if !r.Enabled {
		return
	}
	cond.CleanStreak++
	if float64(cond.CleanStreak) < r.Threshold {
		return
	}
	e.restore(st, s.NodeID, s.ResourceID, RuleAutoRecover, "故障信号连续 "+fmt.Sprint(cond.CleanStreak)+" 个周期消失，自动恢复")
	e.bump(func(st *Stats) { st.AutoRecovered++ })
}

func (e *Engine) restore(st *deviceState, nodeID, resourceID, ruleID, msg string) {
	cond := st.cond
	ev := Event{Level: LevelInfo, Category: "device", RuleID: ruleID, NodeID: nodeID, ResourceID: resourceID, Title: "设备恢复", Message: msg}
	if cond.Degraded || cond.Isolated || cond.NeedsReset {
		if err := e.actuator.SetDeviceHealth(resourceID, pool.DeviceHealthHealthy); err == nil {
			ev.Actions = append(ev.Actions, "设备健康状态 → healthy")
		}
	}
	if cond.Isolated {
		if err := e.actuator.CordonDevice(resourceID, false); err == nil {
			ev.Actions = append(ev.Actions, "解除隔离 (uncordon)")
		}
	}
	dur := e.now().Sub(cond.Since).Seconds()
	e.mu.Lock()
	e.mttrSum += dur
	e.mttrCount++
	e.mu.Unlock()
	st.cond = nil
	e.Record(ev)
}

// ResetDevice 人工 reset 后确认恢复（清除致命状态）
func (e *Engine) ResetDevice(resourceID, operator string) error {
	e.evalMu.Lock()
	defer e.evalMu.Unlock()
	st, ok := e.devices[resourceID]
	if !ok || st.cond == nil {
		return fmt.Errorf("device %s has no active fault", resourceID)
	}
	wasManual := st.cond.NeedsReset
	st.lastXID = 0
	if operator == "" {
		operator = "operator"
	}
	e.restore(st, st.cond.NodeID, resourceID, "", fmt.Sprintf("%s 完成设备 reset/检修，恢复上线", operator))
	if wasManual {
		e.bump(func(s *Stats) { s.ManualResets++ })
	}
	return nil
}

// HandleNodeFailure 节点心跳丢失
func (e *Engine) HandleNodeFailure(nodeID, reason string) {
	r := e.rule(RuleNodeHeartbeat)
	if !r.Enabled {
		return
	}
	e.bump(func(s *Stats) { s.FaultsDetected++ })
	ev := Event{Level: LevelCritical, Category: "node", RuleID: RuleNodeHeartbeat, NodeID: nodeID, Title: r.Name, Message: reason}
	if r.AutoRemediate {
		moved := e.actuator.RescheduleNode(nodeID, reason)
		ev.AffectedTasks = moved
		ev.Actions = []string{"节点标记不可调度", fmt.Sprintf("重调度 %d 个任务", len(moved))}
		e.bump(func(s *Stats) { s.AutoRemediated++; s.TasksRescheduled += len(moved) })
	} else {
		ev.Actions = []string{"仅告警（规则未开启自动处置）"}
	}
	e.Record(ev)
}

// HandleNodeRecovered 节点恢复
func (e *Engine) HandleNodeRecovered(nodeID string) {
	e.bump(func(s *Stats) { s.AutoRecovered++ })
	e.Record(Event{Level: LevelInfo, Category: "node", RuleID: RuleNodeHeartbeat, NodeID: nodeID, Title: "节点恢复", Message: "节点心跳恢复，重新参与调度"})
}
