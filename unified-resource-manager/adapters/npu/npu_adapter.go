/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * NPU Service Adapter - 适配 Ascend-Virtual-Service
 */

package npu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// NPUAdapter NPU 服务适配器：维护 NPU 设备账本，并在分配/释放时调用 flexnpud
// 的 /device/register 与 /device/unregister 完成 vNPU 实例的创建与回收。
type NPUAdapter struct {
	hypervisorEndpoint string // flexnpud 默认端点；为空时仅记账（模拟模式）
	aclServerEndpoint  string

	ledger        *pool.Ledger
	nodeEndpoints map[string]string // 每个 NPU 节点上 flexnpud 的地址
	httpClient    *http.Client
	mu            sync.RWMutex
}

// NPUDevice NPU 设备信息
type NPUDevice = pool.DeviceSpec

// NewNPUAdapter 创建 NPU 适配器
func NewNPUAdapter(hypervisorEndpoint, aclServerEndpoint string) *NPUAdapter {
	return &NPUAdapter{
		hypervisorEndpoint: strings.TrimRight(hypervisorEndpoint, "/"),
		aclServerEndpoint:  aclServerEndpoint,
		ledger:             pool.NewLedger(pool.ResourceTypeNPU),
		nodeEndpoints:      make(map[string]string),
		httpClient:         &http.Client{Timeout: 5 * time.Second},
	}
}

// Ledger 返回 NPU 设备账本
func (na *NPUAdapter) Ledger() *pool.Ledger {
	return na.ledger
}

// SyncNode 同步节点 NPU 设备
func (na *NPUAdapter) SyncNode(nodeID string, devices []NPUDevice) {
	na.ledger.SyncNode(nodeID, devices)
}

// SetNodeEndpoint 设置节点 flexnpud 地址（如 http://10.0.0.5:8080）
func (na *NPUAdapter) SetNodeEndpoint(nodeID, endpoint string) {
	na.mu.Lock()
	defer na.mu.Unlock()
	na.nodeEndpoints[nodeID] = strings.TrimRight(endpoint, "/")
}

func (na *NPUAdapter) endpointFor(nodeID string) string {
	na.mu.RLock()
	defer na.mu.RUnlock()
	if ep, ok := na.nodeEndpoints[nodeID]; ok {
		return ep
	}
	return na.hypervisorEndpoint
}

// NewNPUResourcePool 创建 NPU 资源池；nodes 为空表示纳管全部 NPU 节点
func NewNPUResourcePool(adapter *NPUAdapter, poolID, name string, nodes []string) *pool.LedgerPool {
	return pool.NewLedgerPool(adapter.ledger, name, pool.ResourceTypeNPU, nodes, adapter)
}

type registerRequest struct {
	FlexNPUID   string `json:"flexnpu_id"`
	Entity      string `json:"entity"`
	NPU         int    `json:"npu"`
	Chip        int    `json:"chip"`
	AICoreAlloc int    `json:"aicore_alloc"` // 百分比
	MemoryAlloc int64  `json:"memory_alloc"` // MB
}

func flexNPUID(allocationID string, idx int) string {
	return fmt.Sprintf("%s-%d", allocationID, idx)
}

// OnAllocate 为每个切片在对应节点的 flexnpud 上注册 vNPU，失败时回滚已注册部分
func (na *NPUAdapter) OnAllocate(ctx context.Context, alloc *pool.LedgerAllocation) error {
	for i, s := range alloc.Slices {
		ep := na.endpointFor(s.NodeID)
		if ep == "" {
			continue
		}
		npuIdx, err := strconv.Atoi(s.DeviceID)
		if err != nil {
			return fmt.Errorf("invalid npu device id %q", s.DeviceID)
		}
		aicore := 100
		if r, ok := na.ledger.Get(s.ResourceID); ok && r.TotalCores > 0 {
			aicore = s.Cores * 100 / r.TotalCores
		}
		body := registerRequest{
			FlexNPUID:   flexNPUID(alloc.ID, i),
			Entity:      alloc.TaskID,
			NPU:         npuIdx,
			Chip:        0,
			AICoreAlloc: aicore,
			MemoryAlloc: s.MemoryGB * 1024,
		}
		if err := na.post(ctx, ep+"/device/register", body); err != nil {
			for j := 0; j < i; j++ {
				_ = na.post(ctx, na.endpointFor(alloc.Slices[j].NodeID)+"/device/unregister",
					map[string]string{"flexnpu_id": flexNPUID(alloc.ID, j)})
			}
			return err
		}
	}
	return nil
}

// OnRelease 注销 vNPU
func (na *NPUAdapter) OnRelease(ctx context.Context, alloc *pool.LedgerAllocation) error {
	var firstErr error
	for i, s := range alloc.Slices {
		ep := na.endpointFor(s.NodeID)
		if ep == "" {
			continue
		}
		if err := na.post(ctx, ep+"/device/unregister", map[string]string{"flexnpu_id": flexNPUID(alloc.ID, i)}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (na *NPUAdapter) post(ctx context.Context, url string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := na.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("flexnpud %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("flexnpud %s: %s %s", url, resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}
