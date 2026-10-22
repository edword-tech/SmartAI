/*
 * Unified Resource Manager - NPU Device Discoverer
 *
 * 发现方式：
 * 1. 节点 Agent 主动上报（POST /api/v1/nodes/register，推荐）
 * 2. 节点标签 discovery=local 时在本机执行 npu-smi 查询
 */

package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// NPUDeviceDiscoverer NPU 设备发现器
type NPUDeviceDiscoverer struct {
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewNPUDeviceDiscoverer 创建 NPU 设备发现器
func NewNPUDeviceDiscoverer() *NPUDeviceDiscoverer {
	return &NPUDeviceDiscoverer{run: runCommand}
}

// DiscoverGPUs 对于 NPU 发现器，GPU 返回空
func (d *NPUDeviceDiscoverer) DiscoverGPUs(ctx context.Context, node *Node) ([]*GPUInfo, error) {
	return []*GPUInfo{}, nil
}

// DiscoverNPUs 发现节点上的 NPU 设备
func (d *NPUDeviceDiscoverer) DiscoverNPUs(ctx context.Context, node *Node) ([]*NPUInfo, error) {
	if node.Labels["discovery"] == "local" {
		return d.discoverLocal(ctx)
	}
	if node.Resources != nil {
		return node.Resources.NPUs, nil
	}
	return []*NPUInfo{}, nil
}

func (d *NPUDeviceDiscoverer) discoverLocal(ctx context.Context) ([]*NPUInfo, error) {
	out, err := d.run(ctx, "npu-smi", "info", "-l")
	if err != nil {
		return nil, err
	}
	ids, names := ParseNPUSMIList(string(out))
	var npus []*NPUInfo
	for _, id := range ids {
		info := &NPUInfo{DeviceID: id, Model: "Ascend " + names[id], TotalCores: 100}
		if mem, err := d.run(ctx, "npu-smi", "info", "-t", "memory", "-i", id); err == nil {
			info.TotalMemory = ParseNPUSMIHBMCapacityGB(string(mem))
		}
		npus = append(npus, info)
	}
	if len(npus) == 0 {
		return nil, fmt.Errorf("npu-smi reported no devices")
	}
	return npus, nil
}

func splitKV(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// ParseNPUSMIList 解析 `npu-smi info -l`，返回 NPU ID 列表与 Product Name
func ParseNPUSMIList(out string) ([]string, map[string]string) {
	var ids []string
	names := make(map[string]string)
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		switch k {
		case "NPU ID":
			cur = v
			ids = append(ids, v)
		case "Product Name":
			if cur != "" {
				names[cur] = v
			}
		}
	}
	return ids, names
}

// ParseNPUSMIHBMCapacityGB 解析 `npu-smi info -t memory -i N` 中的 HBM Capacity(MB)
func ParseNPUSMIHBMCapacityGB(out string) int64 {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := splitKV(line)
		if !ok || !strings.HasPrefix(k, "HBM Capacity") {
			continue
		}
		if mb, err := strconv.ParseInt(v, 10, 64); err == nil {
			return (mb + 512) / 1024
		}
	}
	return 0
}
