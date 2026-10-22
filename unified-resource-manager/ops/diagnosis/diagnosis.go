/*
 * Unified Resource Manager - Diagnosis
 *
 * 故障诊断助手：汇总节点/设备的遥测、自愈事件、压测归因，生成规则诊断结论；
 * 配置 OpenAI 兼容接口（URM_LLM_BASE_URL / URM_LLM_API_KEY / URM_LLM_MODEL，可指向私有化部署的
 * vLLM / Qwen / DeepSeek）后，将上下文交给大模型生成根因分析与处置建议。未配置或调用失败时回退到规则诊断。
 */

package diagnosis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"flexai/unified-resource-manager/ops/benchmark"
	"flexai/unified-resource-manager/ops/remediation"
	"flexai/unified-resource-manager/ops/telemetry"
)

// LLMConfig 大模型配置
type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// LLMConfigFromEnv 从环境变量读取配置
func LLMConfigFromEnv() LLMConfig {
	return LLMConfig{
		BaseURL: strings.TrimRight(os.Getenv("URM_LLM_BASE_URL"), "/"),
		APIKey:  os.Getenv("URM_LLM_API_KEY"),
		Model:   os.Getenv("URM_LLM_MODEL"),
		Timeout: 60 * time.Second,
	}
}

// Enabled 是否配置了大模型
func (c LLMConfig) Enabled() bool { return c.BaseURL != "" && c.Model != "" }

// Context 诊断上下文
type Context struct {
	NodeID     string                        `json:"node_id,omitempty"`
	ResourceID string                        `json:"resource_id,omitempty"`
	Question   string                        `json:"question,omitempty"`
	Telemetry  []*telemetry.DeviceTelemetry  `json:"telemetry"`
	Events     []remediation.Event           `json:"events"`
	Conditions []remediation.DeviceCondition `json:"conditions"`
	Anomalies  []benchmark.Result            `json:"benchmark_anomalies"`
}

// Finding 规则诊断结论
type Finding struct {
	Severity   remediation.Level `json:"severity"`
	Target     string            `json:"target"`
	Symptom    string            `json:"symptom"`
	RootCause  string            `json:"root_cause"`
	Suggestion string            `json:"suggestion"`
}

// Report 诊断报告
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Scope       string    `json:"scope"`
	Findings    []Finding `json:"findings"`
	Summary     string    `json:"summary"`
	LLMUsed     bool      `json:"llm_used"`
	LLMModel    string    `json:"llm_model,omitempty"`
	LLMAnalysis string    `json:"llm_analysis,omitempty"`
	LLMError    string    `json:"llm_error,omitempty"`
}

// Diagnoser 诊断器
type Diagnoser struct {
	cfg    LLMConfig
	client *http.Client
}

