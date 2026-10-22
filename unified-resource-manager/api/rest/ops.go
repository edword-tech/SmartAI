/*
 * Unified Resource Manager - Ops APIs (telemetry / self-heal / benchmark / diagnosis)
 */

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"flexai/unified-resource-manager/core/pool"
	"flexai/unified-resource-manager/ops/benchmark"
	"flexai/unified-resource-manager/ops/diagnosis"
	"flexai/unified-resource-manager/ops/probe"
	"flexai/unified-resource-manager/ops/remediation"
	"flexai/unified-resource-manager/ops/telemetry"
)

func (s *APIServer) requireOps(w http.ResponseWriter) *OpsRuntime {
	if s.ops == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("ops runtime not configured"))
		return nil
	}
	return s.ops
}

func (s *APIServer) opsOverview(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	out := map[string]interface{}{
		"telemetry_source": ops.Hub.SourceName(),
		"self_heal":        ops.Engine.Stats(),
		"conditions":       ops.Engine.Conditions(),
		"llm_enabled":      ops.Diagnoser.LLMEnabled(),
		"llm_model":        ops.Diagnoser.LLMModel(),
	}
	if err := ops.Hub.LastError(); err != nil {
		out["telemetry_error"] = err.Error()
	}
	if intel := s.intel(); intel != nil {
		out["scheduler"] = intel.GetStats()
		out["fragmentation"] = intel.AnalyzeFragmentation()
	}
	if ops.Probe != nil {
		out["readiness"] = ops.Probe.Readiness(r.Context())
		out["last_probe"] = ops.Probe.Last()
	}
	if s.clusterMgr != nil {
		stats, _ := s.clusterMgr.GetClusterStats()
		out["cluster"] = stats
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *APIServer) listTelemetry(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	samples := ops.Hub.Latest()
	if nodeID := r.URL.Query().Get("node_id"); nodeID != "" {
		filtered := make([]*telemetry.DeviceTelemetry, 0, len(samples))
		for _, t := range samples {
			if t.NodeID == nodeID {
				filtered = append(filtered, t)
			}
		}
		samples = filtered
	}
	respondJSON(w, http.StatusOK, samples)
}

func (s *APIServer) getTelemetry(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	id := mux.Vars(r)["resourceId"]
	latest, ok := ops.Hub.Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, fmt.Errorf("no telemetry for %s", id))
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"latest":  latest,
		"history": ops.Hub.History(id),
	})
}

func (s *APIServer) listOpsEvents(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	var since time.Time
	if raw := q.Get("since"); raw != "" {
		since, _ = time.Parse(time.RFC3339, raw)
	}
	respondJSON(w, http.StatusOK, ops.Engine.Events(remediation.EventFilter{
		Level:      remediation.Level(q.Get("level")),
		Category:   q.Get("category"),
		NodeID:     q.Get("node_id"),
		ResourceID: q.Get("resource_id"),
		Since:      since,
		Limit:      limit,
	}))
}

func (s *APIServer) listOpsRules(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	respondJSON(w, http.StatusOK, ops.Engine.Rules())
}

func (s *APIServer) updateOpsRule(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	var u remediation.RuleUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	rule, err := ops.Engine.UpdateRule(mux.Vars(r)["ruleId"], u)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, rule)
}

func (s *APIServer) listOpsConditions(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	respondJSON(w, http.StatusOK, ops.Engine.Conditions())
}

type resetDeviceRequest struct {
	Operator string `json:"operator,omitempty"`
}

func (s *APIServer) resetDevice(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	var req resetDeviceRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	id := mux.Vars(r)["resourceId"]
	if ops.Sim != nil {
		ops.Sim.ClearFault(id)
	}
	if err := ops.Engine.ResetDevice(id, req.Operator); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "device reset acknowledged", "resource_id": id})
}

func (s *APIServer) injectFault(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil || ops.Sim == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("fault injection requires simulated telemetry"))
		return
	}
	var f telemetry.Fault
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	id := mux.Vars(r)["resourceId"]
	ops.Sim.InjectFault(id, f)
	ops.Hub.CollectOnce(r.Context())
	respondJSON(w, http.StatusOK, map[string]string{"message": "fault injected", "resource_id": id})
}

func (s *APIServer) clearFault(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil || ops.Sim == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("fault injection requires simulated telemetry"))
		return
	}
	id := mux.Vars(r)["resourceId"]
	ops.Sim.ClearFault(id)
	respondJSON(w, http.StatusOK, map[string]string{"message": "fault cleared", "resource_id": id})
}

