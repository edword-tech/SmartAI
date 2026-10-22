/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - REST API Server
 */

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"flexai/unified-resource-manager/auth/rbac"
	"flexai/unified-resource-manager/auth/tenant"
	"flexai/unified-resource-manager/core/cluster"
	"flexai/unified-resource-manager/core/compute"
	"flexai/unified-resource-manager/core/models"
	"flexai/unified-resource-manager/core/nim"
	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/core/project"
	"flexai/unified-resource-manager/core/scheduler"
	"flexai/unified-resource-manager/ops/benchmark"
	"flexai/unified-resource-manager/ops/diagnosis"
	"flexai/unified-resource-manager/ops/probe"
	"flexai/unified-resource-manager/ops/remediation"
	"flexai/unified-resource-manager/ops/telemetry"
)

// APIServer REST API 服务器
type APIServer struct {
	router       *mux.Router
	resourcePool *pool.UnifiedResourcePool
	scheduler    scheduler.Scheduler
	clusterMgr   *cluster.ClusterManager
	rbacMgr      *rbac.RBACManager
	tenantMgr    *tenant.TenantManager
	modelStore   *models.Store
	projectStore *project.Store
	nimStore     *nim.Store

	// 算力管理模块（独立管理）
	computePerception    *compute.ComputePerception
	computeQuality       *compute.ComputeQuality
	computeOrchestration *compute.ComputeOrchestration
	trainingAcceleration *compute.TrainingAcceleration

	// 集群大池与子池
	clusterPoolMgr *pool.ClusterPoolManager
	subPoolMgr     *pool.SubPoolManager

	// 部署记录：大模型部署在池分配上的绑定
	deploymentStore map[string]*DeploymentRecord
	deploymentMu    sync.RWMutex

	// 运维与调度扩展（可选注入）
	ops *OpsRuntime

	server       *http.Server
}

// OpsRuntime 运维运行时：遥测、自愈、压测、诊断
type OpsRuntime struct {
	Hub       *telemetry.Hub
	Sim       *telemetry.SimulatedSource
	Engine    *remediation.Engine
	Benchmark *benchmark.Manager
	Diagnoser *diagnosis.Diagnoser
	Probe     *probe.Manager
}

// DeploymentRecord 大模型部署记录（跑在池化/切分资源上）
type DeploymentRecord struct {
	ID            string    `json:"id"`
	ModelID       string    `json:"model_id"`
	AllocationID  string    `json:"allocation_id"`
	PoolID        string    `json:"pool_id"`
	Status        string    `json:"status"` // running / stopped
	CreatedAt     time.Time `json:"created_at"`
}

