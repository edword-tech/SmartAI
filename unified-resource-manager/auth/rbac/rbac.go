/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2024-2025. All rights reserved.
 * Unified Resource Manager - RBAC (Role-Based Access Control)
 */

package rbac

import (
	"context"
	"fmt"
	"sync"
)

// Role 角色
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleUser     Role = "user"
	RoleViewer   Role = "viewer"
)

// Permission 权限
type Permission string

const (
	PermissionCreatePool    Permission = "pools:create"
	PermissionDeletePool    Permission = "pools:delete"
	PermissionViewPool      Permission = "pools:view"
	PermissionAllocate      Permission = "resources:allocate"
	PermissionRelease       Permission = "resources:release"
	PermissionSubmitTask    Permission = "tasks:submit"
	PermissionCancelTask    Permission = "tasks:cancel"
	PermissionViewTask      Permission = "tasks:view"
	PermissionViewMetrics   Permission = "metrics:view"
)

// RolePermissions 角色权限映射
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermissionCreatePool,
		PermissionDeletePool,
		PermissionViewPool,
		PermissionAllocate,
		PermissionRelease,
		PermissionSubmitTask,
		PermissionCancelTask,
		PermissionViewTask,
		PermissionViewMetrics,
	},
	RoleOperator: {
		PermissionViewPool,
		PermissionAllocate,
		PermissionRelease,
		PermissionSubmitTask,
		PermissionCancelTask,
		PermissionViewTask,
		PermissionViewMetrics,
	},
	RoleUser: {
		PermissionViewPool,
		PermissionAllocate,
		PermissionRelease,
		PermissionSubmitTask,
		PermissionViewTask,
	},
	RoleViewer: {
		PermissionViewPool,
		PermissionViewTask,
		PermissionViewMetrics,
	},
}

// User 用户
type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []Role   `json:"roles"`
	TenantID string   `json:"tenant_id"`
}

// RBACManager RBAC 管理器
type RBACManager struct {
	users map[string]*User
	mu    sync.RWMutex
}

// NewRBACManager 创建 RBAC 管理器
func NewRBACManager() *RBACManager {
	return &RBACManager{
		users: make(map[string]*User),
	}
}

// CheckPermission 检查权限
func (rm *RBACManager) CheckPermission(ctx context.Context, userID string, permission Permission) (bool, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	user, ok := rm.users[userID]
	if !ok {
		return false, fmt.Errorf("user not found: %s", userID)
	}
	
	// 检查用户的所有角色是否有该权限
	for _, role := range user.Roles {
		permissions, ok := RolePermissions[role]
		if !ok {
			continue
		}
		
		for _, p := range permissions {
			if p == permission {
				return true, nil
			}
		}
	}
	
	return false, nil
}

// GetUser 获取用户
func (rm *RBACManager) GetUser(userID string) (*User, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	user, ok := rm.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	
	return user, nil
}

// CreateUser 创建用户
func (rm *RBACManager) CreateUser(user *User) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if user.ID == "" {
		return fmt.Errorf("user ID is required")
	}
	
	rm.users[user.ID] = user
	return nil
}

// UpdateUserRoles 更新用户角色
func (rm *RBACManager) UpdateUserRoles(userID string, roles []Role) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	user, ok := rm.users[userID]
	if !ok {
		return fmt.Errorf("user not found: %s", userID)
	}
	
	user.Roles = roles
	return nil
}
