/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - Metrics Collection
 */

package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"flexai/unified-resource-manager/core/pool"
)

// MetricsCollector 指标收集器
type MetricsCollector struct {
	// 资源池指标
	poolTotalResources *prometheus.GaugeVec
	poolAvailableResources *prometheus.GaugeVec
	poolAllocatedResources *prometheus.GaugeVec
	poolMemoryTotal *prometheus.GaugeVec
	poolMemoryUsed *prometheus.GaugeVec
	
	// 资源指标
	resourceUtilization *prometheus.GaugeVec
	resourceMemoryUsed *prometheus.GaugeVec
	resourceTemperature *prometheus.GaugeVec
	resourcePowerUsage *prometheus.GaugeVec
	
	// 任务指标
	taskTotal *prometheus.CounterVec
	taskDuration *prometheus.HistogramVec
	taskStatus *prometheus.GaugeVec
	
	// 分配指标
	allocationTotal *prometheus.CounterVec
	allocationDuration *prometheus.HistogramVec
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		poolTotalResources: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_pool_total_resources",
				Help: "Total number of resources in the pool",
			},
			[]string{"pool_id", "pool_name", "resource_type"},
		),
		poolAvailableResources: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_pool_available_resources",
				Help: "Number of available resources in the pool",
			},
			[]string{"pool_id", "pool_name", "resource_type"},
		),
		poolAllocatedResources: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_pool_allocated_resources",
				Help: "Number of allocated resources in the pool",
			},
			[]string{"pool_id", "pool_name", "resource_type"},
		),
		poolMemoryTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_pool_memory_total_gb",
				Help: "Total memory in GB in the pool",
			},
			[]string{"pool_id", "pool_name", "resource_type"},
		),
		poolMemoryUsed: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_pool_memory_used_gb",
				Help: "Used memory in GB in the pool",
			},
			[]string{"pool_id", "pool_name", "resource_type"},
		),
		resourceUtilization: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_utilization_percent",
				Help: "Resource utilization percentage",
			},
			[]string{"resource_id", "node_id", "resource_type"},
		),
		resourceMemoryUsed: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_memory_used_gb",
				Help: "Used memory in GB for the resource",
			},
			[]string{"resource_id", "node_id", "resource_type"},
		),
		resourceTemperature: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_temperature_celsius",
				Help: "Resource temperature in Celsius",
			},
			[]string{"resource_id", "node_id", "resource_type"},
		),
		resourcePowerUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_power_usage_watts",
				Help: "Resource power usage in Watts",
			},
			[]string{"resource_id", "node_id", "resource_type"},
		),
		taskTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "unified_resource_tasks_total",
				Help: "Total number of tasks",
			},
			[]string{"task_type", "status"},
		),
		taskDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "unified_resource_task_duration_seconds",
				Help: "Task duration in seconds",
				Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
			},
			[]string{"task_type", "status"},
		),
		taskStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "unified_resource_tasks_status",
				Help: "Current number of tasks by status",
			},
			[]string{"task_type", "status"},
		),
		allocationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "unified_resource_allocations_total",
				Help: "Total number of resource allocations",
			},
			[]string{"pool_id", "resource_type"},
		),
		allocationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "unified_resource_allocation_duration_seconds",
				Help: "Resource allocation duration in seconds",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
			},
			[]string{"pool_id", "resource_type"},
		),
	}
}

// UpdatePoolMetrics 更新资源池指标
func (mc *MetricsCollector) UpdatePoolMetrics(poolInfo *pool.PoolInfo) {
	poolID := poolInfo.ID
	poolName := poolInfo.Name
	resourceType := string(poolInfo.Type)
	
	mc.poolTotalResources.WithLabelValues(poolID, poolName, resourceType).Set(float64(poolInfo.TotalCount))
	mc.poolAvailableResources.WithLabelValues(poolID, poolName, resourceType).Set(float64(poolInfo.AvailableCount))
	mc.poolAllocatedResources.WithLabelValues(poolID, poolName, resourceType).Set(float64(poolInfo.AllocatedCount))
	mc.poolMemoryTotal.WithLabelValues(poolID, poolName, resourceType).Set(float64(poolInfo.TotalMemory))
	mc.poolMemoryUsed.WithLabelValues(poolID, poolName, resourceType).Set(float64(poolInfo.UsedMemory))
}

// UpdateResourceMetrics 更新资源指标
func (mc *MetricsCollector) UpdateResourceMetrics(metrics *pool.ResourceMetrics) {
	resourceID := metrics.ResourceID
	nodeID := "" // 需要从 metrics 中提取
	resourceType := "" // 需要从 metrics 中提取
	
	mc.resourceUtilization.WithLabelValues(resourceID, nodeID, resourceType).Set(metrics.Utilization)
	mc.resourceMemoryUsed.WithLabelValues(resourceID, nodeID, resourceType).Set(float64(metrics.MemoryUsed))
	mc.resourceTemperature.WithLabelValues(resourceID, nodeID, resourceType).Set(metrics.Temperature)
	mc.resourcePowerUsage.WithLabelValues(resourceID, nodeID, resourceType).Set(metrics.PowerUsage)
}

// RecordTask 记录任务指标
func (mc *MetricsCollector) RecordTask(taskType, status string, duration time.Duration) {
	mc.taskTotal.WithLabelValues(taskType, status).Inc()
	mc.taskDuration.WithLabelValues(taskType, status).Observe(duration.Seconds())
}

// UpdateTaskStatus 更新任务状态指标
func (mc *MetricsCollector) UpdateTaskStatus(taskType, status string, count float64) {
	mc.taskStatus.WithLabelValues(taskType, status).Set(count)
}

// RecordAllocation 记录分配指标
func (mc *MetricsCollector) RecordAllocation(poolID, resourceType string, duration time.Duration) {
	mc.allocationTotal.WithLabelValues(poolID, resourceType).Inc()
	mc.allocationDuration.WithLabelValues(poolID, resourceType).Observe(duration.Seconds())
}

// StartCollection 启动指标收集
func (mc *MetricsCollector) StartCollection(ctx context.Context, resourcePool *pool.UnifiedResourcePool) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 定期更新资源池指标
			pools, err := resourcePool.ListPools(ctx)
			if err == nil {
				for _, poolInfo := range pools {
					mc.UpdatePoolMetrics(poolInfo)
				}
			}
		}
	}
}