// NewAPIServer 创建 API 服务器
func NewAPIServer(port int, resourcePool *pool.UnifiedResourcePool, sched scheduler.Scheduler) *APIServer {
	router := mux.NewRouter()
	
	api := &APIServer{
		router:          router,
		resourcePool:    resourcePool,
		scheduler:       sched,
		clusterMgr:      nil,
		rbacMgr:         rbac.NewRBACManager(),
		tenantMgr:       tenant.NewTenantManager(),
		modelStore:      models.NewStore(),
		projectStore:    project.NewStore(),
		deploymentStore: make(map[string]*DeploymentRecord),
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
	
	api.setupRoutes()
	return api
}

// WithClusterManager 为 APIServer 注入 ClusterManager，用于节点与设备管理相关接口。
func (s *APIServer) WithClusterManager(cm *cluster.ClusterManager) {
	s.clusterMgr = cm
}

// WithNIMConfig 注入 NIM 配置存储，用于 NVIDIA NIM 推理后端配置。
func (s *APIServer) WithNIMConfig(store *nim.Store) {
	s.nimStore = store
}

// WithComputeModules 注入算力管理模块（独立管理）
func (s *APIServer) WithComputeModules(
	perception *compute.ComputePerception,
	quality *compute.ComputeQuality,
	orchestration *compute.ComputeOrchestration,
	acceleration *compute.TrainingAcceleration,
) {
	s.computePerception = perception
	s.computeQuality = quality
	s.computeOrchestration = orchestration
	s.trainingAcceleration = acceleration
}

// WithClusterPoolManagers 注入集群池与子池管理器（大池/切分池化）
func (s *APIServer) WithClusterPoolManagers(clusterMgr *pool.ClusterPoolManager, subMgr *pool.SubPoolManager) {
	s.clusterPoolMgr = clusterMgr
	s.subPoolMgr = subMgr
}

// WithOps 注入运维运行时
func (s *APIServer) WithOps(ops *OpsRuntime) {
	s.ops = ops
}

func (s *APIServer) intel() *scheduler.IntelligentScheduler {
	if is, ok := s.scheduler.(*scheduler.IntelligentScheduler); ok {
		return is
	}
	return nil
}

// GetRBACManager 获取 RBAC 管理器（用于初始化示例数据）
func (s *APIServer) GetRBACManager() *rbac.RBACManager {
	return s.rbacMgr
}

// GetTenantManager 获取租户管理器（用于初始化示例数据）
func (s *APIServer) GetTenantManager() *tenant.TenantManager {
	return s.tenantMgr
}

// setupRoutes 设置路由
func (s *APIServer) setupRoutes() {
	api := s.router.PathPrefix("/api/v1").Subrouter()
	
	// 资源池管理
	api.HandleFunc("/pools", s.listPools).Methods("GET")
	api.HandleFunc("/pools", s.createPool).Methods("POST")
	api.HandleFunc("/pools/{poolId}", s.getPool).Methods("GET")
	api.HandleFunc("/pools/{poolId}/resources", s.listResources).Methods("GET")
	api.HandleFunc("/pools/{poolId}/resources/{resourceId}", s.getResource).Methods("GET")
	api.HandleFunc("/pools/{poolId}/resources/{resourceId}/metrics", s.getResourceMetrics).Methods("GET")
	
	// 资源分配
	api.HandleFunc("/pools/{poolId}/allocate", s.allocateResources).Methods("POST")
	api.HandleFunc("/pools/{poolId}/release/{allocationId}", s.releaseResources).Methods("POST")
	api.HandleFunc("/pools/{poolId}/allocations", s.listPoolAllocations).Methods("GET")
	api.HandleFunc("/pools/{poolId}/slots", s.listPoolSlots).Methods("GET")
	api.HandleFunc("/pools/{poolId}/deploy", s.deployModelOnPool).Methods("POST")
	api.HandleFunc("/deployments", s.listDeployments).Methods("GET")

	// 集群大池（多节点 GPU 聚合成大池，供大模型集群调度）
	api.HandleFunc("/cluster-pools", s.listClusterPools).Methods("GET")
	api.HandleFunc("/cluster-pools", s.createClusterPool).Methods("POST")
	api.HandleFunc("/cluster-pools/{clusterPoolId}", s.getClusterPool).Methods("GET")
	api.HandleFunc("/cluster-pools/{clusterPoolId}", s.updateClusterPool).Methods("PUT")
	api.HandleFunc("/cluster-pools/{clusterPoolId}", s.deleteClusterPool).Methods("DELETE")
	api.HandleFunc("/cluster-pools/{clusterPoolId}/info", s.getClusterPoolInfo).Methods("GET")
	api.HandleFunc("/cluster-pools/{clusterPoolId}/allocate", s.allocateFromClusterPool).Methods("POST")
	api.HandleFunc("/cluster-pools/{clusterPoolId}/resources", s.listClusterPoolResources).Methods("GET")

	// 子池（大池内切分/配额）
	api.HandleFunc("/sub-pools", s.listSubPools).Methods("GET")
	api.HandleFunc("/sub-pools", s.createSubPool).Methods("POST")
	api.HandleFunc("/sub-pools/{subPoolId}", s.getSubPool).Methods("GET")
	api.HandleFunc("/sub-pools/{subPoolId}", s.deleteSubPool).Methods("DELETE")
	api.HandleFunc("/sub-pools/{subPoolId}/usage", s.getSubPoolUsage).Methods("GET")
	api.HandleFunc("/sub-pools/{subPoolId}/allocate", s.allocateFromSubPool).Methods("POST")
	
	// 资源规格
	api.HandleFunc("/flavors", s.listFlavors).Methods("GET")
	
	// 任务管理
	api.HandleFunc("/tasks", s.listTasks).Methods("GET")
	api.HandleFunc("/tasks", s.submitTask).Methods("POST")
	api.HandleFunc("/tasks/{taskId}", s.getTask).Methods("GET")
	api.HandleFunc("/tasks/{taskId}", s.cancelTask).Methods("DELETE")
	api.HandleFunc("/tasks/{taskId}/complete", s.completeTask).Methods("POST")

	// 调度器（队列 / 配置 / 评估 / 碎片整理）
	api.HandleFunc("/scheduler/stats", s.getSchedulerStats).Methods("GET")
	api.HandleFunc("/scheduler/config", s.getSchedulerConfig).Methods("GET")
	api.HandleFunc("/scheduler/config", s.updateSchedulerConfig).Methods("PUT")
	api.HandleFunc("/scheduler/evaluate", s.evaluatePlacement).Methods("POST")
	api.HandleFunc("/scheduler/queues", s.listQueues).Methods("GET")
	api.HandleFunc("/scheduler/queues", s.upsertQueue).Methods("POST")
	api.HandleFunc("/scheduler/queues/{name}", s.getQueue).Methods("GET")
	api.HandleFunc("/scheduler/queues/{name}", s.deleteQueue).Methods("DELETE")
	api.HandleFunc("/scheduler/fragmentation", s.getFragmentation).Methods("GET")
	api.HandleFunc("/scheduler/defrag", s.planDefrag).Methods("GET")
	api.HandleFunc("/scheduler/defrag", s.executeDefrag).Methods("POST")

	// K8s Scheduler Extender（与 kube-scheduler 的 HTTP 扩展器协议对齐）
	s.router.HandleFunc("/scheduler/extender/filter", s.k8sExtenderFilter).Methods("POST")
	s.router.HandleFunc("/scheduler/extender/prioritize", s.k8sExtenderPrioritize).Methods("POST")
	s.router.HandleFunc("/scheduler/extender/bind", s.k8sExtenderBind).Methods("POST")

	// 节点管理
	api.HandleFunc("/nodes", s.listNodes).Methods("GET")
	api.HandleFunc("/nodes", s.registerNode).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}", s.getNode).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/discover", s.discoverNodeResources).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/heartbeat", s.nodeHeartbeat).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/cordon", s.cordonNode).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/drain", s.drainNode).Methods("POST")

	// 运维：遥测 / 自愈 / 压测 / 诊断
	api.HandleFunc("/ops/overview", s.opsOverview).Methods("GET")
	api.HandleFunc("/ops/telemetry", s.listTelemetry).Methods("GET")
	api.HandleFunc("/ops/telemetry/{resourceId}", s.getTelemetry).Methods("GET")
	api.HandleFunc("/ops/events", s.listOpsEvents).Methods("GET")
	api.HandleFunc("/ops/rules", s.listOpsRules).Methods("GET")
	api.HandleFunc("/ops/rules/{ruleId}", s.updateOpsRule).Methods("PUT")
	api.HandleFunc("/ops/conditions", s.listOpsConditions).Methods("GET")
	api.HandleFunc("/ops/devices/{resourceId}/reset", s.resetDevice).Methods("POST")
	api.HandleFunc("/ops/devices/{resourceId}/fault", s.injectFault).Methods("POST")
	api.HandleFunc("/ops/devices/{resourceId}/fault", s.clearFault).Methods("DELETE")
	api.HandleFunc("/ops/benchmark/baselines", s.listBaselines).Methods("GET")
	api.HandleFunc("/ops/benchmark/baselines", s.setBaseline).Methods("POST")
	api.HandleFunc("/ops/benchmark/runs", s.listBenchmarkRuns).Methods("GET")
	api.HandleFunc("/ops/benchmark/runs", s.runBenchmark).Methods("POST")
	api.HandleFunc("/ops/benchmark/runs/{runId}", s.getBenchmarkRun).Methods("GET")
	api.HandleFunc("/ops/benchmark/runs/{runId}/calibrate", s.calibrateBaseline).Methods("POST")
	api.HandleFunc("/ops/benchmark/ingest", s.ingestBenchmark).Methods("POST")
	api.HandleFunc("/ops/diagnose", s.diagnose).Methods("POST")
	api.HandleFunc("/ops/probe", s.getProbe).Methods("GET")
	api.HandleFunc("/ops/probe", s.runProbe).Methods("POST")
	api.HandleFunc("/ops/readiness", s.opsReadiness).Methods("GET")

	// Prometheus 指标
	s.router.Handle("/metrics", promhttp.Handler()).Methods("GET")
	
	// NVIDIA NIM 配置
	api.HandleFunc("/nim/config", s.getNIMConfig).Methods("GET")
	api.HandleFunc("/nim/config", s.updateNIMConfig).Methods("PUT")

	// 模型市场
	api.HandleFunc("/models", s.listModels).Methods("GET")
	api.HandleFunc("/models", s.createModel).Methods("POST")
	api.HandleFunc("/models/{modelId}", s.getModel).Methods("GET")
	api.HandleFunc("/models/{modelId}", s.deleteModel).Methods("DELETE")
	
	// 项目管理
	api.HandleFunc("/projects", s.listProjects).Methods("GET")
	api.HandleFunc("/projects", s.createProject).Methods("POST")
	api.HandleFunc("/projects/{projectId}", s.getProject).Methods("GET")
	api.HandleFunc("/projects/{projectId}", s.updateProject).Methods("PUT")
	api.HandleFunc("/projects/{projectId}", s.deleteProject).Methods("DELETE")
	
	// 算力感知 API
	api.HandleFunc("/compute/perception/states", s.listResourceStates).Methods("GET")
	api.HandleFunc("/compute/perception/states/{resourceId}", s.getResourceState).Methods("GET")
	api.HandleFunc("/compute/perception/bind", s.bindResource).Methods("POST")
	api.HandleFunc("/compute/perception/unbind", s.unbindResource).Methods("POST")
	api.HandleFunc("/compute/perception/cluster-settings", s.getClusterSettings).Methods("GET")
	api.HandleFunc("/compute/perception/cluster-settings", s.updateClusterSettings).Methods("PUT")
	
	// 算力质量 API
	api.HandleFunc("/compute/quality/slicing-configs", s.listSlicingConfigs).Methods("GET")
	api.HandleFunc("/compute/quality/slicing-configs", s.createSlicingConfig).Methods("POST")
	api.HandleFunc("/compute/quality/exclusive-configs", s.listExclusiveConfigs).Methods("GET")
	api.HandleFunc("/compute/quality/exclusive-configs", s.createExclusiveConfig).Methods("POST")
	api.HandleFunc("/compute/quality/allocation-strategies", s.listAllocationStrategies).Methods("GET")
	api.HandleFunc("/compute/quality/allocation-strategies", s.createAllocationStrategy).Methods("POST")
	
	// 算力编排 API
	api.HandleFunc("/compute/orchestration/distribute", s.distributeTask).Methods("POST")
	api.HandleFunc("/compute/orchestration/optimize", s.optimizeScheduling).Methods("POST")
	api.HandleFunc("/compute/orchestration/strategies", s.getOrchestrationStrategies).Methods("GET")
	
	// 训练加速 API
	api.HandleFunc("/compute/acceleration/data-loading", s.listDataLoadingConfigs).Methods("GET")
	api.HandleFunc("/compute/acceleration/data-loading", s.configureDataLoading).Methods("POST")
	api.HandleFunc("/compute/acceleration/distributed", s.listDistributedConfigs).Methods("GET")
	api.HandleFunc("/compute/acceleration/distributed", s.configureDistributedTraining).Methods("POST")
	api.HandleFunc("/compute/acceleration/checkpoint", s.listCheckpointConfigs).Methods("GET")
	api.HandleFunc("/compute/acceleration/checkpoint", s.configureCheckpoint).Methods("POST")
	api.HandleFunc("/compute/acceleration/memory", s.listMemoryOptimizationConfigs).Methods("GET")
	api.HandleFunc("/compute/acceleration/memory", s.configureMemoryOptimization).Methods("POST")
	api.HandleFunc("/compute/acceleration/optimize/{taskId}", s.optimizeTask).Methods("POST")
	
	// 健康检查
	s.router.HandleFunc("/health", s.healthCheck).Methods("GET")
	s.router.HandleFunc("/ready", s.readyCheck).Methods("GET")
}

