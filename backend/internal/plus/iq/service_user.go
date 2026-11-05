// Package iq — [plus] 用户端用例。菜单即身份：这里只产出用户 DTO（dto_user.go），
// 结构里就没有账号、Key、错误、耗时等字段，管理端的数据不会经过这里。
package iq

import (
	"context"
	"sort"
)

// UserService 用户端「模型画廊」
type UserService struct {
	Config     ConfigStore
	Tasks      TaskStore
	Runs       RunReader
	Catalog    GroupCatalog
	Visibility Visibility
	Signer     func(ctx context.Context) (Signer, error)
}

// Meta 侧边栏是否显示菜单、菜单名称
func (s *UserService) Meta(ctx context.Context) (UserMeta, error) {
	cfg, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return UserMeta{}, err
	}
	return UserMeta{Enabled: cfg.UserEnabled, MenuName: cfg.MenuName}, nil
}

// Gallery 当前用户可见分组的成功样本，最新在前
func (s *UserService) Gallery(ctx context.Context, userID int64) (UserGallery, error) {
	cfg, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return UserGallery{}, err
	}
	if !cfg.UserEnabled {
		return UserGallery{}, ErrDisabled
	}
	visible, err := s.Visibility.VisibleGroups(ctx, userID)
	if err != nil {
		return UserGallery{}, err
	}
	groups, err := s.Catalog.ListGroups(ctx)
	if err != nil {
		return UserGallery{}, err
	}
	tasks, err := s.Tasks.ListTasks(ctx)
	if err != nil {
		return UserGallery{}, err
	}
	byGroup := map[int64][]Task{}
	var ids []int64
	for _, t := range tasks {
		if t.Enabled && visible[t.GroupID] {
			byGroup[t.GroupID] = append(byGroup[t.GroupID], t)
			ids = append(ids, t.ID)
		}
	}
	recent, err := s.Runs.RecentRuns(ctx, ids, cfg.UserSamplesPerTask, true)
	if err != nil {
		return UserGallery{}, err
	}
	signer, err := s.Signer(ctx)
	if err != nil {
		return UserGallery{}, err
	}

	out := UserGallery{Groups: []UserGroup{}}
	for _, g := range groups {
		var ug *UserGroup
		gt := byGroup[g.ID]
		sort.Slice(gt, func(i, j int) bool { return gt[i].Model < gt[j].Model })
		for _, t := range gt {
			runs := recent[t.ID]
			if len(runs) == 0 {
				continue
			}
			if ug == nil {
				ug = &UserGroup{ID: g.ID, Name: g.Name, Platform: g.Platform}
			}
			ut := UserTask{Model: t.Model, Samples: make([]UserSample, 0, len(runs))}
			for _, r := range runs {
				at := r.QueuedAt
				if r.FinishedAt != nil {
					at = *r.FinishedAt
				}
				ut.Samples = append(ut.Samples, UserSample{ID: r.ID, CreatedAt: at, PreviewURL: signer.URL(r.ID, ScopeUser)})
			}
			ug.Tasks = append(ug.Tasks, ut)
		}
		if ug != nil {
			out.Groups = append(out.Groups, *ug)
		}
	}
	return out, nil
}
