/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - Main Entry Point
 */

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flexai/unified-resource-manager/adapters/gpu"
	"flexai/unified-resource-manager/adapters/npu"
	"flexai/unified-resource-manager/api/rest"
	"flexai/unified-resource-manager/auth/rbac"
	"flexai/unified-resource-manager/auth/tenant"
	"flexai/unified-resource-manager/core/cluster"
	"flexai/unified-resource-manager/core/compute"
	"flexai/unified-resource-manager/core/nim"
	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/core/scheduler"
	"flexai/unified-resource-manager/observability/metrics"
	"flexai/unified-resource-manager/ops/benchmark"
	"flexai/unified-resource-manager/ops/probe"
	"flexai/unified-resource-manager/ops/remediation"
)

var (
	port              = flag.Int("port", 8080, "API server port")
	gpuDevicePluginEndpoint = flag.String("gpu-device-plugin", "unix:///var/lib/kubelet/device-plugins/gpu.sock", "GPU device plugin endpoint")
	gpuRuntimeEndpoint = flag.String("gpu-runtime", "unix:///var/run/gpu-runtime.sock", "GPU runtime service endpoint")
	gpuExporterEndpoint = flag.String("gpu-exporter", "http://localhost:9091", "GPU exporter endpoint")
	npuHypervisorEndpoint = flag.String("npu-hypervisor", "", "NPU hypervisor (flexnpud) endpoint; empty = ledger-only")
	npuAclServerEndpoint = flag.String("npu-acl-server", "", "NPU ACL server endpoint")
	prometheusURL         = flag.String("prometheus", "", "Prometheus base URL for DCGM telemetry (or URM_PROMETHEUS_URL)")
)