// Start 启动服务器
func (s *APIServer) Start() error {
	log.Printf("Starting API server on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

// Stop 停止服务器
func (s *APIServer) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// 请求/响应结构

type CreatePoolRequest struct {
	Name        string            `json:"name"`
	Type        pool.ResourceType `json:"type"`
	Nodes       []string          `json:"nodes"`
	Quota       *PoolQuota        `json:"quota,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type PoolQuota struct {
	MaxCount  int   `json:"max_count,omitempty"`
	MaxMemory int64 `json:"max_memory_gb,omitempty"`
}

type SubmitTaskRequest struct {
	Name                    string                  `json:"name"`
	Type                    scheduler.TaskType      `json:"type"`
	Priority                scheduler.TaskPriority  `json:"priority"`
	ResourceReq             *pool.AllocationRequest `json:"resource_req"`
	InferenceBackend        string                  `json:"inference_backend,omitempty"`
	ProjectID               string                  `json:"project_id,omitempty"`
	ModelID                 string                  `json:"model_id,omitempty"`
	Queue                   string                  `json:"queue,omitempty"`
	GangGroup               string                  `json:"gang_group,omitempty"`
	MinMember               int                     `json:"min_member,omitempty"`
	MinCount                int                     `json:"min_count,omitempty"`
	NodeSelector            map[string]string       `json:"node_selector,omitempty"`
	NonPreemptible          bool                    `json:"non_preemptible,omitempty"`
	ExpectedDurationSeconds int                     `json:"expected_duration_seconds,omitempty"`
	Labels                  map[string]string       `json:"labels,omitempty"`
}

// CreateModelRequest 用于创建模型的请求结构。
type CreateModelRequest struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Version           string            `json:"version"`
	Type              string            `json:"type"`
	Framework         string            `json:"framework"`
	Format            string            `json:"format"`
	ResourceFlavorID  string            `json:"resource_flavor_id"`
	Path              string            `json:"path"`
	Description       string            `json:"description"`
	ContextLength     int               `json:"context_length"`
	Size              int               `json:"size"`
	Family            string            `json:"family"`
	Languages         []string          `json:"languages"`
	Abilities         []string          `json:"abilities"`
	ProjectID         string            `json:"project_id"`
	InferenceBackend  string            `json:"inference_backend,omitempty"` // gpu / nvidia_nim
	NIMModelID        string            `json:"nim_model_id,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
}

// CreateProjectRequest 用于创建项目的请求结构。
type CreateProjectRequest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	TenantID    string            `json:"tenant_id"`
	OwnerID     string            `json:"owner_id"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// 处理器方法

func (s *APIServer) listPools(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requirePermission(w, r, rbac.PermissionViewPool); !ok {
		return
	}
	ctx := r.Context()
	
	pools, err := s.resourcePool.ListPools(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, pools)
}

func (s *APIServer) createPool(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requirePermission(w, r, rbac.PermissionCreatePool); !ok {
		return
	}
	var req CreatePoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	// 根据类型创建对应的资源池
	// 这里简化实现，实际需要调用适配器创建
	respondJSON(w, http.StatusCreated, map[string]string{
		"message": "Pool created successfully",
		"pool_id": "pool-" + fmt.Sprintf("%d", time.Now().UnixNano()),
	})
}

func (s *APIServer) getPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	
	ctx := r.Context()
	pool := s.resourcePool.GetPool(poolID)
	if pool == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("pool not found: %s", poolID))
		return
	}
	
	info, err := pool.GetPoolInfo(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, info)
}

func (s *APIServer) listResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	
	ctx := r.Context()
	pool := s.resourcePool.GetPool(poolID)
	if pool == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("pool not found: %s", poolID))
		return
	}
	
	// 解析查询参数作为过滤器
	filters := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			filters[k] = v[0]
		}
	}
	
	resources, err := pool.ListResources(ctx, filters)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, resources)
}

func (s *APIServer) getResource(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	resourceID := vars["resourceId"]
	
	ctx := r.Context()
	pool := s.resourcePool.GetPool(poolID)
	if pool == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("pool not found: %s", poolID))
		return
	}
	
	resource, err := pool.GetResource(ctx, resourceID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	
	respondJSON(w, http.StatusOK, resource)
}

func (s *APIServer) getResourceMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	resourceID := vars["resourceId"]
	
	ctx := r.Context()
	pool := s.resourcePool.GetPool(poolID)
	if pool == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("pool not found: %s", poolID))
		return
	}
	
	// 解析 duration 参数
	duration := 1 * time.Hour
	if d := r.URL.Query().Get("duration"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			duration = parsed
		}
	}
	
	metrics, err := pool.GetResourceMetrics(ctx, resourceID, duration)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, metrics)
}

func (s *APIServer) allocateResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	
	var req pool.AllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if _, _, ok := s.requirePermission(w, r, rbac.PermissionAllocate); !ok {
		return
	}
	
	ctx := r.Context()
	result, err := s.resourcePool.AllocateFromPool(ctx, poolID, &req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, result)
}

func (s *APIServer) releaseResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	allocationID := vars["allocationId"]
	
	if _, _, ok := s.requirePermission(w, r, rbac.PermissionRelease); !ok {
		return
	}
	
	ctx := r.Context()
	if err := s.resourcePool.ReleaseAllocation(ctx, poolID, allocationID); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Resources released successfully",
	})
}

// listPoolAllocations 列出某池的已分配记录，感知已分配资源与池化剩余
func (s *APIServer) listPoolAllocations(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	list := s.resourcePool.ListAllocations(poolID)
	respondJSON(w, http.StatusOK, list)
}

// PoolSlot 池内槽位（按切分配置划分，用于 GPU 分割视图）
type PoolSlot struct {
	SlotID     string `json:"slot_id"`
	ResourceID string `json:"resource_id"`
	NodeID     string `json:"node_id"`
	MemoryGB   int64  `json:"memory_gb"`
	Cores      int    `json:"cores"`
	State      string `json:"state"`
}

// listPoolSlots 列出池内槽位（按 slice_config_id 切分），感知可分配槽位与已占槽位
func (s *APIServer) listPoolSlots(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	sliceConfigID := r.URL.Query().Get("slice_config_id")
	ctx := r.Context()
	p := s.resourcePool.GetPool(poolID)
	if p == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("pool not found: %s", poolID))
		return
	}
	resources, err := p.ListResources(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	var slots []PoolSlot
	if sliceConfigID != "" && s.computeQuality != nil {
		config, err := s.computeQuality.GetSlicingConfig(sliceConfigID)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Errorf("slice config not found: %s", sliceConfigID))
			return
		}
		memPerSlot := config.SliceSize.MemoryGB
		if memPerSlot <= 0 {
			memPerSlot = 1
		}
		coresPerSlot := config.SliceSize.Cores
		maxSlices := config.MaxSlices
		if maxSlices <= 0 {
			maxSlices = 8
		}
		for _, res := range resources {
			n := int(res.TotalMemory / memPerSlot)
			if n > maxSlices {
				n = maxSlices
			}
			if n <= 0 {
				n = 1
			}
			state := "available"
			if res.State != pool.StateAvailable {
				state = string(res.State)
			}
			for i := 0; i < n; i++ {
				slots = append(slots, PoolSlot{
					SlotID:     fmt.Sprintf("%s-slot-%d", res.ID, i),
					ResourceID: res.ID,
					NodeID:     res.NodeID,
					MemoryGB:   memPerSlot,
					Cores:      coresPerSlot,
					State:      state,
				})
			}
		}
	} else {
		// 无切分配置时，每个可用资源视为 1 个“整卡槽位”
		for _, res := range resources {
			state := "available"
			if res.State != pool.StateAvailable {
				state = string(res.State)
			}
			slots = append(slots, PoolSlot{
				SlotID:     res.ID,
				ResourceID: res.ID,
				NodeID:     res.NodeID,
				MemoryGB:   res.TotalMemory,
				Cores:      res.TotalCores,
				State:      state,
			})
		}
	}
	respondJSON(w, http.StatusOK, slots)
}

// deployModelOnPool 在池化/切分资源上部署大模型：先分配再绑定部署记录
func (s *APIServer) deployModelOnPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolID := vars["poolId"]
	var req struct {
		ModelID     string                 `json:"model_id"`
		ResourceReq *pool.AllocationRequest `json:"resource_req"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if req.ModelID == "" {
		respondError(w, http.StatusBadRequest, fmt.Errorf("model_id required"))
		return
	}
	if req.ResourceReq == nil {
		req.ResourceReq = &pool.AllocationRequest{
			TaskID:       "deploy-" + req.ModelID + "-" + fmt.Sprintf("%d", time.Now().UnixNano()),
			TaskType:     "inference",
			ResourceType: pool.ResourceTypeGPU,
			Count:        1,
			MemoryGB:     24,
			Cores:        100,
			Priority:     5,
		}
	}
	if req.ResourceReq.TaskID == "" {
		req.ResourceReq.TaskID = "deploy-" + req.ModelID + "-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	ctx := r.Context()
	result, err := s.resourcePool.AllocateFromPool(ctx, poolID, req.ResourceReq)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	s.deploymentMu.Lock()
	deployID := "deploy-" + fmt.Sprintf("%d", time.Now().UnixNano())
	s.deploymentStore[deployID] = &DeploymentRecord{
		ID:           deployID,
		ModelID:      req.ModelID,
		AllocationID: result.AllocationID,
		PoolID:       poolID,
		Status:       "running",
		CreatedAt:    time.Now(),
	}
	s.deploymentMu.Unlock()
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"deployment_id":  deployID,
		"allocation_id":  result.AllocationID,
		"model_id":       req.ModelID,
		"pool_id":        poolID,
		"resources":      result.Resources,
		"status":         "running",
	})
}

