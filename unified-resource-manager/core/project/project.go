/*
 * Unified Resource Manager - Project Management
 *
 * 项目管理：用于组织和管理 AI 项目，关联模型、任务、资源等
 */

package project

import (
	"fmt"
	"sync"
	"time"
)

// Project 表示一个 AI 项目
type Project struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	TenantID    string            `json:"tenant_id"`
	OwnerID     string            `json:"owner_id"`
	Labels      map[string]string `json:"labels"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Store 项目存储
type Store struct {
	mu       sync.RWMutex
	projects map[string]*Project
}

// NewStore 创建项目存储
func NewStore() *Store {
	return &Store{
		projects: make(map[string]*Project),
	}
}

// ListProjects 列出所有项目
func (s *Store) ListProjects() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		result = append(result, p)
	}
	return result
}

// GetProject 获取项目
func (s *Store) GetProject(id string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	if !ok {
		return nil, fmt.Errorf("project not found: %s", id)
	}
	return p, nil
}

// CreateProject 创建项目
func (s *Store) CreateProject(p *Project) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[p.ID]; exists {
		return fmt.Errorf("project already exists: %s", p.ID)
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}
	s.projects[p.ID] = p
	return nil
}

// UpdateProject 更新项目
func (s *Store) UpdateProject(p *Project) error {
	if p == nil || p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[p.ID]; !exists {
		return fmt.Errorf("project not found: %s", p.ID)
	}
	p.UpdatedAt = time.Now()
	s.projects[p.ID] = p
	return nil
}

// DeleteProject 删除项目
func (s *Store) DeleteProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[id]; !ok {
		return fmt.Errorf("project not found: %s", id)
	}
	delete(s.projects, id)
	return nil
}
