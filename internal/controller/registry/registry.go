// Package registry 提供两种"按名查找"的注册表：
//
//   - SkillRegistry         技能注册表（这个文件）— 从 Store 拉取启用的 Skill
//   - ClusterClientRegistry 多集群客户端注册表（cluster_registry.go）
//
// SkillRegistry 实现了 translator.SkillProvider 接口（鸭子类型），
// 让 Translator 不直接依赖 store 包，便于测试时注入假数据。
//
// 注意：ListEnabled 每次都查 Store，未做缓存 — 这是有意为之，
// 让 DiagnosticSkill CR 修改后下一次诊断立刻生效（热重载）。
package registry

import (
	"context"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// SkillRegistry 返回当前所有启用的技能集合。
type SkillRegistry interface {
	// ListEnabled returns all enabled skills, ordered by priority ASC.
	ListEnabled(ctx context.Context) ([]*store.Skill, error)
}

type storeRegistry struct {
	store store.Store
}

func New(s store.Store) SkillRegistry {
	return &storeRegistry{store: s}
}

func (r *storeRegistry) ListEnabled(ctx context.Context) ([]*store.Skill, error) {
	all, err := r.store.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]*store.Skill, 0, len(all))
	for _, s := range all {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}