// New 创建诊断器
func New(cfg LLMConfig) *Diagnoser {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Diagnoser{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

// LLMEnabled 是否启用了大模型
func (d *Diagnoser) LLMEnabled() bool { return d.cfg.Enabled() }

// LLMModel 模型名
func (d *Diagnoser) LLMModel() string { return d.cfg.Model }

// RuleFindings 基于规则的诊断
func RuleFindings(c Context) []Finding {
	var out []Finding
	for _, t := range c.Telemetry {
		target := t.ResourceID
		if desc, ok := remediation.FatalXIDs[t.XID]; ok {
			out = append(out, Finding{Severity: remediation.LevelCritical, Target: target, Symptom: fmt.Sprintf("XID %d", t.XID),
				RootCause: desc, Suggestion: "排空任务后 nvidia-smi -r 重置；复现则收集 nvidia-bug-report.sh 并返修"})
		} else if desc, ok := remediation.ApplicationXIDs[t.XID]; ok {
			out = append(out, Finding{Severity: remediation.LevelWarning, Target: target, Symptom: fmt.Sprintf("XID %d", t.XID),
				RootCause: desc, Suggestion: "检查该卡上运行任务的日志（CUDA 越界 / OOM / 进程被 kill），设备本身无需隔离"})
		}
		if t.ECCDoubleBit > 0 {
			out = append(out, Finding{Severity: remediation.LevelCritical, Target: target, Symptom: fmt.Sprintf("ECC DBE=%d", t.ECCDoubleBit),
				RootCause: "显存不可纠正错误", Suggestion: "reset 触发行重映射；nvidia-smi -q -d ROW_REMAPPER 查看剩余备用行，耗尽则返修"})
		}
		if t.TemperatureC >= 85 {
			out = append(out, Finding{Severity: remediation.LevelWarning, Target: target, Symptom: fmt.Sprintf("温度 %.0f°C", t.TemperatureC),
				RootCause: "散热不足（风扇/风道/液冷/机房进风温度）", Suggestion: "对比同机其它卡温度判断单卡或整机问题；检查 BMC 风扇策略与冷通道温度"})
		}
		if t.NVLinkCRCErrors > 0 {
			out = append(out, Finding{Severity: remediation.LevelWarning, Target: target, Symptom: fmt.Sprintf("NVLink CRC=%d", t.NVLinkCRCErrors),
				RootCause: "NVLink 链路信号质量问题", Suggestion: "nvidia-smi nvlink -e 定位链路，检查 NVSwitch/桥接；多卡训练带宽会明显下降"})
		}
		if t.PCIeReplay > 0 {
			out = append(out, Finding{Severity: remediation.LevelWarning, Target: target, Symptom: fmt.Sprintf("PCIe replay=%d", t.PCIeReplay),
				RootCause: "PCIe 链路重传", Suggestion: "lspci -vv 确认 LnkSta 代际/宽度；检查 Riser、插槽与 BIOS PCIe 设置"})
		}
	}
	for _, r := range c.Anomalies {
		out = append(out, Finding{Severity: remediation.LevelWarning, Target: fmt.Sprintf("%s/%s", r.NodeID, r.Target),
			Symptom:   fmt.Sprintf("%s %.2f%s（基线 %.2f，%.0f%%）", r.Test, r.Value, r.Unit, r.Expected, r.Ratio*100),
			RootCause: r.Attribution, Suggestion: "处理后重新执行该压测项确认恢复"})
	}
	for _, cond := range c.Conditions {
		if cond.NeedsReset {
			out = append(out, Finding{Severity: remediation.LevelCritical, Target: cond.ResourceID, Symptom: "设备处于隔离状态",
				RootCause: cond.Reason, Suggestion: "人工 reset/检修后调用 POST /api/v1/ops/devices/{id}/reset 恢复上线"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i].Severity) > rank(out[j].Severity) })
	return out
}

func rank(l remediation.Level) int {
	switch l {
	case remediation.LevelCritical:
		return 2
	case remediation.LevelWarning:
		return 1
	}
	return 0
}

// Diagnose 生成诊断报告
func (d *Diagnoser) Diagnose(ctx context.Context, c Context) *Report {
	scope := "cluster"
	switch {
	case c.ResourceID != "":
		scope = "device " + c.ResourceID
	case c.NodeID != "":
		scope = "node " + c.NodeID
	}
	rep := &Report{GeneratedAt: time.Now(), Scope: scope, Findings: RuleFindings(c)}
	if len(rep.Findings) == 0 {
		rep.Summary = "未发现异常：遥测指标、自愈事件与压测结果均正常"
	} else {
		crit := 0
		for _, f := range rep.Findings {
			if f.Severity == remediation.LevelCritical {
				crit++
			}
		}
		rep.Summary = fmt.Sprintf("发现 %d 项异常（严重 %d 项），首要问题：%s — %s", len(rep.Findings), crit, rep.Findings[0].Target, rep.Findings[0].RootCause)
	}
	if !d.cfg.Enabled() {
		return rep
	}
	analysis, err := d.askLLM(ctx, c, rep)
	if err != nil {
		rep.LLMError = err.Error()
		return rep
	}
	rep.LLMUsed, rep.LLMModel, rep.LLMAnalysis = true, d.cfg.Model, analysis
	return rep
}

const systemPrompt = `你是 GPU/NPU 智算集群的资深 SRE。根据给出的遥测、自愈事件、压测异常与规则诊断，
输出：1) 根因判断（按可能性排序，说明依据）；2) 立即处置步骤（给出具体命令）；3) 预防/长期改进建议。
只基于给定数据推断，数据不足时明确指出还需采集什么。使用中文，简洁。`

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (d *Diagnoser) askLLM(ctx context.Context, c Context, rep *Report) (string, error) {
	// 控制上下文体积
	if len(c.Events) > 30 {
		c.Events = c.Events[:30]
	}
	if len(c.Telemetry) > 32 {
		c.Telemetry = c.Telemetry[:32]
	}
	payload, _ := json.Marshal(map[string]interface{}{"context": c, "rule_findings": rep.Findings})
	user := string(payload)
	if c.Question != "" {
		user = "问题：" + c.Question + "\n数据：" + user
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":       d.cfg.Model,
		"temperature": 0.2,
		"messages":    []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: user}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.APIKey)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