func main() {
	flag.Parse()
	
	log.Println("Starting Unified Resource Manager...")
	
	// 创建资源池管理器
	resourcePool := pool.NewUnifiedResourcePool()
	
	// 注册默认的资源规格（ResourceFlavor）
	initResourceFlavors(resourcePool)
	
	// 创建 GPU 适配器和资源池
	gpuAdapter := gpu.NewGPUAdapter(
		*gpuDevicePluginEndpoint,
		*gpuRuntimeEndpoint,
		*gpuExporterEndpoint,
	)
	gpuPool := gpu.NewGPUResourcePool(gpuAdapter, "gpu-pool-1", "GPU Pool 1", []string{})
	resourcePool.RegisterPool("gpu-pool-1", gpuPool)
	
	// 创建 NPU 适配器和资源池
	npuAdapter := npu.NewNPUAdapter(
		*npuHypervisorEndpoint,
		*npuAclServerEndpoint,
	)
	npuPool := npu.NewNPUResourcePool(npuAdapter, "npu-pool-1", "NPU Pool 1", []string{})
	resourcePool.RegisterPool("npu-pool-1", npuPool)
	
	// 创建调度器，并挂上节点可调度性 / RDMA 域检查器
	sched := scheduler.NewIntelligentScheduler(resourcePool, &scheduler.BinpackStrategy{})
	initSchedulerQueues(sched)

	// 创建集群管理器
	clusterMgr := cluster.NewClusterManager(30*time.Second, 5*time.Second)
	clusterMgr.SetDeviceDiscoverer(cluster.NewHybridDeviceDiscoverer())
	sched.SetNodeInspector(clusterMgr)

	// 遥测 + 自愈 + 压测 + 诊断
	prom := firstNonEmpty(*prometheusURL, os.Getenv("URM_PROMETHEUS_URL"), *gpuExporterEndpoint)
	hub, sim := newTelemetryHub(resourcePool, prom)
	gpuPool.SetMetricsProvider(hub.MetricsProvider())
	npuPool.SetMetricsProvider(hub.MetricsProvider())
	engine := remediation.NewEngine(&platformActuator{pool: resourcePool, sched: sched})
	hub.Subscribe(engine.Evaluate)
	bench := benchmark.NewManager(&platformInventory{cluster: clusterMgr, pool: resourcePool}, hub, sim)
	bench.OnAcceptance(func(nodeID string, passed bool, run *benchmark.Run) {
		node, err := clusterMgr.GetNode(nodeID)
		if err != nil {
			return
		}
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		if passed {
			node.Labels["acceptance"] = "passed"
			_ = clusterMgr.SetCordon(nodeID, false)
		} else {
			node.Labels["acceptance"] = "failed"
			_ = clusterMgr.SetCordon(nodeID, true)
			log.Printf("Acceptance failed for %s, node cordoned", nodeID)
		}
	})
	wireClusterOps(clusterMgr, gpuAdapter, npuAdapter, engine)

	// 初始化示例节点（带 NUMA / NVLink / RDMA 拓扑），监听器会同步到账本
	initExampleNodes(clusterMgr)
	if reserved, err := probe.ReserveCapacity(context.Background(), resourcePool); err != nil {
		log.Printf("Reserve debug capacity: %v", err)
	} else {
		log.Printf("Reserved GPU/NPU debug devices: %v", reserved)
	}
	probeMgr := probe.New(resourcePool)
	if boot := probeMgr.Probe(context.Background(), nil); boot.Status != probe.StatusPass {
		log.Printf("Startup GPU/NPU probe FAILED: %s", boot.Summary)
	} else {
		log.Printf("Startup GPU/NPU probe passed: %s", boot.Summary)
	}

	metricsCollector := metrics.NewMetricsCollector()
	computePerception := compute.NewComputePerception(clusterMgr, resourcePool)
	computeQuality := compute.NewComputeQuality(resourcePool)
	computeOrchestration := compute.NewComputeOrchestration(sched, resourcePool)
	trainingAcceleration := compute.NewTrainingAcceleration(sched, resourcePool)
	clusterPoolMgr := pool.NewClusterPoolManager()
	subPoolMgr := pool.NewSubPoolManager()

	nimStore := nim.NewStore()
	apiServer := rest.NewAPIServer(*port, resourcePool, sched)
	apiServer.WithClusterManager(clusterMgr)
	apiServer.WithNIMConfig(nimStore)
	apiServer.WithComputeModules(computePerception, computeQuality, computeOrchestration, trainingAcceleration)
	apiServer.WithClusterPoolManagers(clusterPoolMgr, subPoolMgr)
	apiServer.WithOps(&rest.OpsRuntime{
		Hub: hub, Sim: sim, Engine: engine, Benchmark: bench, Diagnoser: newDiagnoser(), Probe: probeMgr,
	})
	initRBACAndTenants(apiServer.GetRBACManager(), apiServer.GetTenantManager())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	if err := clusterMgr.Start(); err != nil {
		log.Fatalf("Failed to start cluster manager: %v", err)
	}
	if err := computePerception.Start(); err != nil {
		log.Fatalf("Failed to start compute perception: %v", err)
	}
	go metricsCollector.StartCollection(ctx, resourcePool)
	go hub.Run(ctx)

	go func() {
		if err := apiServer.Start(); err != nil {
			log.Fatalf("Failed to start API server: %v", err)
		}
	}()

	log.Printf("Unified Resource Manager started on port %d (scheduler + ops wired)", *port)
	
	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Println("Shutting down...")
	
	// 优雅关闭
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	apiServer.Stop(shutdownCtx)
	sched.Stop(shutdownCtx)
	clusterMgr.Stop()
	computePerception.Stop()
	
	log.Println("Shutdown complete")
}