func (s *APIServer) listDeployments(w http.ResponseWriter, r *http.Request) {
	poolID := r.URL.Query().Get("pool_id")
	s.deploymentMu.RLock()
	defer s.deploymentMu.RUnlock()
	var list []*DeploymentRecord
	for _, d := range s.deploymentStore {
		if poolID == "" || d.PoolID == poolID {
			list = append(list, d)
		}
	}
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) listFlavors(w http.ResponseWriter, r *http.Request) {
	// 获取所有已注册的规格
	flavors := s.resourcePool.ListFlavors()
	respondJSON(w, http.StatusOK, flavors)
}

// --- 集群大池 API ---

func (s *APIServer) listClusterPools(w http.ResponseWriter, r *http.Request) {
	if s.clusterPoolMgr == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	list := s.clusterPoolMgr.List()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) createClusterPool(w http.ResponseWriter, r *http.Request) {
	if s.clusterPoolMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster pool not configured"))
		return
	}
	var spec pool.ClusterPoolSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if spec.ID == "" {
		spec.ID = "cluster-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if err := s.clusterPoolMgr.Create(&spec); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusCreated, spec)
}

func (s *APIServer) getClusterPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["clusterPoolId"]
	if s.clusterPoolMgr == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("cluster pool not found: %s", id))
		return
	}
	spec, err := s.clusterPoolMgr.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, spec)
}

