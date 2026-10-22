/*
 * Unified Resource Manager - Scheduler / Queue / Defrag APIs
 */

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"flexai/unified-resource-manager/core/cluster"
	"flexai/unified-resource-manager/core/scheduler"
)

func (s *APIServer) requireIntel(w http.ResponseWriter) *scheduler.IntelligentScheduler {
	intel := s.intel()
	if intel == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("intelligent scheduler not configured"))
		return nil
	}
	return intel
}

func (s *APIServer) getSchedulerStats(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	respondJSON(w, http.StatusOK, intel.GetStats())
}

func (s *APIServer) getSchedulerConfig(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	respondJSON(w, http.StatusOK, intel.GetConfig())
}

func (s *APIServer) updateSchedulerConfig(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	var cfg scheduler.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := intel.UpdateConfig(cfg); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, intel.GetConfig())
}

type evaluateRequest struct {
	Task  *scheduler.Task `json:"task"`
	Nodes []string        `json:"nodes,omitempty"`
}

func (s *APIServer) evaluatePlacement(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	var req evaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Task == nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("task is required"))
		return
	}
	respondJSON(w, http.StatusOK, intel.EvaluateNodes(req.Task, req.Nodes))
}

func (s *APIServer) listQueues(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	respondJSON(w, http.StatusOK, intel.Queues().List())
}

func (s *APIServer) getQueue(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	q, ok := intel.Queues().Get(mux.Vars(r)["name"])
	if !ok {
		respondError(w, http.StatusNotFound, fmt.Errorf("queue not found"))
		return
	}
	respondJSON(w, http.StatusOK, q)
}

func (s *APIServer) upsertQueue(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	var q scheduler.Queue
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := intel.Queues().Upsert(&q); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	saved, _ := intel.Queues().Get(q.Name)
	respondJSON(w, http.StatusOK, saved)
}

func (s *APIServer) deleteQueue(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	if err := intel.Queues().Delete(mux.Vars(r)["name"]); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "queue deleted"})
}

func (s *APIServer) getFragmentation(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	respondJSON(w, http.StatusOK, intel.AnalyzeFragmentation())
}

func (s *APIServer) planDefrag(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	respondJSON(w, http.StatusOK, intel.PlanDefragmentation())
}

func (s *APIServer) executeDefrag(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	plan, err := intel.ExecuteDefragmentation()
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"plan": plan, "error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, plan)
}

type completeTaskRequest struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func (s *APIServer) completeTask(w http.ResponseWriter, r *http.Request) {
	intel := s.requireIntel(w)
	if intel == nil {
		return
	}
	var req completeTaskRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := intel.CompleteTask(r.Context(), mux.Vars(r)["taskId"], req.Success, req.Message); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "task completed"})
}

func (s *APIServer) registerNode(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	var node cluster.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := s.clusterMgr.RegisterNode(&node); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	saved, _ := s.clusterMgr.GetNode(node.ID)
	respondJSON(w, http.StatusCreated, saved)
}

func (s *APIServer) nodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	if err := s.clusterMgr.Heartbeat(mux.Vars(r)["nodeId"]); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

type cordonRequest struct {
	Cordoned bool `json:"cordoned"`
}

func (s *APIServer) cordonNode(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	var req cordonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := s.clusterMgr.SetCordon(mux.Vars(r)["nodeId"], req.Cordoned); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"node_id": mux.Vars(r)["nodeId"], "cordoned": req.Cordoned})
}

func (s *APIServer) drainNode(w http.ResponseWriter, r *http.Request) {
	if s.clusterMgr == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("cluster manager not configured"))
		return
	}
	nodeID := mux.Vars(r)["nodeId"]
	if err := s.clusterMgr.SetCordon(nodeID, true); err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	var moved []string
	if intel := s.intel(); intel != nil {
		moved = intel.DrainNode(nodeID)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"node_id": nodeID, "cordoned": true, "migrated_tasks": moved})
}