// initResourceFlavors 初始化默认的资源规格
func initResourceFlavors(resourcePool *pool.UnifiedResourcePool) {
	flavors := []*pool.ResourceFlavor{
		{
			ID:        "gpu.a100.20g.infer",
			Name:      "A100 20GB 推理规格",
			Type:      pool.ResourceTypeGPU,
			Model:     "A100",
			MemoryGB:  20,
			Cores:     50, // 50% 算力
			ForUsage:  "inference",
			Labels:    map[string]string{"use_case": "inference", "model_size": "medium"},
		},
		{
			ID:        "gpu.a100.40g.train",
			Name:      "A100 40GB 训练规格",
			Type:      pool.ResourceTypeGPU,
			Model:     "A100",
			MemoryGB:  40,
			Cores:     100, // 100% 算力（整卡）
			ForUsage:  "training",
			Labels:    map[string]string{"use_case": "training", "model_size": "large"},
		},
		{
			ID:        "gpu.4090.24g.infer",
			Name:      "RTX 4090 24GB 推理规格",
			Type:      pool.ResourceTypeGPU,
			Model:     "RTX 4090",
			MemoryGB:  24,
			Cores:     50,
			ForUsage:  "inference",
			Labels:    map[string]string{"use_case": "inference", "model_size": "small"},
		},
		{
			ID:        "npu.910b3.8g.infer",
			Name:      "Ascend 910B3 8GB 推理规格",
			Type:      pool.ResourceTypeNPU,
			Model:     "Ascend 910B3",
			MemoryGB:  8,
			Cores:     2, // aicore_alloc = 2
			ForUsage:  "inference",
			Labels:    map[string]string{"use_case": "inference", "model_size": "small"},
		},
		{
			ID:        "npu.910b3.16g.train",
			Name:      "Ascend 910B3 16GB 训练规格",
			Type:      pool.ResourceTypeNPU,
			Model:     "Ascend 910B3",
			MemoryGB:  16,
			Cores:     8,
			ForUsage:  "training",
			Labels:    map[string]string{"use_case": "training", "model_size": "medium"},
		},
		{
			ID:       "gpu.h100.80g.train",
			Name:     "H100 80GB 整卡训练",
			Type:     pool.ResourceTypeGPU,
			Model:    "H100",
			MemoryGB: 80,
			Cores:    100,
			ForUsage: "training",
			Labels:   map[string]string{"use_case": "training", "share_mode": "exclusive"},
		},
		{
			ID:       "gpu.a100.1g10gb.mig",
			Name:     "A100 MIG 1g.10gb",
			Type:     pool.ResourceTypeGPU,
			Model:    "A100",
			MemoryGB: 10,
			Cores:    14,
			ForUsage: "inference",
			Labels:   map[string]string{"use_case": "inference", "share_mode": "mig", "mig_profile": "1g.10gb"},
		},
	}
	
	for _, flavor := range flavors {
		resourcePool.RegisterFlavor(flavor)
		log.Printf("Registered resource flavor: %s (%s)", flavor.ID, flavor.Name)
	}
}

// initExampleNodes 初始化示例节点（用于演示）
func initExampleNodes(clusterMgr *cluster.ClusterManager) {
	nodes := []*cluster.Node{
		{
			ID:      "node-gpu-1",
			Name:    "GPU训练节点-A100",
			Type:    cluster.NodeTypeGPU,
			State:   cluster.NodeStateOnline,
			Address: "192.168.1.10",
			Labels:  map[string]string{"zone": "zone-a", "rack": "r1", "gpu.model": "A100", "agent": "static"},
			Network: rdmaNet("rail-a", 200, 2),
			Resources: &cluster.NodeResources{
				GPUs: gpuSpecs("A100-SXM4-40GB", 8, 40, "nvl-a"),
			},
		},
		{
			ID:      "node-gpu-2",
			Name:    "GPU训练节点-H100",
			Type:    cluster.NodeTypeGPU,
			State:   cluster.NodeStateOnline,
			Address: "192.168.1.11",
			Labels:  map[string]string{"zone": "zone-a", "rack": "r1", "gpu.model": "H100", "agent": "static"},
			Network: rdmaNet("rail-a", 400, 2),
			Resources: &cluster.NodeResources{
				GPUs: gpuSpecs("H100-SXM5-80GB", 8, 80, "nvl-h"),
			},
		},
		{
			ID:      "node-npu-1",
			Name:    "NPU节点-910B3",
			Type:    cluster.NodeTypeNPU,
			State:   cluster.NodeStateOnline,
			Address: "192.168.1.20",
			Labels:  map[string]string{"zone": "zone-b", "rack": "r2", "npu.model": "910B3", "agent": "static"},
			Network: &cluster.NodeNetwork{
				RDMADomain: "rail-b",
				RDMANics:   []*cluster.RDMANic{{Name: "hns_0", RateGbps: 200, State: "active", LinkType: "RoCE"}},
			},
			Resources: &cluster.NodeResources{
				NPUs: npuSpecs("Ascend 910B3", 8, 32, 10, "hccs"),
			},
		},
		{
			ID:      "node-hybrid-1",
			Name:    "混合推理节点",
			Type:    cluster.NodeTypeHybrid,
			State:   cluster.NodeStateOnline,
			Address: "192.168.1.30",
			Labels:  map[string]string{"zone": "zone-c", "purpose": "inference", "agent": "static"},
			Network: rdmaNet("rail-c", 200, 1),
			Resources: &cluster.NodeResources{
				GPUs: gpuSpecs("A100-SXM4-40GB", 4, 40, "nvl-hy"),
				NPUs: npuSpecs("Ascend 910B3", 2, 32, 10, "hccs-hy"),
			},
		},
	}
	
	for _, node := range nodes {
		if err := clusterMgr.RegisterNode(node); err != nil {
			log.Printf("Failed to register node %s: %v", node.ID, err)
		} else {
			log.Printf("Registered node: %s (%s)", node.ID, node.Name)
		}
	}
}

