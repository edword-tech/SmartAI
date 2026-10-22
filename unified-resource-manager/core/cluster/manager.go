/*
 * Unified Resource Manager - Cluster Manager
 *
 * 节点注册、心跳与健康检查、隔离 (cordon)、设备发现，以及节点变化事件分发。
 */

package cluster

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ClusterManager 集群管理器
type ClusterManager struct {
	nodes map[string]*Node
	mu    sync.RWMutex

	deviceDiscoverer DeviceDiscoverer
	listeners        []NodeListener

	// 心跳超过 healthCheckInterval*missedHeartbeats 未更新即判定节点失联
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	missedHeartbeats    int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewClusterManager 创建集群管理器
func NewClusterManager(healthCheckInterval, healthCheckTimeout time.Duration) *ClusterManager {
	ctx, cancel := context.WithCancel(context.Background())
	if healthCheckInterval == 0 {
		healthCheckInterval = 30 * time.Second
	}
	if healthCheckTimeout == 0 {
		healthCheckTimeout = 5 * time.Second
	}
	return &ClusterManager{
		nodes:               make(map[string]*Node),
		healthCheckInterval: healthCheckInterval,
		healthCheckTimeout:  healthCheckTimeout,
		missedHeartbeats:    3,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// SetDeviceDiscoverer 设置设备发现器。如果未设置，将默认使用 DummyDeviceDiscoverer。
func (cm *ClusterManager) SetDeviceDiscoverer(d DeviceDiscoverer) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.deviceDiscoverer = d
}

func (cm *ClusterManager) getDeviceDiscoverer() DeviceDiscoverer {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.deviceDiscoverer != nil {
		return cm.deviceDiscoverer
	}
	return &DummyDeviceDiscoverer{}
}

// AddListener 注册节点变化回调
func (cm *ClusterManager) AddListener(l NodeListener) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.listeners = append(cm.listeners, l)
}

func (cm *ClusterManager) notify(node *Node, event NodeEvent) {
	cm.mu.RLock()
	ls := append([]NodeListener(nil), cm.listeners...)
	cm.mu.RUnlock()
	for _, l := range ls {
		l(node, event)
	}
}

// RegisterNode 注册节点；已存在时更新资源、地址与标签（节点 Agent 重复注册是幂等的）
func (cm *ClusterManager) RegisterNode(node *Node) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	cm.mu.Lock()
	if node.ID == "" {
		node.ID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	now := time.Now()
	if existing, ok := cm.nodes[node.ID]; ok {
		node.CreatedAt = existing.CreatedAt
		node.Cordoned = existing.Cordoned
	} else {
		node.CreatedAt = now
	}
	node.UpdatedAt = now
	node.LastSeen = now
	if node.State == "" {
		node.State = NodeStateOnline
	}
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	cm.nodes[node.ID] = node
	cm.mu.Unlock()
	cm.notify(node, NodeEventRegistered)
	return nil
}

// RemoveNode 移除节点
func (cm *ClusterManager) RemoveNode(nodeID string) error {
	cm.mu.Lock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		cm.mu.Unlock()
		return fmt.Errorf("node not found: %s", nodeID)
	}
	delete(cm.nodes, nodeID)
	cm.mu.Unlock()
	cm.notify(node, NodeEventRemoved)
	return nil
}

// GetNode 获取节点
func (cm *ClusterManager) GetNode(nodeID string) (*Node, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	return node, nil
}

// ListNodes 列出所有节点（按 ID 排序）
func (cm *ClusterManager) ListNodes(filters map[string]string) ([]*Node, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var nodes []*Node
	for _, node := range cm.nodes {
		if matchNodeFilters(node, filters) {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, nil
}

// IsSchedulable 节点是否可接收新任务
func (cm *ClusterManager) IsSchedulable(nodeID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		// 未纳管到集群管理器的节点（如仅通过资源池注册）不做限制
		return true
	}
	return node.State == NodeStateOnline && !node.Cordoned
}

// NodeLabels 返回节点标签副本，并附加 RDMA 域拓扑标签（topology.rdma_domain）
func (cm *ClusterManager) NodeLabels(nodeID string) map[string]string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(node.Labels)+1)
	for k, v := range node.Labels {
		out[k] = v
	}
	if node.Network != nil && node.Network.RDMADomain != "" {
		out["topology.rdma_domain"] = node.Network.RDMADomain
	}
	return out
}

// UpdateNodeState 更新节点状态，并在进入/离开异常状态时发出事件
func (cm *ClusterManager) UpdateNodeState(nodeID string, state NodeState) error {
	cm.mu.Lock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		cm.mu.Unlock()
		return fmt.Errorf("node not found: %s", nodeID)
	}
	prev := node.State
	node.State = state
	node.UpdatedAt = time.Now()
	cm.mu.Unlock()

	failed := func(s NodeState) bool { return s == NodeStateError || s == NodeStateOffline }
	switch {
	case !failed(prev) && failed(state):
		cm.notify(node, NodeEventFailed)
	case failed(prev) && state == NodeStateOnline:
		cm.notify(node, NodeEventRecovered)
	}
	return nil
}

// SetCordon 隔离/解除隔离节点
func (cm *ClusterManager) SetCordon(nodeID string, cordoned bool) error {
	cm.mu.Lock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		cm.mu.Unlock()
		return fmt.Errorf("node not found: %s", nodeID)
	}
	changed := node.Cordoned != cordoned
	node.Cordoned = cordoned
	node.UpdatedAt = time.Now()
	cm.mu.Unlock()
	if changed {
		if cordoned {
			cm.notify(node, NodeEventCordoned)
		} else {
			cm.notify(node, NodeEventUncordoned)
		}
	}
	return nil
}

