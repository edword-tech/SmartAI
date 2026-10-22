/*
 * Unified Resource Manager - Device Telemetry
 *
 * 设备遥测：统一的 GPU/NPU 健康与性能指标采集。
 * - PrometheusSource：从 Prometheus 查询 DCGM exporter 指标（生产环境）
 * - SimulatedSource：按账本占用推算指标，并支持故障注入（演示 / 演练故障自愈）
 * - Hub：周期采集、保留短期历史，并把最新样本推送给订阅者（自愈引擎、指标导出）
 */

package telemetry

import (
	"context"
	"sort"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// DeviceTelemetry 单设备遥测样本
type DeviceTelemetry struct {
	ResourceID      string            `json:"resource_id"`
	NodeID          string            `json:"node_id"`
	DeviceID        string            `json:"device_id"`
	Model           string            `json:"model"`
	Type            pool.ResourceType `json:"type"`
	Timestamp       time.Time         `json:"timestamp"`
	UtilizationPct  float64           `json:"utilization_pct"`
	MemoryUsedGB    float64           `json:"memory_used_gb"`
	MemoryTotalGB   float64           `json:"memory_total_gb"`
	TemperatureC    float64           `json:"temperature_c"`
	PowerW          float64           `json:"power_w"`
	PowerLimitW     float64           `json:"power_limit_w"`
	SMClockMHz      float64           `json:"sm_clock_mhz"`
	ECCSingleBit    int64             `json:"ecc_sbe_total"`
	ECCDoubleBit    int64             `json:"ecc_dbe_total"`
	XID             int               `json:"xid,omitempty"` // 最近一次 XID 错误码，0 表示无
	PCIeReplay      int64             `json:"pcie_replay_total"`
	NVLinkCRCErrors int64             `json:"nvlink_crc_errors_total"`
	Throttle        []string          `json:"throttle,omitempty"`
	Source          string            `json:"source"`
}

// Source 遥测数据源
type Source interface {
	Name() string
	Collect(ctx context.Context, devices []*pool.Resource) (map[string]*DeviceTelemetry, error)
}

// Subscriber 遥测样本订阅者
type Subscriber func(samples map[string]*DeviceTelemetry)

// Hub 遥测中心
type Hub struct {
	resources   pool.ResourcePool
	source      Source
	fallback    Source
	interval    time.Duration
	historySize int

	latest      map[string]*DeviceTelemetry
	history     map[string][]*DeviceTelemetry
	subscribers []Subscriber
	lastErr     error
	mu          sync.RWMutex
}

// NewHub 创建遥测中心；fallback 用于补齐主数据源缺失的设备
func NewHub(resources pool.ResourcePool, source, fallback Source, interval time.Duration) *Hub {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Hub{
		resources:   resources,
		source:      source,
		fallback:    fallback,
		interval:    interval,
		historySize: 60,
		latest:      make(map[string]*DeviceTelemetry),
		history:     make(map[string][]*DeviceTelemetry),
	}
}

// Subscribe 订阅样本
func (h *Hub) Subscribe(s Subscriber) {
	h.mu.Lock()
	h.subscribers = append(h.subscribers, s)
	h.mu.Unlock()
}

// Run 周期采集直到 ctx 结束
func (h *Hub) Run(ctx context.Context) {
	h.CollectOnce(ctx)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.CollectOnce(ctx)
		}
	}
}

// CollectOnce 采集一次并通知订阅者
func (h *Hub) CollectOnce(ctx context.Context) map[string]*DeviceTelemetry {
	devices, err := h.resources.ListResources(ctx, nil)
	if err != nil {
		return nil
	}
	samples := make(map[string]*DeviceTelemetry)
	var collectErr error
	if h.source != nil {
		cctx, cancel := context.WithTimeout(ctx, h.interval)
		got, err := h.source.Collect(cctx, devices)
		cancel()
		collectErr = err
		for k, v := range got {
			samples[k] = v
		}
	}
	if h.fallback != nil && len(samples) < len(devices) {
		var missing []*pool.Resource
		for _, d := range devices {
			if _, ok := samples[d.ID]; !ok {
				missing = append(missing, d)
			}
		}
		if got, err := h.fallback.Collect(ctx, missing); err == nil {
			for k, v := range got {
				samples[k] = v
			}
		}
	}

	h.mu.Lock()
	h.lastErr = collectErr
	for id, s := range samples {
		h.latest[id] = s
		hist := append(h.history[id], s)
		if len(hist) > h.historySize {
			hist = hist[len(hist)-h.historySize:]
		}
		h.history[id] = hist
	}
	subs := make([]Subscriber, len(h.subscribers))
	copy(subs, h.subscribers)
	h.mu.Unlock()

	for _, s := range subs {
		s(samples)
	}
	return samples
}

// Latest 返回最新样本（按资源 ID 排序）
func (h *Hub) Latest() []*DeviceTelemetry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*DeviceTelemetry, 0, len(h.latest))
	for _, s := range h.latest {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ResourceID < out[j].ResourceID })
	return out
}

// Get 获取单设备最新样本
func (h *Hub) Get(resourceID string) (*DeviceTelemetry, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.latest[resourceID]
	return s, ok
}

// History 获取单设备历史样本
func (h *Hub) History(resourceID string) []*DeviceTelemetry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]*DeviceTelemetry(nil), h.history[resourceID]...)
}

// SourceName 返回当前主数据源名称
func (h *Hub) SourceName() string {
	if h.source == nil {
		return "none"
	}
	return h.source.Name()
}

// LastError 返回最近一次主数据源采集错误
func (h *Hub) LastError() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastErr
}

// MetricsProvider 返回可注入资源池的指标提供函数
func (h *Hub) MetricsProvider() pool.MetricsProvider {
	return func(resourceID string) (*pool.ResourceMetrics, bool) {
		s, ok := h.Get(resourceID)
		if !ok {
			return nil, false
		}
		return &pool.ResourceMetrics{
			ResourceID:  s.ResourceID,
			NodeID:      s.NodeID,
			Type:        s.Type,
			Timestamp:   s.Timestamp,
			MemoryUsed:  int64(s.MemoryUsedGB),
			MemoryTotal: int64(s.MemoryTotalGB),
			Utilization: s.UtilizationPct,
			Temperature: s.TemperatureC,
			PowerUsage:  s.PowerW,
		}, true
	}
}
