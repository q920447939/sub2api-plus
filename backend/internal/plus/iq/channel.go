// Package iq — [plus] 执行通道。本期只有 GatewayChannel：本站网关 + 分组 Key，贴近用户的真实调用。
// 以后的 AccountDirectChannel（直连某个账号的 base_url + key）只需实现同一个接口。
package iq

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// GatewayChannel 网关通道
type GatewayChannel struct {
	Keys KeyStore
}

var _ Channel = GatewayChannel{}

func (g GatewayChannel) Resolve(ctx context.Context, task Task, group GroupInfo, cfg Config) (Target, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return Target{}, errors.New("请先在设置里填写网关地址")
	}
	key, keyGroup, err := g.Keys.ResolveKey(ctx, task.APIKeyID)
	if errors.Is(err, ErrKeyUnavailable) {
		return Target{}, errors.New("探测 Key 不可用（已删除、停用或过期）")
	}
	if err != nil {
		return Target{}, fmt.Errorf("读取探测 Key：%w", err)
	}
	if keyGroup != task.GroupID {
		return Target{}, errors.New("探测 Key 已不属于该分组")
	}
	return Target{Endpoint: endpoint, APIKey: key, Model: task.Model, Format: ResolveFormat(task.Format, group.Platform)}, nil
}
