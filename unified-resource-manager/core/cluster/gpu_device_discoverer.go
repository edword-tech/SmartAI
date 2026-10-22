/*
 * Unified Resource Manager - GPU Device Discoverer
 *
 * 发现方式：
 * 1. 节点 Agent 主动上报（POST /api/v1/nodes/register，推荐，适用于远端节点）
 * 2. 节点标签 discovery=local 时在本机执行 nvidia-smi 查询（管理面与 GPU 同机部署）
 */

package cluster

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"flexai/unified-resource-manager/core/pool"
)

// GPUDeviceDiscoverer GPU 设备发现器
type GPUDeviceDiscoverer struct {
	// run 执行外部命令，便于测试替换
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewGPUDeviceDiscoverer 创建 GPU 设备发现器
func NewGPUDeviceDiscoverer() *GPUDeviceDiscoverer {
	return &GPUDeviceDiscoverer{run: runCommand}
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// DiscoverGPUs 发现节点上的 GPU 设备
func (d *GPUDeviceDiscoverer) DiscoverGPUs(ctx context.Context, node *Node) ([]*GPUInfo, error) {
	if node.Labels["discovery"] == "local" {
		return d.discoverLocal(ctx)
	}
	if node.Resources != nil {
		return node.Resources.GPUs, nil
	}
	return []*GPUInfo{}, nil
}

// discoverLocal 通过 nvidia-smi 查询本机 GPU 与 NUMA 亲和
func (d *GPUDeviceDiscoverer) discoverLocal(ctx context.Context) ([]*GPUInfo, error) {
	out, err := d.run(ctx, "nvidia-smi", "--query-gpu=index,name,memory.total", "--format=csv,noheader,nounits")
	if err != nil {
		return nil, err
	}
	gpus, err := ParseNvidiaSMIQuery(string(out))
	if err != nil {
		return nil, err
	}
	if topo, err := d.run(ctx, "nvidia-smi", "topo", "-m"); err == nil {
		numa := ParseNvidiaSMITopoNUMA(string(topo))
		for _, g := range gpus {
			if n, ok := numa[g.DeviceID]; ok {
				g.Topology = &pool.DeviceTopology{NUMANode: n}
			}
		}
	}
	return gpus, nil
}

// ParseNvidiaSMIQuery 解析 `nvidia-smi --query-gpu=index,name,memory.total --format=csv,noheader,nounits`
func ParseNvidiaSMIQuery(out string) ([]*GPUInfo, error) {
	var gpus []*GPUInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			return nil, fmt.Errorf("unexpected nvidia-smi line: %q", line)
		}
		memMiB, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse memory %q: %w", fields[2], err)
		}
		gpus = append(gpus, &GPUInfo{
			DeviceID:    strings.TrimSpace(fields[0]),
			Model:       strings.TrimSpace(fields[1]),
			TotalMemory: (memMiB + 512) / 1024,
			TotalCores:  100,
		})
	}
	return gpus, nil
}

// ParseNvidiaSMITopoNUMA 从 `nvidia-smi topo -m` 输出中解析每张 GPU 的 NUMA Affinity 列
func ParseNvidiaSMITopoNUMA(out string) map[string]int {
	res := make(map[string]int)
	lines := strings.Split(out, "\n")
	numaCol := -1
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if numaCol < 0 {
			for i, f := range fields {
				if f == "NUMA" {
					// 表头行无行首标签列，数据行首列为 GPUx
					numaCol = i + 1
					break
				}
			}
			continue
		}
		if !strings.HasPrefix(fields[0], "GPU") || numaCol >= len(fields) {
			continue
		}
		if n, err := strconv.Atoi(fields[numaCol]); err == nil {
			res[strings.TrimPrefix(fields[0], "GPU")] = n
		}
	}
	return res
}

// DiscoverNPUs 对于 GPU 发现器，NPU 返回空
func (d *GPUDeviceDiscoverer) DiscoverNPUs(ctx context.Context, node *Node) ([]*NPUInfo, error) {
	return []*NPUInfo{}, nil
}