func (s *APIServer) updateClusterPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["clusterPoolId"]
	if s.clusterPoolMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster pool not configured"))
		return
	}
	var spec pool.ClusterPoolSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	spec.ID = id
	if err := s.clusterPoolMgr.Update(&spec); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, spec)
}

func (s *APIServer) deleteClusterPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["clusterPoolId"]
	if s.clusterPoolMgr == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("cluster pool not found: %s", id))
		return
	}
	if err := s.clusterPoolMgr.Delete(id); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (s *APIServer) getClusterPoolInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["clusterPoolId"]
	if s.clusterPoolMgr == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("cluster pool not found: %s", id))
		return
	}
	spec, err := s.clusterPoolMgr.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	info, err := s.clusterPoolMgr.GetPoolInfoFromUnified(r.Context(), spec, s.resourcePool)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, info)
}

func (s *APIServer) allocateFromClusterPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterPoolID := vars["clusterPoolId"]
	if s.clusterPoolMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster pool not configured"))
		return
	}
	spec, err := s.clusterPoolMgr.Get(clusterPoolID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	var req pool.AllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	req.ConstrainNodeIDs = spec.NodeIDs
	if len(spec.PoolIDs) > 0 {
		req.ConstrainPoolIDs = spec.PoolIDs
	}
	ctx := r.Context()
	result, err := s.resourcePool.AllocateAny(ctx, &req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (s *APIServer) listClusterPoolResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterPoolID := vars["clusterPoolId"]
	if s.clusterPoolMgr == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	spec, err := s.clusterPoolMgr.Get(clusterPoolID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	nodeSet := make(map[string]bool)
	for _, n := range spec.NodeIDs {
		nodeSet[n] = true
	}
	filters := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			filters[k] = v[0]
		}
	}
	all, err := s.resourcePool.ListResources(r.Context(), filters)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	var out []*pool.Resource
	for _, r := range all {
		if len(nodeSet) > 0 && !nodeSet[r.NodeID] {
			continue
		}
		out = append(out, r)
	}
	respondJSON(w, http.StatusOK, out)
}

// --- 子池 API ---

