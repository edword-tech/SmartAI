package telemetry

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// Fault 注入的故障
type Fault struct {
	XID             int     `json:"xid,omitempty"`
	ECCDoubleBit    int64   `json:"ecc_dbe,omitempty"`
	ECCSingleBit    int64   `json:"ecc_sbe,omitempty"`
	TemperatureAdd  float64 `json:"temperature_add,omitempty"`
	NVLinkCRCErrors int64   `json:"nvlink_crc_errors,omitempty"`
	PCIeReplay      int64   `json:"pcie_replay,omitempty"`
	// PerfFactor 性能折损系数（0-1），用于压测结果仿真，0 表示不折损
	PerfFactor float64   `json:"perf_factor,omitempty"`
	Note       string    `json:"note,omitempty"`
	InjectedAt time.Time `json:"injected_at"`
}

// SimulatedSource 基于账本占用的遥测仿真，支持故障注入
type SimulatedSource struct {
	faults map[string]*Fault
	now    func() time.Time
	mu     sync.RWMutex
}

// NewSimulatedSource 创建仿真数据源
func NewSimulatedSource() *SimulatedSource {
	return &SimulatedSource{faults: make(map[string]*Fault), now: time.Now}
}

// Name 数据源名称
func (s *SimulatedSource) Name() string { return "simulated" }

// InjectFault 为设备注入故障（累加到已有故障上）
func (s *SimulatedSource) InjectFault(resourceID string, f Fault) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.InjectedAt = s.now()
	if old, ok := s.faults[resourceID]; ok {
		if f.XID == 0 {
			f.XID = old.XID
		}
		f.ECCDoubleBit += old.ECCDoubleBit
		f.ECCSingleBit += old.ECCSingleBit
		f.NVLinkCRCErrors += old.NVLinkCRCErrors
		f.PCIeReplay += old.PCIeReplay
		if f.TemperatureAdd == 0 {
			f.TemperatureAdd = old.TemperatureAdd
		}
		if f.PerfFactor == 0 {
			f.PerfFactor = old.PerfFactor
		}
	}
	s.faults[resourceID] = &f
}

// ClearFault 清除设备故障（模拟 GPU reset / 更换）
func (s *SimulatedSource) ClearFault(resourceID string) {
	s.mu.Lock()
	delete(s.faults, resourceID)
	s.mu.Unlock()
}

// Faults 返回当前注入的故障
func (s *SimulatedSource) Faults() map[string]Fault {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Fault, len(s.faults))
	for k, v := range s.faults {
		out[k] = *v
	}
	return out
}

// Fault 查询单设备故障
func (s *SimulatedSource) Fault(resourceID string) (Fault, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.faults[resourceID]
	if !ok {
		return Fault{}, false
	}
	return *f, true
}

func jitter(id string, t time.Time, amplitude float64) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	phase := float64(h.Sum32()%360) * math.Pi / 180
	return amplitude * math.Sin(float64(t.Unix())/30+phase)
}

// PowerLimitFor 按型号返回典型 TDP（W）
func PowerLimitFor(model string) float64 {
	m := strings.ToUpper(model)
	switch {
	case strings.Contains(m, "H100"), strings.Contains(m, "H800"):
		return 700
	case strings.Contains(m, "A100"), strings.Contains(m, "A800"):
		return 400
	case strings.Contains(m, "4090"):
		return 450
	case strings.Contains(m, "910"):
		return 400
	}
	return 300
}

// Collect 生成样本
func (s *SimulatedSource) Collect(ctx context.Context, devices []*pool.Resource) (map[string]*DeviceTelemetry, error) {
	now := s.now()
	out := make(map[string]*DeviceTelemetry, len(devices))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range devices {
		util := 0.0
		if d.TotalCores > 0 && d.Holders > 0 {
			util = math.Min(100, float64(d.UsedCores)/float64(d.TotalCores)*90+5+jitter(d.ID, now, 4))
		}
		limit := PowerLimitFor(d.Model)
		t := &DeviceTelemetry{
			ResourceID:     d.ID,
			NodeID:         d.NodeID,
			DeviceID:       d.DeviceID,
			Model:          d.Model,
			Type:           d.Type,
			Timestamp:      now,
			UtilizationPct: math.Max(0, util),
			MemoryUsedGB:   float64(d.UsedMemory),
			MemoryTotalGB:  float64(d.TotalMemory),
			TemperatureC:   34 + util*0.45 + jitter(d.ID+"t", now, 1.5),
			PowerLimitW:    limit,
			PowerW:         limit*0.15 + limit*0.8*util/100,
			SMClockMHz:     1410,
			Source:         s.Name(),
		}
		if f, ok := s.faults[d.ID]; ok {
			t.XID = f.XID
			t.ECCDoubleBit = f.ECCDoubleBit
			t.ECCSingleBit = f.ECCSingleBit
			t.NVLinkCRCErrors = f.NVLinkCRCErrors
			t.PCIeReplay = f.PCIeReplay
			t.TemperatureC += f.TemperatureAdd
			if f.XID == 79 {
				// GPU 掉卡后驱动无法读取指标
				t.UtilizationPct, t.PowerW, t.SMClockMHz = 0, 0, 0
			}
		}
		if t.TemperatureC >= 87 {
			t.Throttle = append(t.Throttle, "hw_thermal_slowdown")
			t.SMClockMHz = 1000
		}
		out[d.ID] = t
	}
	return out, nil
}