// Heartbeat 节点 Agent 心跳；失联节点收到心跳后自动恢复
func (cm *ClusterManager) Heartbeat(nodeID string) error {
	cm.mu.Lock()
	node, ok := cm.nodes[nodeID]
	if !ok {
		cm.mu.Unlock()
		return fmt.Errorf("node not found: %s", nodeID)
	}
	node.LastSeen = time.Now()
	recovered := node.State == NodeStateError || node.State == NodeStateOffline
	cm.mu.Unlock()
	if recovered {
		return cm.UpdateNodeState(nodeID, NodeStateOnline)
	}
	return nil
}

// UpdateNodeLastSeen 兼容旧接口，等价于心跳
func (cm *ClusterManager) UpdateNodeLastSeen(nodeID string) {
	_ = cm.Heartbeat(nodeID)
}

// Start 启动集群管理器
func (cm *ClusterManager) Start() error {
	cm.wg.Add(1)
	go cm.healthCheckLoop()
	return nil
}

// Stop 停止集群管理器
func (cm *ClusterManager) Stop() {
	cm.cancel()
	cm.wg.Wait()
}

func (cm *ClusterManager) healthCheckLoop() {
	defer cm.wg.Done()
	ticker := time.NewTicker(cm.healthCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.PerformHealthCheck()
		}
	}
}

// PerformHealthCheck 检查所有节点心跳，失联节点置为 error
func (cm *ClusterManager) PerformHealthCheck() {
	cm.mu.RLock()
	nodes := make([]*Node, 0, len(cm.nodes))
	for _, node := range cm.nodes {
		nodes = append(nodes, node)
	}
	cm.mu.RUnlock()

	for _, node := range nodes {
		ctx, cancel := context.WithTimeout(cm.ctx, cm.healthCheckTimeout)
		healthy := cm.checkNodeHealth(ctx, node)
		cancel()
		if !healthy && node.State == NodeStateOnline {
			_ = cm.UpdateNodeState(node.ID, NodeStateError)
		}
	}
}

func (cm *ClusterManager) checkNodeHealth(ctx context.Context, node *Node) bool {
	cm.mu.RLock()
	lastSeen := node.LastSeen
	skip := node.Labels["health.heartbeat"] == "disabled" || node.Labels["agent"] == "static"
	cm.mu.RUnlock()
	if skip {
		return true
	}
	return time.Since(lastSeen) <= cm.healthCheckInterval*time.Duration(cm.missedHeartbeats)
}

// DiscoverResources 发现节点资源
func (cm *ClusterManager) DiscoverResources(nodeID string) (*NodeResources, error) {
	node, err := cm.GetNode(nodeID)
	if err != nil {
		return nil, err
	}
	discoverer := cm.getDeviceDiscoverer()
	res := &NodeResources{}
	ctx := cm.ctx
	switch node.Type {
	case NodeTypeGPU:
		if res.GPUs, err = discoverer.DiscoverGPUs(ctx, node); err != nil {
			return nil, err
		}
	case NodeTypeNPU:
		if res.NPUs, err = discoverer.DiscoverNPUs(ctx, node); err != nil {
			return nil, err
		}
	case NodeTypeHybrid:
		if res.GPUs, err = discoverer.DiscoverGPUs(ctx, node); err != nil {
			return nil, err
		}
		if res.NPUs, err = discoverer.DiscoverNPUs(ctx, node); err != nil {
			return nil, err
		}
	default:
		res.GPUs, _ = discoverer.DiscoverGPUs(ctx, node)
		res.NPUs, _ = discoverer.DiscoverNPUs(ctx, node)
	}

	cm.mu.Lock()
	node.Resources = res
	node.UpdatedAt = time.Now()
	cm.mu.Unlock()
	cm.notify(node, NodeEventResourcesUpdated)
	return res, nil
}

// GetClusterStats 获取集群统计信息
func (cm *ClusterManager) GetClusterStats() (*ClusterStats, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	stats := &ClusterStats{TotalNodes: len(cm.nodes)}
	for _, node := range cm.nodes {
		switch node.State {
		case NodeStateOnline:
			stats.OnlineNodes++
		case NodeStateOffline:
			stats.OfflineNodes++
		case NodeStateError:
			stats.ErrorNodes++
		}
		if node.Cordoned {
			stats.CordonedNodes++
		}
		if node.Resources != nil {
			for _, gpu := range node.Resources.GPUs {
				stats.TotalGPUs++
				stats.TotalGPUMemory += gpu.TotalMemory
			}
			for _, npu := range node.Resources.NPUs {
				stats.TotalNPUs++
				stats.TotalNPUMemory += npu.TotalMemory
			}
		}
	}
	return stats, nil
}

// ClusterStats 集群统计信息
type ClusterStats struct {
	TotalNodes     int   `json:"total_nodes"`
	OnlineNodes    int   `json:"online_nodes"`
	OfflineNodes   int   `json:"offline_nodes"`
	ErrorNodes     int   `json:"error_nodes"`
	CordonedNodes  int   `json:"cordoned_nodes"`
	TotalGPUs      int   `json:"total_gpus"`
	TotalNPUs      int   `json:"total_npus"`
	TotalGPUMemory int64 `json:"total_gpu_memory"` // GB
	TotalNPUMemory int64 `json:"total_npu_memory"` // GB
}

func matchNodeFilters(node *Node, filters map[string]string) bool {
	for k, v := range filters {
		switch k {
		case "state":
			if string(node.State) != v {
				return false
			}
		case "type":
			if string(node.Type) != v {
				return false
			}
		default:
			if node.Labels[k] != v {
				return false
			}
		}
	}
	return true
}