func (s *APIServer) listSubPools(w http.ResponseWriter, r *http.Request) {
	if s.subPoolMgr == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	clusterID := r.URL.Query().Get("cluster_pool_id")
	parentID := r.URL.Query().Get("parent_pool_id")
	var list []*pool.SubPoolSpec
	if clusterID != "" || parentID != "" {
		list = s.subPoolMgr.ListByParent(clusterID, parentID)
	} else {
		list = s.subPoolMgr.List()
	}
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) createSubPool(w http.ResponseWriter, r *http.Request) {
	if s.subPoolMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("sub pool not configured"))
		return
	}
	var spec pool.SubPoolSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if spec.ID == "" {
		spec.ID = "sub-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if err := s.subPoolMgr.Create(&spec); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusCreated, spec)
}

func (s *APIServer) getSubPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["subPoolId"]
	if s.subPoolMgr == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("sub pool not found: %s", id))
		return
	}
	spec, err := s.subPoolMgr.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, spec)
}

func (s *APIServer) deleteSubPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["subPoolId"]
	if s.subPoolMgr == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("sub pool not found: %s", id))
		return
	}
	if err := s.subPoolMgr.Delete(id); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (s *APIServer) getSubPoolUsage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["subPoolId"]
	if s.subPoolMgr == nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("sub pool not found: %s", id))
		return
	}
	usage, err := s.subPoolMgr.GetUsage(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, usage)
}

func (s *APIServer) allocateFromSubPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	subPoolID := vars["subPoolId"]
	if s.subPoolMgr == nil || s.clusterPoolMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("sub pool not configured"))
		return
	}
	subSpec, err := s.subPoolMgr.Get(subPoolID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	var req pool.AllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	memGB := req.MemoryGB * int64(req.Count)
	if memGB <= 0 {
		memGB = req.MemoryGB
	}
	ok, err := s.subPoolMgr.CheckQuota(subPoolID, req.Count, memGB)
	if err != nil || !ok {
		respondError(w, http.StatusBadRequest, fmt.Errorf("quota exceeded or invalid: %v", err))
		return
	}
	ctx := r.Context()
	var result *pool.AllocationResult
	if subSpec.ParentClusterPoolID != "" {
		clusterSpec, err := s.clusterPoolMgr.Get(subSpec.ParentClusterPoolID)
		if err != nil {
			respondError(w, http.StatusBadRequest, err)
			return
		}
		req.ConstrainNodeIDs = clusterSpec.NodeIDs
		req.ConstrainPoolIDs = clusterSpec.PoolIDs
		result, err = s.resourcePool.AllocateAny(ctx, &req)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		result, err = s.resourcePool.AllocateFromPool(ctx, subSpec.ParentPoolID, &req)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
	}
	_ = s.subPoolMgr.AddUsage(subPoolID, req.Count, memGB)
	respondJSON(w, http.StatusOK, result)
}

func (s *APIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requirePermission(w, r, rbac.PermissionViewTask); !ok {
		return
	}
	ctx := r.Context()
	
	// 解析查询参数作为过滤器
	filters := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			filters[k] = v[0]
		}
	}
	
	tasks, err := s.scheduler.ListTasks(ctx, filters)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, tasks)
}

func (s *APIServer) submitTask(w http.ResponseWriter, r *http.Request) {
	var req SubmitTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	_, tenantID, ok := s.requirePermission(w, r, rbac.PermissionSubmitTask)
	if !ok {
		return
	}
	
	if req.Labels == nil {
		req.Labels = make(map[string]string)
	}
	task := &scheduler.Task{
		Name:                    req.Name,
		Type:                    req.Type,
		Priority:                req.Priority,
		ResourceReq:             req.ResourceReq,
		InferenceBackend:        req.InferenceBackend,
		ProjectID:               req.ProjectID,
		ModelID:                 req.ModelID,
		Queue:                   req.Queue,
		GangGroup:               req.GangGroup,
		MinMember:               req.MinMember,
		MinCount:                req.MinCount,
		NodeSelector:            req.NodeSelector,
		NonPreemptible:          req.NonPreemptible,
		ExpectedDurationSeconds: req.ExpectedDurationSeconds,
		TenantID:                tenantID,
		Labels:                  req.Labels,
	}
	if task.ProjectID == "" {
		task.ProjectID = req.Labels["project_id"]
	}
	if task.ModelID == "" {
		task.ModelID = req.Labels["model_id"]
	}
	if task.GangGroup == "" {
		task.GangGroup = req.Labels["gang_group"]
	}
	if task.InferenceBackend == "" && req.Labels["inference_backend"] != "" {
		task.InferenceBackend = req.Labels["inference_backend"]
	}
	if task.ResourceReq != nil {
		task.ResourceReq.TenantID = tenantID
	}
	
	ctx := r.Context()
	// 如果存在租户，按任务和资源请求做一次简单配额检查（示例实现）
	if tenantID != "" {
		q := &tenant.QuotaRequest{
			Tasks: 1,
		}
		if task.ResourceReq != nil {
			if task.ResourceReq.ResourceType == pool.ResourceTypeGPU {
				q.GPUs = task.ResourceReq.Count
			} else if task.ResourceReq.ResourceType == pool.ResourceTypeNPU {
				q.NPUs = task.ResourceReq.Count
			}
			q.MemoryGB = task.ResourceReq.MemoryGB * int64(task.ResourceReq.Count)
		}
		ok, err := s.tenantMgr.CheckQuota(ctx, tenantID, q)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err)
			return
		}
		if !ok {
			respondError(w, http.StatusForbidden, fmt.Errorf("tenant quota exceeded"))
			return
		}
	}
	
	if err := s.scheduler.SubmitTask(ctx, task); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, task)
}

func (s *APIServer) getTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	
	ctx := r.Context()
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	
	respondJSON(w, http.StatusOK, task)
}

func (s *APIServer) cancelTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	
	if _, _, ok := s.requirePermission(w, r, rbac.PermissionCancelTask); !ok {
		return
	}
	
	ctx := r.Context()
	if err := s.scheduler.CancelTask(ctx, taskID); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Task cancelled successfully",
	})
}

// --- 节点管理处理函数 ---

