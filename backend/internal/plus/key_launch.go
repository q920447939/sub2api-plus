// Package plus contains fork-specific extensions to sub2api.
// Each file is self-contained; forks touch upstream code only at marked [plus] hooks.
package plus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// KeyLaunchConfig is stored as JSON in CustomMenuItem.PlusConfig for menus with URL "plus:key_launch".
// It is admin-only: public settings strip plus_config, and the backend re-validates it on every launch.
type KeyLaunchConfig struct {
	GroupID        int64  `json:"group_id"`
	URLTemplate    string `json:"url_template"`               // must contain {key}
	RechargeMenuID string `json:"recharge_menu_id,omitempty"` // menu id of the recharge entry
}

// ParseKeyLaunchConfig validates and returns a KeyLaunchConfig from raw JSON.
func ParseKeyLaunchConfig(raw json.RawMessage) (KeyLaunchConfig, error) {
	if len(raw) == 0 {
		return KeyLaunchConfig{}, fmt.Errorf("plus_config is required for key_launch menus")
	}
	var cfg KeyLaunchConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return KeyLaunchConfig{}, fmt.Errorf("invalid plus_config: %w", err)
	}
	if cfg.GroupID <= 0 {
		return KeyLaunchConfig{}, fmt.Errorf("plus_config.group_id is required")
	}
	if !strings.Contains(cfg.URLTemplate, "{key}") {
		return KeyLaunchConfig{}, fmt.Errorf("plus_config.url_template must contain {key}")
	}
	probe := strings.ReplaceAll(cfg.URLTemplate, "{key}", "probe")
	u, err := url.Parse(probe)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return KeyLaunchConfig{}, fmt.Errorf("plus_config.url_template must be an absolute http(s) URL")
	}
	return cfg, nil
}

// LaunchResult is returned to the frontend so it can open the target URL.
type LaunchResult struct {
	URL string `json:"url"`
}

// BlockAction is a button the frontend renders so the user can resolve the block.
type BlockAction struct {
	Label string `json:"label"`
	Path  string `json:"path"` // frontend route, e.g. /purchase, /keys or /custom/<id>
}

// BlockReason explains why the launch was refused.
type BlockReason struct {
	Code    string       `json:"code"` // no_group_access | no_balance | key_exhausted | key_expired
	Message string       `json:"message"`
	Action  *BlockAction `json:"action,omitempty"`
}

// ErrBlocked signals an expected, user-resolvable refusal (HTTP 403), not a server fault.
type ErrBlocked struct {
	Reason BlockReason
}

func (e *ErrBlocked) Error() string { return "launch blocked: " + e.Reason.Code }

// Deps are the upstream services used by Launch. They are passed in from registerRoutes,
// so wire.go and wire_gen.go stay untouched.
type Deps struct {
	APIKey       *service.APIKeyService
	Subscription *service.SubscriptionService
	Settings     *service.SettingService
}

// dedicatedKeyName identifies the auto-created dedicated key. Renaming it causes a new key
// to be created on the next launch; the old key is left alone.
const dedicatedKeyName = "自动创建（请勿改名）"

