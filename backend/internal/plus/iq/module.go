// Package iq — [plus] 模块装配（Composition Root）
package iq

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Register 装配并注册路由，启动调度器。
// v1 用于挂载预览接口（签名鉴权，不走 JWT）；userGroup 为 /plus（JWT）；adminGroup 为 /plus/admin。
func Register(db *sql.DB, apiKeys *service.APIKeyService, v1, userGroup, adminGroup *gin.RouterGroup) {
	repo := NewRepository(db)
	signer := cachedSigner(repo)
	client := StreamClient{}
	channel := GatewayChannel{Keys: repo}

	exec := NewExecutor(ExecutorDeps{
		Config: repo, Tasks: repo, Runs: repo, Catalog: repo,
		Channel: channel, Challenge: PelicanChallenge{}, Client: client,
		Attributor: UsageLogAttributor{DB: db},
	})
	sched := NewScheduler(repo, repo, exec)

	NewUserHandler(&UserService{
		Config: repo, Tasks: repo, Runs: repo, Catalog: repo,
		Visibility: availableGroups{svc: apiKeys}, Signer: signer,
	}).RegisterUserRoutes(userGroup)

	NewAdminHandler(NewAdminService(AdminDeps{
		Config: repo, Tasks: repo, Runs: repo, Queue: repo, Catalog: repo, Keys: repo,
		Provisioner: keyProvisioner{svc: apiKeys}, Channel: channel, Client: client,
		Models: client.ListModels, Signer: signer, Wake: sched.Wake,
	})).RegisterAdminRoutes(adminGroup)

	(&PreviewHandler{Runs: repo, Config: repo, Signer: signer}).Register(v1)

	sched.Start(context.Background())
}

// cachedSigner 签名密钥在迁移时生成且不变，读一次即可
func cachedSigner(store ConfigStore) func(ctx context.Context) (Signer, error) {
	var (
		mu     sync.Mutex
		secret []byte
	)
	return func(ctx context.Context) (Signer, error) {
		mu.Lock()
		defer mu.Unlock()
		if secret == nil {
			s, err := store.PreviewSecret(ctx)
			if err != nil {
				return Signer{}, err
			}
			secret = s
		}
		return Signer{Secret: secret}, nil
	}
}

// availableGroups 用户可见分组 = 用户能绑定 Key 的分组（与「API 密钥」页可选分组一致）
type availableGroups struct{ svc *service.APIKeyService }

func (a availableGroups) VisibleGroups(ctx context.Context, userID int64) (map[int64]bool, error) {
	groups, err := a.svc.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(groups))
	for _, g := range groups {
		out[g.ID] = true
	}
	return out, nil
}

// keyProvisioner 通过上游 APIKeyService 创建 Key（走正常的分组权限校验）
type keyProvisioner struct{ svc *service.APIKeyService }

func (k keyProvisioner) FindGroupKey(ctx context.Context, userID, groupID int64, name string) (int64, bool, error) {
	key, err := service.FindKeyByName(ctx, k.svc, userID, groupID, name)
	if err != nil || key == nil {
		return 0, false, err
	}
	return key.ID, true, nil
}

func (k keyProvisioner) EnsureGroupKey(ctx context.Context, userID, groupID int64, name string) (int64, error) {
	if id, found, err := k.FindGroupKey(ctx, userID, groupID, name); err != nil || found {
		return id, err
	}
	gid := groupID
	key, err := service.CreateDedicatedKey(ctx, k.svc, userID, name, &gid)
	if err != nil {
		return 0, err
	}
	if key == nil {
		return 0, errors.New("创建 Key 失败")
	}
	return key.ID, nil
}