func (s *APIServer) listNodes(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	
	// 查询参数作为过滤器
	filters := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			filters[k] = v[0]
		}
	}
	
	nodes, err := s.clusterMgr.ListNodes(filters)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, nodes)
}

func (s *APIServer) getNode(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	
	node, err := s.clusterMgr.GetNode(nodeID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	
	respondJSON(w, http.StatusOK, node)
}

func (s *APIServer) discoverNodeResources(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	
	vars := mux.Vars(r)
	nodeID := vars["nodeId"]
	
	res, err := s.clusterMgr.DiscoverResources(nodeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, res)
}

// --- 模型市场处理函数 ---

func (s *APIServer) listModels(w http.ResponseWriter, r *http.Request) {
	models := s.modelStore.ListModels()
	respondJSON(w, http.StatusOK, models)
}

func (s *APIServer) getModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["modelId"]
	
	m, err := s.modelStore.GetModel(modelID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, m)
}

func (s *APIServer) createModel(w http.ResponseWriter, r *http.Request) {
	var req CreateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if req.ID == "" {
		req.ID = fmt.Sprintf("model-%d", time.Now().UnixNano())
	}
	
	m := &models.Model{
		ID:               req.ID,
		Name:             req.Name,
		Version:          req.Version,
		Type:             req.Type,
		Framework:        req.Framework,
		Format:           req.Format,
		ResourceFlavorID: req.ResourceFlavorID,
		Path:             req.Path,
		Description:      req.Description,
		ContextLength:    req.ContextLength,
		Size:             req.Size,
		Family:           req.Family,
		Languages:        req.Languages,
		Abilities:        req.Abilities,
		ProjectID:        req.ProjectID,
		InferenceBackend: req.InferenceBackend,
		NIMModelID:       req.NIMModelID,
		Labels:           req.Labels,
	}
	if err := s.modelStore.CreateModel(m); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, m)
}

func (s *APIServer) deleteModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["modelId"]
	if err := s.modelStore.DeleteModel(modelID); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Model deleted successfully",
	})
}

// --- NVIDIA NIM 配置处理函数 ---

func (s *APIServer) getNIMConfig(w http.ResponseWriter, r *http.Request) {
	if s.nimStore == nil {
		respondJSON(w, http.StatusOK, nim.Config{})
		return
	}
	cfg := s.nimStore.GetConfig()
	respondJSON(w, http.StatusOK, cfg)
}

func (s *APIServer) updateNIMConfig(w http.ResponseWriter, r *http.Request) {
	if s.nimStore == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("NIM config not configured"))
		return
	}
	var cfg nim.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := s.nimStore.UpdateConfig(&cfg); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	updated := s.nimStore.GetConfig()
	respondJSON(w, http.StatusOK, updated)
}

// --- 项目管理处理函数 ---

func (s *APIServer) listProjects(w http.ResponseWriter, r *http.Request) {
	projects := s.projectStore.ListProjects()
	respondJSON(w, http.StatusOK, projects)
}

func (s *APIServer) getProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	
	p, err := s.projectStore.GetProject(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, p)
}

func (s *APIServer) createProject(w http.ResponseWriter, r *http.Request) {
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if req.ID == "" {
		req.ID = fmt.Sprintf("project-%d", time.Now().UnixNano())
	}
	
	p := &project.Project{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		TenantID:    req.TenantID,
		OwnerID:     req.OwnerID,
		Labels:      req.Labels,
	}
	if err := s.projectStore.CreateProject(p); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, p)
}

func (s *APIServer) updateProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	p := &project.Project{
		ID:          projectID,
		Name:        req.Name,
		Description: req.Description,
		TenantID:    req.TenantID,
		OwnerID:     req.OwnerID,
		Labels:      req.Labels,
	}
	if err := s.projectStore.UpdateProject(p); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	
	respondJSON(w, http.StatusOK, p)
}

func (s *APIServer) deleteProject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectID := vars["projectId"]
	if err := s.projectStore.DeleteProject(projectID); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Project deleted successfully",
	})
}

// --- 算力感知 API 处理函数 ---

func (s *APIServer) listResourceStates(w http.ResponseWriter, r *http.Request) {
	if s.computePerception == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute perception not configured"))
		return
	}
	
	filters := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			filters[k] = v[0]
		}
	}
	
	states := s.computePerception.ListResourceStates(filters)
	respondJSON(w, http.StatusOK, states)
}

func (s *APIServer) getResourceState(w http.ResponseWriter, r *http.Request) {
	if s.computePerception == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute perception not configured"))
		return
	}
	
	vars := mux.Vars(r)
	resourceID := vars["resourceId"]
	
	state, err := s.computePerception.GetResourceState(resourceID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	
	respondJSON(w, http.StatusOK, state)
}

func (s *APIServer) bindResource(w http.ResponseWriter, r *http.Request) {
	if s.computePerception == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute perception not configured"))
		return
	}
	
	var req struct {
		ResourceID string `json:"resource_id"`
		TaskID     string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	ctx := r.Context()
	if err := s.computePerception.BindResource(ctx, req.ResourceID, req.TaskID); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{"message": "Resource bound successfully"})
}

