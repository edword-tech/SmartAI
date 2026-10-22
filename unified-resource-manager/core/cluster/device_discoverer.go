/*
 * Unified Resource Manager - Device Discoverer
 *
 * 负责从底层 GPU / NPU 系统或已有组件（如 GPU-device-plugin、flexnpud、xpu-exporter）
 * 中发现节点上的物理设备信息，并填充到 NodeResources 中。
 *
 * 当前实现为占位 / 示例，实现了接口和基础结构，具体对接可根据实际部署环境补充：
 * - GPU：可通过 GPU-device-plugin 的 gRPC / HTTP 接口或 NVML 查询
 * - NPU：可通过 flexnpud 的 HTTP 接口 / dcmi 查询
 */

package cluster

import (
	"context"
	"log"
)

// DeviceDiscoverer 设备发现接口，按节点发现 GPU / NPU 信息。
type DeviceDiscoverer interface {
	DiscoverGPUs(ctx context.Context, node *Node) ([]*GPUInfo, error)
	DiscoverNPUs(ctx context.Context, node *Node) ([]*NPUInfo, error)
}

// DummyDeviceDiscoverer 一个默认的占位实现，不做真实发现，仅返回已有信息或空列表。
// 这样可以保证编译运行，通过配置可以逐步替换为真实实现。
type DummyDeviceDiscoverer struct{}

// DiscoverGPUs 返回当前节点已有的 GPU 列表（如果已在 Node.Resources 中填充），否则为空。
func (d *DummyDeviceDiscoverer) DiscoverGPUs(ctx context.Context, node *Node) ([]*GPUInfo, error) {
	if node.Resources != nil && len(node.Resources.GPUs) > 0 {
		return node.Resources.GPUs, nil
	}
	log.Printf("[cluster] DummyDeviceDiscoverer: no GPU info for node %s, return empty list", node.ID)
	return []*GPUInfo{}, nil
}

// DiscoverNPUs 返回当前节点已有的 NPU 列表（如果已在 Node.Resources 中填充），否则为空。
func (d *DummyDeviceDiscoverer) DiscoverNPUs(ctx context.Context, node *Node) ([]*NPUInfo, error) {
	if node.Resources != nil && len(node.Resources.NPUs) > 0 {
		return node.Resources.NPUs, nil
	}
	log.Printf("[cluster] DummyDeviceDiscoverer: no NPU info for node %s, return empty list", node.ID)
	return []*NPUInfo{}, nil
}
