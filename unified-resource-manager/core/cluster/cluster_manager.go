/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - Cluster Management
 */

package cluster

import (
	"time"

	"flexai/unified-resource-manager/core/pool"
)

// NodeState 节点状态
type NodeState string

const (
	NodeStateOnline      NodeState = "online"
	NodeStateOffline     NodeState = "offline"
	NodeStateMaintenance NodeState = "maintenance"
	NodeStateError       NodeState = "error"
)

// NodeType 节点类型
type NodeType string

const (
	NodeTypeGPU NodeType = "gpu"
	NodeTypeNPU NodeType = "npu"
	NodeTypeHybrid NodeType = "hybrid" // 同时支持 GPU 和 NPU
)

// Node 集群节点
type Node struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        NodeType          `json:"type"`
	State       NodeState         `json:"state"`
	Address     string            `json:"address"`
	Labels      map[string]string `json:"labels"`
	Resources   *NodeResources    `json:"resources"`
	Network     *NodeNetwork      `json:"network,omitempty"`
	// Cordoned 节点被隔离后不再接收新任务（已运行任务不受影响，drain 时迁移）
	Cordoned    bool              `json:"cordoned"`
	LastSeen    time.Time         `json:"last_seen"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// NodeNetwork 节点高速网络信息
type NodeNetwork struct {
	// RDMADomain 同一 RDMA 域（同 Leaf/Spine 或同轨道 rail）内节点间通信开销最低
	RDMADomain string     `json:"rdma_domain,omitempty"`
	RDMANics   []*RDMANic `json:"rdma_nics,omitempty"`
}

// RDMANic RDMA 网卡（IB / RoCE）
type RDMANic struct {
	Name     string `json:"name"`      // mlx5_0
	RateGbps int    `json:"rate_gbps"` // 200 / 400
	State    string `json:"state"`     // active / down
	LinkType string `json:"link_type"` // InfiniBand / Ethernet
}

// NodeResources 节点资源信息
type NodeResources struct {
	GPUs []*GPUInfo `json:"gpus,omitempty"`
	NPUs []*NPUInfo `json:"npus,omitempty"`
}

// GPUInfo GPU 信息
type GPUInfo struct {
	DeviceID    string               `json:"device_id"`
	Model       string               `json:"model"`
	TotalMemory int64                `json:"total_memory"` // GB
	TotalCores  int                  `json:"total_cores"`
	Topology    *pool.DeviceTopology `json:"topology,omitempty"`
}

// NPUInfo NPU 信息
type NPUInfo struct {
	DeviceID    string               `json:"device_id"`
	Model       string               `json:"model"`
	TotalMemory int64                `json:"total_memory"` // GB
	TotalCores  int                  `json:"total_cores"`  // aicore_alloc quota
	Topology    *pool.DeviceTopology `json:"topology,omitempty"`
}

// NodeEvent 节点变化事件
type NodeEvent string

const (
	NodeEventRegistered       NodeEvent = "registered"
	NodeEventResourcesUpdated NodeEvent = "resources_updated"
	NodeEventFailed           NodeEvent = "failed"
	NodeEventRecovered        NodeEvent = "recovered"
	NodeEventCordoned         NodeEvent = "cordoned"
	NodeEventUncordoned       NodeEvent = "uncordoned"
	NodeEventRemoved          NodeEvent = "removed"
)

// NodeListener 节点变化回调（在锁外调用）
type NodeListener func(node *Node, event NodeEvent)

// GPUSpecs 转换为设备账本规格
func (r *NodeResources) GPUSpecs() []pool.DeviceSpec {
	if r == nil {
		return nil
	}
	out := make([]pool.DeviceSpec, 0, len(r.GPUs))
	for _, g := range r.GPUs {
		out = append(out, pool.DeviceSpec{DeviceID: g.DeviceID, Model: g.Model, TotalMemory: g.TotalMemory, TotalCores: g.TotalCores, Topology: g.Topology})
	}
	return out
}

// NPUSpecs 转换为设备账本规格
func (r *NodeResources) NPUSpecs() []pool.DeviceSpec {
	if r == nil {
		return nil
	}
	out := make([]pool.DeviceSpec, 0, len(r.NPUs))
	for _, n := range r.NPUs {
		out = append(out, pool.DeviceSpec{DeviceID: n.DeviceID, Model: n.Model, TotalMemory: n.TotalMemory, TotalCores: n.TotalCores, Topology: n.Topology})
	}
	return out
}