func (s *APIServer) unbindResource(w http.ResponseWriter, r *http.Request) {
	if s.computePerception == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute perception not configured"))
		return
	}
	
	var req struct {
		ResourceID string `json:"resource_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	ctx := r.Context()
	if err := s.computePerception.UnbindResource(ctx, req.ResourceID); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{"message": "Resource unbound successfully"})
}

func (s *APIServer) getClusterSettings(w http.ResponseWriter, r *http.Request) {
	if s.computePerception == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute perception not configured"))
		return
	}
	
	settings := s.computePerception.GetClusterSettings()
	respondJSON(w, http.StatusOK, settings)
}

func (s *APIServer) updateClusterSettings(w http.ResponseWriter, r *http.Request) {
	if s.computePerception == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute perception not configured"))
		return
	}
	
	var settings compute.ClusterSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.computePerception.UpdateClusterSettings(&settings); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusOK, settings)
}

// --- 算力质量 API 处理函数 ---

func (s *APIServer) listSlicingConfigs(w http.ResponseWriter, r *http.Request) {
	if s.computeQuality == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute quality not configured"))
		return
	}
	
	configs := s.computeQuality.ListSlicingConfigs()
	respondJSON(w, http.StatusOK, configs)
}

func (s *APIServer) createSlicingConfig(w http.ResponseWriter, r *http.Request) {
	if s.computeQuality == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute quality not configured"))
		return
	}
	
	var config compute.SlicingConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.computeQuality.RegisterSlicingConfig(&config); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, config)
}

func (s *APIServer) listExclusiveConfigs(w http.ResponseWriter, r *http.Request) {
	if s.computeQuality == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute quality not configured"))
		return
	}
	list := s.computeQuality.ListExclusiveConfigs()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) createExclusiveConfig(w http.ResponseWriter, r *http.Request) {
	if s.computeQuality == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute quality not configured"))
		return
	}
	
	var config compute.ExclusiveConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.computeQuality.RegisterExclusiveConfig(&config); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, config)
}

func (s *APIServer) listAllocationStrategies(w http.ResponseWriter, r *http.Request) {
	if s.computeQuality == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute quality not configured"))
		return
	}
	list := s.computeQuality.ListAllocationStrategies()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) createAllocationStrategy(w http.ResponseWriter, r *http.Request) {
	if s.computeQuality == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute quality not configured"))
		return
	}
	
	var strategy compute.AllocationStrategy
	if err := json.NewDecoder(r.Body).Decode(&strategy); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.computeQuality.RegisterAllocationStrategy(&strategy); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, strategy)
}

// --- 算力编排 API 处理函数 ---

func (s *APIServer) distributeTask(w http.ResponseWriter, r *http.Request) {
	if s.computeOrchestration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute orchestration not configured"))
		return
	}
	
	var req struct {
		Task    *scheduler.Task `json:"task"`
		Strategy string         `json:"strategy"` // balanced/min_fragment/elastic
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	ctx := r.Context()
	if err := s.computeOrchestration.DistributeTask(ctx, req.Task, req.Strategy); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, req.Task)
}

func (s *APIServer) optimizeScheduling(w http.ResponseWriter, r *http.Request) {
	if s.computeOrchestration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute orchestration not configured"))
		return
	}
	
	ctx := r.Context()
	if err := s.computeOrchestration.OptimizeScheduling(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{"message": "Scheduling optimized"})
}

func (s *APIServer) getOrchestrationStrategies(w http.ResponseWriter, r *http.Request) {
	if s.computeOrchestration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("compute orchestration not configured"))
		return
	}
	
	strategies := map[string]interface{}{
		"balanced":     s.computeOrchestration.GetBalancedStrategy(),
		"min_fragment": s.computeOrchestration.GetMinFragmentStrategy(),
		"elastic":      s.computeOrchestration.GetElasticStrategy(),
	}
	
	respondJSON(w, http.StatusOK, strategies)
}

// --- 训练加速 API 处理函数 ---

func (s *APIServer) listDataLoadingConfigs(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	list := s.trainingAcceleration.ListDataLoadingConfigs()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) listDistributedConfigs(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	list := s.trainingAcceleration.ListDistributedConfigs()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) listCheckpointConfigs(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	list := s.trainingAcceleration.ListCheckpointConfigs()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) listMemoryOptimizationConfigs(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	list := s.trainingAcceleration.ListMemoryOptimizationConfigs()
	respondJSON(w, http.StatusOK, list)
}

func (s *APIServer) configureDataLoading(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("training acceleration not configured"))
		return
	}
	
	var config compute.DataLoadingConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.trainingAcceleration.ConfigureDataLoading(&config); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, config)
}

func (s *APIServer) configureDistributedTraining(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("training acceleration not configured"))
		return
	}
	
	var config compute.DistributedTrainingConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.trainingAcceleration.ConfigureDistributedTraining(&config); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, config)
}

func (s *APIServer) configureCheckpoint(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("training acceleration not configured"))
		return
	}
	
	var config compute.CheckpointConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.trainingAcceleration.ConfigureCheckpoint(&config); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, config)
}

func (s *APIServer) configureMemoryOptimization(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("training acceleration not configured"))
		return
	}
	
	var config compute.MemoryOptimizationConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	
	if err := s.trainingAcceleration.ConfigureMemoryOptimization(&config); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	
	respondJSON(w, http.StatusCreated, config)
}

func (s *APIServer) optimizeTask(w http.ResponseWriter, r *http.Request) {
	if s.trainingAcceleration == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("training acceleration not configured"))
		return
	}
	
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	
	ctx := r.Context()
	if err := s.trainingAcceleration.OptimizeTask(ctx, taskID); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]string{"message": "Task optimized successfully"})
}

func (s *APIServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

func (s *APIServer) readyCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

// 辅助函数

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

// --- RBAC / Tenant 辅助 ---

// getUserContext 从请求头中获取 userID / tenantID。
// 为简化示例，如果未提供，则视为匿名请求且不强制校验。
func (s *APIServer) getUserContext(r *http.Request) (string, string) {
	userID := r.Header.Get("X-User-Id")
	tenantID := r.Header.Get("X-Tenant-Id")
	return userID, tenantID
}

// requirePermission 检查用户是否具有指定权限。
// 如果未提供 userID，则当前版本默认放行（便于无认证环境下体验）。
func (s *APIServer) requirePermission(w http.ResponseWriter, r *http.Request, perm rbac.Permission) (string, string, bool) {
	userID, tenantID := s.getUserContext(r)
	if userID == "" {
		return userID, tenantID, true
	}
	ok, err := s.rbacMgr.CheckPermission(r.Context(), userID, perm)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return "", "", false
	}
	if !ok {
		respondError(w, http.StatusForbidden, fmt.Errorf("permission denied"))
		return "", "", false
	}
	return userID, tenantID, true
}
