package registry

import (
	"context"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// SkillRegistry provides the current merged set of skills.
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