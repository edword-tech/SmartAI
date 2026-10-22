package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// DCGM exporter 指标名（https://github.com/NVIDIA/dcgm-exporter）
const (
	DCGMUtil       = "DCGM_FI_DEV_GPU_UTIL"
	DCGMFBUsed     = "DCGM_FI_DEV_FB_USED" // MiB
	DCGMTemp       = "DCGM_FI_DEV_GPU_TEMP"
	DCGMPower      = "DCGM_FI_DEV_POWER_USAGE"
	DCGMSMClock    = "DCGM_FI_DEV_SM_CLOCK"
	DCGMXID        = "DCGM_FI_DEV_XID_ERRORS"
	DCGMECCSBE     = "DCGM_FI_DEV_ECC_SBE_VOL_TOTAL"
	DCGMECCDBE     = "DCGM_FI_DEV_ECC_DBE_VOL_TOTAL"
	DCGMPCIeReplay = "DCGM_FI_DEV_PCIE_REPLAY_COUNTER"
	DCGMNVLinkCRC  = "DCGM_FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL"
)

// PrometheusSource 通过 Prometheus HTTP API 查询 DCGM exporter 指标
type PrometheusSource struct {
	BaseURL string
	// NodeLabel 映射到节点 ID 的标签（dcgm-exporter 默认 Hostname）
	NodeLabel string
	// DeviceLabel 映射到设备 ID 的标签（dcgm-exporter 默认 gpu）
	DeviceLabel string
	Client      *http.Client
}

// NewPrometheusSource 创建 Prometheus 数据源
func NewPrometheusSource(baseURL string) *PrometheusSource {
	return &PrometheusSource{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		NodeLabel:   "Hostname",
		DeviceLabel: "gpu",
		Client:      &http.Client{Timeout: 5 * time.Second},
	}
}

// Name 数据源名称
func (p *PrometheusSource) Name() string { return "prometheus-dcgm" }

type promResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]interface{}    `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p *PrometheusSource) query(ctx context.Context, q string) (map[string]float64, error) {
	u := p.BaseURL + "/api/v1/query?query=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var pr promResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus query %q: %s", q, pr.Error)
	}
	out := make(map[string]float64, len(pr.Data.Result))
	for _, r := range pr.Data.Result {
		node, dev := r.Metric[p.NodeLabel], r.Metric[p.DeviceLabel]
		if node == "" || dev == "" {
			continue
		}
		s, _ := r.Value[1].(string)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			continue
		}
		out[pool.MakeResourceID(node, dev)] = v
	}
	return out, nil
}

// Collect 查询 DCGM 指标并映射到资源 ID
func (p *PrometheusSource) Collect(ctx context.Context, devices []*pool.Resource) (map[string]*DeviceTelemetry, error) {
	util, err := p.query(ctx, DCGMUtil)
	if err != nil {
		return nil, err
	}
	get := func(metric string) map[string]float64 {
		m, _ := p.query(ctx, metric)
		return m
	}
	fb, temp, power, clock := get(DCGMFBUsed), get(DCGMTemp), get(DCGMPower), get(DCGMSMClock)
	xid, sbe, dbe, replay, nvl := get(DCGMXID), get(DCGMECCSBE), get(DCGMECCDBE), get(DCGMPCIeReplay), get(DCGMNVLinkCRC)

	now := time.Now()
	out := make(map[string]*DeviceTelemetry)
	for _, d := range devices {
		u, ok := util[d.ID]
		if !ok {
			continue
		}
		t := &DeviceTelemetry{
			ResourceID: d.ID, NodeID: d.NodeID, DeviceID: d.DeviceID, Model: d.Model, Type: d.Type,
			Timestamp: now, UtilizationPct: u, MemoryUsedGB: fb[d.ID] / 1024, MemoryTotalGB: float64(d.TotalMemory),
			TemperatureC: temp[d.ID], PowerW: power[d.ID], PowerLimitW: PowerLimitFor(d.Model), SMClockMHz: clock[d.ID],
			XID: int(xid[d.ID]), ECCSingleBit: int64(sbe[d.ID]), ECCDoubleBit: int64(dbe[d.ID]),
			PCIeReplay: int64(replay[d.ID]), NVLinkCRCErrors: int64(nvl[d.ID]), Source: p.Name(),
		}
		out[d.ID] = t
	}
	return out, nil
}