// initRBACAndTenants 初始化 RBAC 和租户示例数据
func initRBACAndTenants(rbacMgr *rbac.RBACManager, tenantMgr *tenant.TenantManager) {
	// 创建示例用户
	users := []*rbac.User{
		{
			ID:       "user-admin",
			Username: "admin",
			Email:    "admin@example.com",
			Roles:    []rbac.Role{rbac.RoleAdmin},
			TenantID: "tenant-admin",
		},
		{
			ID:       "user-operator",
			Username: "operator",
			Email:    "operator@example.com",
			Roles:    []rbac.Role{rbac.RoleOperator},
			TenantID: "tenant-team1",
		},
		{
			ID:       "user-dev1",
			Username: "dev1",
			Email:    "dev1@example.com",
			Roles:    []rbac.Role{rbac.RoleUser},
			TenantID: "tenant-team1",
		},
		{
			ID:       "user-viewer",
			Username: "viewer",
			Email:    "viewer@example.com",
			Roles:    []rbac.Role{rbac.RoleViewer},
			TenantID: "tenant-team2",
		},
	}
	
	for _, user := range users {
		if err := rbacMgr.CreateUser(user); err != nil {
			log.Printf("Failed to create user %s: %v", user.ID, err)
		} else {
			log.Printf("Created user: %s (%s) with roles: %v", user.ID, user.Username, user.Roles)
		}
	}
	
	// 创建示例租户
	tenants := []*tenant.Tenant{
		{
			ID:   "tenant-admin",
			Name: "管理员租户",
			Quota: &tenant.TenantQuota{
				MaxGPUs:     100,
				MaxNPUs:     50,
				MaxMemoryGB: 10240,
				MaxTasks:    1000,
			},
			Used: &tenant.TenantUsage{},
		},
		{
			ID:   "tenant-team1",
			Name: "团队1",
			Quota: &tenant.TenantQuota{
				MaxGPUs:     20,
				MaxNPUs:     10,
				MaxMemoryGB: 2048,
				MaxTasks:    200,
			},
			Used: &tenant.TenantUsage{},
		},
		{
			ID:   "tenant-team2",
			Name: "团队2",
			Quota: &tenant.TenantQuota{
				MaxGPUs:     10,
				MaxNPUs:     5,
				MaxMemoryGB: 1024,
				MaxTasks:    100,
			},
			Used: &tenant.TenantUsage{},
		},
	}
	
	for _, t := range tenants {
		if err := tenantMgr.CreateTenant(t); err != nil {
			log.Printf("Failed to create tenant %s: %v", t.ID, err)
		} else {
			log.Printf("Created tenant: %s (%s) with quota: GPUs=%d, NPUs=%d, Memory=%dGB, Tasks=%d",
				t.ID, t.Name, t.Quota.MaxGPUs, t.Quota.MaxNPUs, t.Quota.MaxMemoryGB, t.Quota.MaxTasks)
		}
	}
}
