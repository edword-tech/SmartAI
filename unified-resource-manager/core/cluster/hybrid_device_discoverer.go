/*
 * Unified Resource Manager - Hybrid Device Discoverer
 *
 * 混合节点发现器，同时支持 GPU 和 NPU 设备发现
 */

package cluster

import (
	"context"
)

// HybridDeviceDiscoverer 混合设备发现器
type HybridDeviceDiscoverer struct {
	gpuDiscoverer DeviceDiscoverer
	npuDiscoverer DeviceDiscoverer
}

// NewHybridDeviceDiscoverer 创建混合设备发现器
func NewHybridDeviceDiscoverer() *HybridDeviceDiscoverer {
	return &HybridDeviceDiscoverer{
		gpuDiscoverer: NewGPUDeviceDiscoverer(),
		npuDiscoverer: NewNPUDeviceDiscoverer(),
	}
}

// DiscoverGPUs 发现 GPU 设备
func (d *HybridDeviceDiscoverer) DiscoverGPUs(ctx context.Context, node *Node) ([]*GPUInfo, error) {
	return d.gpuDiscoverer.DiscoverGPUs(ctx, node)
}

// DiscoverNPUs 发现 NPU 设备
func (d *HybridDeviceDiscoverer) DiscoverNPUs(ctx context.Context, node *Node) ([]*NPUInfo, error) {
	return d.npuDiscoverer.DiscoverNPUs(ctx, node)
}