func (s *APIServer) listBaselines(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	respondJSON(w, http.StatusOK, ops.Benchmark.Baselines())
}

func (s *APIServer) setBaseline(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	var b benchmark.Baseline
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := ops.Benchmark.SetBaseline(b); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, b)
}

func (s *APIServer) listBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	respondJSON(w, http.StatusOK, ops.Benchmark.Runs(r.URL.Query().Get("node_id")))
}

func (s *APIServer) getBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	run, ok := ops.Benchmark.GetRun(mux.Vars(r)["runId"])
	if !ok {
		respondError(w, http.StatusNotFound, fmt.Errorf("run not found"))
		return
	}
	respondJSON(w, http.StatusOK, run)
}

type runBenchmarkRequest struct {
	NodeID string               `json:"node_id"`
	Kind   string               `json:"kind,omitempty"`
	Tests  []benchmark.TestType `json:"tests,omitempty"`
}

func (s *APIServer) runBenchmark(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	var req runBenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		respondError(w, http.StatusBadRequest, fmt.Errorf("node_id is required"))
		return
	}
	run, err := ops.Benchmark.RunSimulated(req.NodeID, req.Kind, req.Tests)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, run)
}

func (s *APIServer) calibrateBaseline(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	updated, err := ops.Benchmark.Calibrate(mux.Vars(r)["runId"])
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

type ingestRequest struct {
	NodeID  string                 `json:"node_id"`
	Kind    string                 `json:"kind,omitempty"`
	Results []benchmark.IngestInput `json:"results"`
}

func (s *APIServer) ingestBenchmark(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	run, err := ops.Benchmark.Ingest(req.NodeID, req.Kind, req.Results)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, run)
}

type diagnoseRequest struct {
	NodeID     string `json:"node_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Question   string `json:"question,omitempty"`
}

func (s *APIServer) diagnose(w http.ResponseWriter, r *http.Request) {
	ops := s.requireOps(w)
	if ops == nil {
		return
	}
	var req diagnoseRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	ctx := diagnosis.Context{
		NodeID:     req.NodeID,
		ResourceID: req.ResourceID,
		Question:   req.Question,
		Events:     ops.Engine.Events(remediation.EventFilter{NodeID: req.NodeID, ResourceID: req.ResourceID, Limit: 30}),
		Conditions: ops.Engine.Conditions(),
	}
	for _, t := range ops.Hub.Latest() {
		if (req.NodeID == "" || t.NodeID == req.NodeID) && (req.ResourceID == "" || t.ResourceID == req.ResourceID) {
			ctx.Telemetry = append(ctx.Telemetry, t)
		}
	}
	for _, run := range ops.Benchmark.Runs(req.NodeID) {
		for _, res := range run.Results {
			if res.Status != benchmark.StatusPass {
				ctx.Anomalies = append(ctx.Anomalies, res)
			}
		}
	}
	respondJSON(w, http.StatusOK, ops.Diagnoser.Diagnose(r.Context(), ctx))
}

func (s *APIServer) requireProbe(w http.ResponseWriter) *probe.Manager {
	ops := s.requireOps(w)
	if ops == nil {
		return nil
	}
	if ops.Probe == nil {
		respondError(w, http.StatusServiceUnavailable, fmt.Errorf("probe manager not configured"))
		return nil
	}
	return ops.Probe
}

func (s *APIServer) opsReadiness(w http.ResponseWriter, r *http.Request) {
	p := s.requireProbe(w)
	if p == nil {
		return
	}
	respondJSON(w, http.StatusOK, p.Readiness(r.Context()))
}

func (s *APIServer) getProbe(w http.ResponseWriter, r *http.Request) {
	p := s.requireProbe(w)
	if p == nil {
		return
	}
	if last := p.Last(); last != nil {
		respondJSON(w, http.StatusOK, last)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "no probe has been run yet"})
}

type runProbeRequest struct {
	Types []pool.ResourceType `json:"types,omitempty"`
}

func (s *APIServer) runProbe(w http.ResponseWriter, r *http.Request) {
	p := s.requireProbe(w)
	if p == nil {
		return
	}
	var req runProbeRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	respondJSON(w, http.StatusOK, p.Probe(r.Context(), req.Types))
}