// Launch runs the whole flow: permission check, balance/subscription check, dedicated key
// resolution, and URL template expansion.
func Launch(ctx context.Context, deps Deps, userID int64, cfg KeyLaunchConfig) (*LaunchResult, error) {
	group, err := service.GetGroupByID(ctx, deps.APIKey, cfg.GroupID)
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	if group == nil || group.Status != service.StatusActive {
		return nil, &ErrBlocked{Reason: BlockReason{
			Code:    "no_group_access",
			Message: "该分组当前不可用",
		}}
	}

	user, err := service.GetUserByID(ctx, deps.APIKey, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if !service.CanUserBindGroup(ctx, deps.APIKey, user, group) {
		return nil, &ErrBlocked{Reason: BlockReason{
			Code:    "no_group_access",
			Message: "您暂无该分组的访问权限，开通后可继续使用",
			Action:  &BlockAction{Label: "去开通", Path: "/purchase"},
		}}
	}

	if err := checkEligibility(ctx, deps, user, group); err != nil {
		return nil, err
	}

	key, err := ensureDedicatedKey(ctx, deps.APIKey, userID, cfg.GroupID)
	if err != nil {
		return nil, err
	}

	finalURL, err := fillTemplate(cfg.URLTemplate, key.Key)
	if err != nil {
		return nil, err
	}
	return &LaunchResult{URL: finalURL}, nil
}

// checkEligibility mirrors the gateway's billing gate so the pre-launch check and the real
// request agree. It deliberately skips the RPM counter, which mutates state.
func checkEligibility(ctx context.Context, deps Deps, user *service.User, group *service.Group) error {
	action := &BlockAction{Label: "去充值", Path: "/purchase"}

	if group.IsSubscriptionType() {
		err := service.CheckSubscriptionEligibility(ctx, deps.Subscription, user.ID, group)
		if err == nil {
			return nil
		}
		if infraerrors.Reason(err) == "SUBSCRIPTION_NOT_FOUND" {
			return &ErrBlocked{Reason: BlockReason{Code: "no_balance", Message: "您还没有该分组的有效订阅", Action: action}}
		}
		return &ErrBlocked{Reason: BlockReason{
			Code:    "no_balance",
			Message: "订阅无效或已超出用量限制",
			Action:  action,
		}}
	}

	if err := service.CheckBalanceEligibility(user, 0); err != nil {
		return &ErrBlocked{Reason: BlockReason{
			Code:    "no_balance",
			Message: "余额不足，请先充值后再使用",
			Action:  action,
		}}
	}
	return nil
}

// ensureDedicatedKey returns the user's dedicated key in the group, creating one when it is
// missing, deleted or disabled. A key the user capped or that expired is reported instead of
// being silently replaced, so a user-set limit is never bypassed.
func ensureDedicatedKey(ctx context.Context, svc *service.APIKeyService, userID, groupID int64) (*service.APIKey, error) {
	existing, err := service.FindKeyByName(ctx, svc, userID, groupID, dedicatedKeyName)
	if err != nil {
		return nil, fmt.Errorf("find dedicated key: %w", err)
	}

	if existing != nil {
		switch existing.Status {
		case service.StatusAPIKeyActive:
			return existing, nil
		case service.StatusAPIKeyQuotaExhausted:
			return nil, &ErrBlocked{Reason: BlockReason{
				Code:    "key_exhausted",
				Message: "专属密钥的额度已用完，请在 API 密钥页调整",
				Action:  &BlockAction{Label: "去管理", Path: "/keys"},
			}}
		case service.StatusAPIKeyExpired:
			return nil, &ErrBlocked{Reason: BlockReason{
				Code:    "key_expired",
				Message: "专属密钥已过期，请在 API 密钥页处理",
				Action:  &BlockAction{Label: "去管理", Path: "/keys"},
			}}
		}
	}

	gid := groupID
	created, err := service.CreateDedicatedKey(ctx, svc, userID, dedicatedKeyName, &gid)
	if err != nil {
		return nil, fmt.Errorf("create dedicated key: %w", err)
	}
	return created, nil
}

// fillTemplate substitutes the URL-encoded key and verifies the substitution cannot move the
// request to a different origin.
func fillTemplate(tmpl, key string) (string, error) {
	result := strings.ReplaceAll(tmpl, "{key}", url.QueryEscape(key))

	tmplURL, err := url.Parse(strings.ReplaceAll(tmpl, "{key}", "probe"))
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}
	resultURL, err := url.Parse(result)
	if err != nil {
		return "", fmt.Errorf("invalid result URL: %w", err)
	}
	if tmplURL.Scheme != resultURL.Scheme || tmplURL.Host != resultURL.Host {
		return "", fmt.Errorf("template substitution changed the target origin")
	}
	return result, nil
}
