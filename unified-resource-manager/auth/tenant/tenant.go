/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - Multi-Tenant Support
 */

package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Tenant 租户
type Tenant struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Quota       *TenantQuota      `json:"quota"`
	Used        *TenantUsage      `json:"used"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// TenantQuota 租户配额
type TenantQuota struct {
	MaxGPUs      int   `json:"max_gpus"`
	MaxNPUs      int   `json:"max_npus"`
	MaxMemoryGB  int64 `json:"max_memory_gb"`
	MaxTasks     int   `json:"max_tasks"`
}

// TenantUsage 租户使用量
type TenantUsage struct {
	GPUs      int   `json:"gpus"`
	NPUs      int   `json:"npus"`
	MemoryGB  int64 `json:"memory_gb"`
	Tasks     int   `json:"tasks"`
}

// TenantManager 租户管理器
type TenantManager struct {
	tenants map[string]*Tenant
	mu      sync.RWMutex
}

// NewTenantManager 创建租户管理器
func NewTenantManager() *TenantManager {
	return &TenantManager{
		tenants: make(map[string]*Tenant),
	}
}

// CreateTenant 创建租户
func (tm *TenantManager) CreateTenant(tenant *Tenant) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if tenant.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	
	if _, exists := tm.tenants[tenant.ID]; exists {
		return fmt.Errorf("tenant already exists: %s", tenant.ID)
	}
	
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()
	if tenant.Used == nil {
		tenant.Used = &TenantUsage{}
	}
	
	tm.tenants[tenant.ID] = tenant
	return nil
}

// GetTenant 获取租户
func (tm *TenantManager) GetTenant(tenantID string) (*Tenant, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	tenant, ok := tm.tenants[tenantID]
	if !ok {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}
	
	return tenant, nil
}

// ListTenants 列出所有租户
func (tm *TenantManager) ListTenants() ([]*Tenant, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	tenants := make([]*Tenant, 0, len(tm.tenants))
	for _, tenant := range tm.tenants {
		tenants = append(tenants, tenant)
	}
	
	return tenants, nil
}

// CheckQuota 检查配额
func (tm *TenantManager) CheckQuota(ctx context.Context, tenantID string, req *QuotaRequest) (bool, error) {
	tenant, err := tm.GetTenant(tenantID)
	if err != nil {
		return false, err
	}
	
	if tenant.Quota == nil {
		// 无配额限制
		return true, nil
	}
	
	// 检查 GPU 配额
	if req.GPUs > 0 {
		if tenant.Used.GPUs+req.GPUs > tenant.Quota.MaxGPUs {
			return false, fmt.Errorf("GPU quota exceeded")
		}
	}
	
	// 检查 NPU 配额
	if req.NPUs > 0 {
		if tenant.Used.NPUs+req.NPUs > tenant.Quota.MaxNPUs {
			return false, fmt.Errorf("NPU quota exceeded")
		}
	}
	
	// 检查内存配额
	if req.MemoryGB > 0 {
		if tenant.Used.MemoryGB+req.MemoryGB > tenant.Quota.MaxMemoryGB {
			return false, fmt.Errorf("memory quota exceeded")
		}
	}
	
	// 检查任务配额
	if req.Tasks > 0 {
		if tenant.Used.Tasks+req.Tasks > tenant.Quota.MaxTasks {
			return false, fmt.Errorf("task quota exceeded")
		}
	}
	
	return true, nil
}

// UpdateUsage 更新使用量
func (tm *TenantManager) UpdateUsage(tenantID string, usage *TenantUsage) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tenant, ok := tm.tenants[tenantID]
	if !ok {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	
	tenant.Used = usage
	tenant.UpdatedAt = time.Now()
	
	return nil
}

// QuotaRequest 配额请求
type QuotaRequest struct {
	GPUs     int   `json:"gpus"`
	NPUs     int   `json:"npus"`
	MemoryGB int64 `json:"memory_gb"`
	Tasks    int   `json:"tasks"`
}
