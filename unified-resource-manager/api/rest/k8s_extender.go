/*
 * Unified Resource Manager - Kubernetes Scheduler Extender
 *
 * 实现 kube-scheduler HTTP Extender 协议（filter / prioritize / bind），
 * 使集群可把 GPU/NPU 拓扑、切分方式、Gang 与租户隔离交给本调度器决策。
 *
 * kube-scheduler 配置示例见 deploy/k8s/scheduler-extender-config.yaml
 *
 * Pod 注解（优先）/ 资源请求：
 *   urm.flexai.io/task-type      inference|training
 *   urm.flexai.io/count          卡数
 *   urm.flexai.io/memory-gb      每卡显存
 *   urm.flexai.io/cores          每卡算力占比 0-100
 *   urm.flexai.io/share-mode     exclusive|shared|mig|timeslice
 *   urm.flexai.io/mig-profile    1g.10gb 等
 *   urm.flexai.io/gang-group     Gang 组名
 *   urm.flexai.io/queue          调度队列
 *   nvidia.com/gpu 或 huawei.com/npu 作为卡数回退
 */

package rest

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/core/scheduler"
)

const (
	annTaskType  = "urm.flexai.io/task-type"
	annCount     = "urm.flexai.io/count"
	annMemory    = "urm.flexai.io/memory-gb"
	annCores     = "urm.flexai.io/cores"
	annShareMode = "urm.flexai.io/share-mode"
	annMIG       = "urm.flexai.io/mig-profile"
	annGang      = "urm.flexai.io/gang-group"
	annQueue     = "urm.flexai.io/queue"
	annTenant    = "urm.flexai.io/tenant"
)

// ExtenderArgs kube-scheduler HTTP 扩展器入参（兼容 NodeNames / Nodes 两种形态）
type ExtenderArgs struct {
	Pod       json.RawMessage   `json:"Pod"`
	NodeNames *[]string         `json:"nodeNames,omitempty"`
	Nodes     *extenderNodeList `json:"nodes,omitempty"`
}

type extenderNodeList struct {
	Items []extenderNode `json:"items"`
}

type extenderNode struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
}

// ExtenderFilterResult filter 出参
type ExtenderFilterResult struct {
	NodeNames   *[]string         `json:"NodeNames,omitempty"`
	Nodes       *extenderNodeList `json:"Nodes,omitempty"`
	FailedNodes map[string]string `json:"FailedNodes,omitempty"`
	Error       string            `json:"Error,omitempty"`
}

// HostPriority 单节点打分
type HostPriority struct {
	Host  string `json:"host"`
	Score int64  `json:"score"`
}

