/*
 * Unified Resource Manager - NVIDIA NIM (Inference Microservices) Integration
 *
 * 提供 NIM 连接配置的存储与查询，供推理任务路由到 NIM 服务时使用。
 * 不修改现有 GPU/NPU 发现与调度逻辑，仅增加 NIM 作为可选推理后端。
 */

package nim

import (
	"sync"
	"time"
)

// BackendType 推理后端类型
const (
	BackendGPU RawBackend = "gpu"       // 直连 GPU 推理（vLLM/TensorRT 等）
	BackendNIM RawBackend = "nvidia_nim" // NVIDIA NIM 推理微服务
)

// RawBackend 表示推理后端标识
type RawBackend string

// Config 表示单个 NIM 实例/集群的配置（如 Base URL、API 端口等）
type Config struct {
	// BaseURL NIM 服务根地址（不含路径），例如 https://nim.example.com 或 http://nim-gateway:8000
	BaseURL string `json:"base_url"`
	// APIKeyCredentialID 可选：Jenkins/凭据系统中的 API Key 凭据 ID，用于调用 NIM API 鉴权
	APIKeyCredentialID string `json:"api_key_credential_id,omitempty"`
	// Enabled 是否启用该 NIM 配置
	Enabled bool `json:"enabled"`
	// DefaultNamespace 可选：K8s 下 NIM 部署的 namespace，用于生成内部路由
	DefaultNamespace string `json:"default_namespace,omitempty"`
	// UpdatedAt 最后更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// Store 内存存储，单例配置（可后续扩展为多 NIM 实例）
type Store struct {
	mu     sync.RWMutex
	config *Config
}

// NewStore 创建 NIM 配置存储
func NewStore() *Store {
	return &Store{
		config: &Config{
			BaseURL: "",
			Enabled: false,
			UpdatedAt: time.Now(),
		},
	}
}

// GetConfig 返回当前 NIM 配置（只读副本）
func (s *Store) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil {
		return Config{UpdatedAt: time.Now()}
	}
	return Config{
		BaseURL:           s.config.BaseURL,
		APIKeyCredentialID: s.config.APIKeyCredentialID,
		Enabled:           s.config.Enabled,
		DefaultNamespace:  s.config.DefaultNamespace,
		UpdatedAt:         s.config.UpdatedAt,
	}
}

// UpdateConfig 更新 NIM 配置
func (s *Store) UpdateConfig(c *Config) error {
	if c == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.config = &Config{
		BaseURL:            c.BaseURL,
		APIKeyCredentialID: c.APIKeyCredentialID,
		Enabled:            c.Enabled,
		DefaultNamespace:   c.DefaultNamespace,
		UpdatedAt:          now,
	}
	return nil
}

// IsNIMBackend 判断给定字符串是否为 NIM 后端
func IsNIMBackend(backend string) bool {
	return RawBackend(backend) == BackendNIM
}
