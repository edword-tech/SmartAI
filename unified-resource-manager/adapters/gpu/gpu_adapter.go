/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * GPU Service Adapter - 适配 GPU-Virtual-Service
 */

package gpu

import (
	"flexai/unified-resource-manager/core/pool"
)

// K8s 侧 vGPU 资源与注解（与 GPU-Virtual-Service 的 GPU-device-plugin 保持一致）
const (
	ResourceVGPUNumber = "huawei.com/vgpu-number"
	ResourceVGPUMemory = "huawei.com/vgpu-memory"
	ResourceVGPUCores  = "huawei.com/vgpu-cores"
)

// GPUAdapter GPU 服务适配器：维护 GPU 设备账本。
// 设备实际切分与挂载由 GPU-device-plugin 在 Pod 绑定阶段按 huawei.com/vgpu-* 资源完成，
// 本适配器负责全局容量视图与分配记账，保证调度决策与设备插件一致。
type GPUAdapter struct {
	devicePluginEndpoint   string
	runtimeServiceEndpoint string
	exporterEndpoint       string

	ledger *pool.Ledger
}

// GPUDevice GPU 设备信息
type GPUDevice = pool.DeviceSpec

// NewGPUAdapter 创建 GPU 适配器
func NewGPUAdapter(devicePluginEndpoint, runtimeServiceEndpoint, exporterEndpoint string) *GPUAdapter {
	return &GPUAdapter{
		devicePluginEndpoint:   devicePluginEndpoint,
		runtimeServiceEndpoint: runtimeServiceEndpoint,
		exporterEndpoint:       exporterEndpoint,
		ledger:                 pool.NewLedger(pool.ResourceTypeGPU),
	}
}

// Ledger 返回 GPU 设备账本
func (ga *GPUAdapter) Ledger() *pool.Ledger {
	return ga.ledger
}

// ExporterEndpoint 返回 xpu-exporter / DCGM exporter 地址
func (ga *GPUAdapter) ExporterEndpoint() string {
	return ga.exporterEndpoint
}

// SyncNode 同步节点 GPU 设备（来自设备发现或节点注册）
func (ga *GPUAdapter) SyncNode(nodeID string, devices []GPUDevice) {
	ga.ledger.SyncNode(nodeID, devices)
}

// NewGPUResourcePool 创建 GPU 资源池；nodes 为空表示纳管全部 GPU 节点
func NewGPUResourcePool(adapter *GPUAdapter, poolID, name string, nodes []string) *pool.LedgerPool {
	return pool.NewLedgerPool(adapter.ledger, name, pool.ResourceTypeGPU, nodes, nil)
}