type podView struct {
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Annotations map[string]string `json:"annotations"`
		Labels      map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		NodeSelector map[string]string `json:"nodeSelector"`
		Containers   []struct {
			Resources struct {
				Requests map[string]string `json:"requests"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"spec"`
}

func (s *APIServer) k8sExtenderFilter(w http.ResponseWriter, r *http.Request) {
	args, names, err := decodeExtenderArgs(r)
	if err != nil {
		respondJSON(w, http.StatusOK, ExtenderFilterResult{Error: err.Error()})
		return
	}
	intel := s.intel()
	if intel == nil {
		respondJSON(w, http.StatusOK, keepAll(args, names))
		return
	}
	task := taskFromPod(args.Pod)
	evals := intel.EvaluateNodes(task, names)
	failed := map[string]string{}
	var feasible []string
	for _, e := range evals {
		if e.Feasible {
			feasible = append(feasible, e.Node)
		} else {
			failed[e.Node] = e.Reason
		}
	}
	log.Printf("[k8s-extender] filter pod=%s feasible=%d/%d", podName(args.Pod), len(feasible), len(names))
	respondJSON(w, http.StatusOK, ExtenderFilterResult{NodeNames: &feasible, FailedNodes: failed})
}

func (s *APIServer) k8sExtenderPrioritize(w http.ResponseWriter, r *http.Request) {
	args, names, err := decodeExtenderArgs(r)
	if err != nil {
		respondJSON(w, http.StatusOK, []HostPriority{})
		return
	}
	intel := s.intel()
	if intel == nil {
		out := make([]HostPriority, 0, len(names))
		for _, n := range names {
			out = append(out, HostPriority{Host: n, Score: 1})
		}
		respondJSON(w, http.StatusOK, out)
		return
	}
	evals := intel.EvaluateNodes(taskFromPod(args.Pod), names)
	byNode := map[string]int64{}
	for _, e := range evals {
		byNode[e.Node] = e.Score
	}
	out := make([]HostPriority, 0, len(names))
	for _, n := range names {
		out = append(out, HostPriority{Host: n, Score: byNode[n]})
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *APIServer) k8sExtenderBind(w http.ResponseWriter, r *http.Request) {
	// Bind 仍由 kube-scheduler 完成；本扩展器只做 filter/score。
	// 返回成功空结果，避免 extender 配置 bindVerb 时出错。
	respondJSON(w, http.StatusOK, map[string]string{})
}

func decodeExtenderArgs(r *http.Request) (*ExtenderArgs, []string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, nil, err
	}
	var args ExtenderArgs
	if err := json.Unmarshal(body, &args); err != nil {
		return nil, nil, err
	}
	var names []string
	if args.NodeNames != nil {
		names = *args.NodeNames
	}
	if args.Nodes != nil {
		for _, n := range args.Nodes.Items {
			names = append(names, n.Metadata.Name)
		}
	}
	return &args, names, nil
}

func keepAll(args *ExtenderArgs, names []string) ExtenderFilterResult {
	cp := append([]string(nil), names...)
	return ExtenderFilterResult{NodeNames: &cp, Nodes: args.Nodes}
}

func podName(raw json.RawMessage) string {
	var p podView
	_ = json.Unmarshal(raw, &p)
	return p.Metadata.Namespace + "/" + p.Metadata.Name
}

func taskFromPod(raw json.RawMessage) *scheduler.Task {
	var p podView
	_ = json.Unmarshal(raw, &p)
	ann := p.Metadata.Annotations
	if ann == nil {
		ann = map[string]string{}
	}
	taskType := scheduler.TaskType(ann[annTaskType])
	if taskType == "" {
		taskType = scheduler.TaskTypeInference
	}
	count := atoiDefault(ann[annCount], resourceCount(p, "nvidia.com/gpu", "huawei.com/npu", "huawei.com/vgpu-number"))
	if count <= 0 {
		count = 1
	}
	mem := int64(atoiDefault(ann[annMemory], 0))
	cores := atoiDefault(ann[annCores], 0)
	rtype := pool.ResourceTypeGPU
	if resourceCount(p, "huawei.com/npu") > 0 {
		rtype = pool.ResourceTypeNPU
	}
	return &scheduler.Task{
		Name:         p.Metadata.Name,
		Type:         taskType,
		Queue:        ann[annQueue],
		GangGroup:    ann[annGang],
		TenantID:     firstNonEmpty(ann[annTenant], p.Metadata.Namespace),
		NodeSelector: p.Spec.NodeSelector,
		Labels:       p.Metadata.Labels,
		ResourceReq: &pool.AllocationRequest{
			TaskType:     string(taskType),
			ResourceType: rtype,
			Count:        count,
			MemoryGB:     mem,
			Cores:        cores,
			ShareMode:    pool.ShareMode(ann[annShareMode]),
			MIGProfile:   ann[annMIG],
			TenantID:     firstNonEmpty(ann[annTenant], p.Metadata.Namespace),
		},
	}
}

func resourceCount(p podView, keys ...string) int {
	total := 0
	for _, c := range p.Spec.Containers {
		for _, k := range keys {
			if v, ok := c.Resources.Requests[k]; ok {
				total += atoiDefault(strings.TrimSuffix(v, "k"), 0)
			}
		}
	}
	return total
}

func atoiDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
