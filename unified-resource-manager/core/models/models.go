/*
 * Unified Resource Manager - Model Marketplace
 *
 * 提供简单的模型注册、查询和部署入口，用于“模型市场”能力。
 * 当前实现为内存存储，便于演示和后续替换为持久化存储。
 */

package models

import (
	"fmt"
	"sync"
	"time"
)

// Model 表示一个可管理的 AI 模型（例如 LLM、CV 模型等）。
type Model struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Type            string            `json:"type"`        // llm/cv/asr/embedding/rerank/image/audio/video/custom
	Framework       string            `json:"framework"`   // pytorch/tensorflow/vllm/...
	Format          string            `json:"format"`      // pytorch/gguf/gptq/awq/fp8/mlx
	ResourceFlavorID string           `json:"resource_flavor_id"` // 推荐资源规格
	Path            string            `json:"path"`        // 模型权重路径或存储地址
	Description     string            `json:"description"`
	ContextLength   int               `json:"context_length"`   // 上下文长度（K tokens）
	Size            int               `json:"size"`             // 模型大小（Billion parameters）
	Family          string            `json:"family"`           // 模型家族（如 llama/gpt/qwen）
	Languages       []string          `json:"languages"`        // 支持的语言（en/zh/...）
	Abilities       []string          `json:"abilities"`         // 能力（generate/chat/vision/tools/reasoning/audio/omni/hybrid）
	Labels            map[string]string `json:"labels"`
	TenantID          string            `json:"tenant_id"`
	ProjectID         string            `json:"project_id"`         // 关联的项目ID
	InferenceBackend  string            `json:"inference_backend"`  // 推理后端：空/gpu/nvidia_nim
	NIMModelID        string            `json:"nim_model_id"`       // 当 inference_backend=nvidia_nim 时，NIM 侧模型 ID
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// Store 简单的内存模型存储。
type Store struct {
	mu     sync.RWMutex
	models map[string]*Model
}

// NewStore 创建新的模型存储。
func NewStore() *Store {
	return &Store{
		models: make(map[string]*Model),
	}
}

// ListModels 列出所有模型，简单实现可根据后续需要增加过滤条件。
func (s *Store) ListModels() []*Model {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Model, 0, len(s.models))
	for _, m := range s.models {
		result = append(result, m)
	}
	return result
}

// GetModel 获取模型。
func (s *Store) GetModel(id string) (*Model, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.models[id]
	if !ok {
		return nil, fmt.Errorf("model not found: %s", id)
	}
	return m, nil
}

// CreateModel 创建模型。
func (s *Store) CreateModel(m *Model) error {
	if m == nil || m.ID == "" {
		return fmt.Errorf("model ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.models[m.ID]; exists {
		return fmt.Errorf("model already exists: %s", m.ID)
	}
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	if m.Labels == nil {
		m.Labels = map[string]string{}
	}
	s.models[m.ID] = m
	return nil
}

// DeleteModel 删除模型。
func (s *Store) DeleteModel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.models[id]; !ok {
		return fmt.Errorf("model not found: %s", id)
	}
	delete(s.models, id)
	return nil
}

