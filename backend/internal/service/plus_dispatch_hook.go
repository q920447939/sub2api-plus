// plus_dispatch_hook.go — [plus] 上游调度台的选号钩子。
//
// 上游只改了 6 个函数声明（改名为 xxxUpstream，标 [plus]），原名在这里重新实现：
// 先调用上游原函数，再按 plus 调度台的「当前调整」改写结果。没有注册过滤器、
// 总开关关闭或分组没有调整时，原样返回，行为和上游完全一致。
//
//   - 候选名单：GatewayService.listSchedulableAccounts、OpenAIGatewayService.listSchedulableAccounts、
//     GeminiMessagesCompatService.listSchedulableAccountsOnce
//   - 粘性会话（只处理「暂停」）：GatewayService.isAccountInGroup、
//     OpenAIGatewayService.openAIAccountMatchesSchedulingGroup、GeminiMessagesCompatService.tryStickySessionHit
package service

import (
	"context"
	"sync/atomic"
)

// PlusDispatchFilter 由 plus/dispatch 实现
type PlusDispatchFilter interface {
	// Filter 返回每个候选是否拿掉；nil 表示不改动。stickyAccountID 为已知的粘性账号（未知为 0）。
	Filter(groupID int64, accountIDs []int64, stickyAccountID int64) []bool
	// Paused 这个账号在这个分组是否被暂停
	Paused(groupID, accountID int64) bool
}

type plusDispatchHolder struct{ f PlusDispatchFilter }

var plusDispatch atomic.Pointer[plusDispatchHolder]

// SetPlusDispatchFilter 注册过滤器（plus 模块启动时调用一次）
func SetPlusDispatchFilter(f PlusDispatchFilter) {
	plusDispatch.Store(&plusDispatchHolder{f: f})
}

func plusDispatchFilter() PlusDispatchFilter {
	if h := plusDispatch.Load(); h != nil {
		return h.f
	}
	return nil
}

// plusApplyDispatch 改写候选名单。上游的名单可能来自共享快照，这里从不原地修改，只返回新切片。
func plusApplyDispatch(ctx context.Context, groupID *int64, accounts []Account) []Account {
	f := plusDispatchFilter()
	if f == nil || groupID == nil || *groupID <= 0 || len(accounts) == 0 {
		return accounts
	}
	ids := make([]int64, len(accounts))
	for i := range accounts {
		ids[i] = accounts[i].ID
	}
	drop := f.Filter(*groupID, ids, prefetchedStickyAccountIDFromContext(ctx, groupID))
	if drop == nil {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if !drop[i] {
			out = append(out, accounts[i])
		}
	}
	return out
}

func plusDispatchPaused(groupID *int64, accountID int64) bool {
	f := plusDispatchFilter()
	return f != nil && groupID != nil && *groupID > 0 && f.Paused(*groupID, accountID)
}

// ---------- 候选名单 ----------

func (s *GatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, bool, error) {
	accounts, useMixed, err := s.listSchedulableAccountsUpstream(ctx, groupID, platform, hasForcePlatform)
	if err != nil {
		return accounts, useMixed, err
	}
	return plusApplyDispatch(ctx, groupID, accounts), useMixed, nil
}

func (s *OpenAIGatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, platform string) ([]Account, error) {
	accounts, err := s.listSchedulableAccountsUpstream(ctx, groupID, platform)
	if err != nil {
		return accounts, err
	}
	return plusApplyDispatch(ctx, groupID, accounts), nil
}

func (s *GeminiMessagesCompatService) listSchedulableAccountsOnce(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]Account, error) {
	accounts, err := s.listSchedulableAccountsOnceUpstream(ctx, groupID, platform, hasForcePlatform)
	if err != nil {
		return accounts, err
	}
	return plusApplyDispatch(ctx, groupID, accounts), nil
}

// ---------- 粘性会话：暂停的账号不再命中，后续正常选号会改绑到别的账号 ----------

func (s *GatewayService) isAccountInGroup(account *Account, groupID *int64) bool {
	return s.isAccountInGroupUpstream(account, groupID) && !plusDispatchPaused(groupID, account.ID)
}

func (s *OpenAIGatewayService) openAIAccountMatchesSchedulingGroup(account *Account, groupID *int64) bool {
	if !s.openAIAccountMatchesSchedulingGroupUpstream(account, groupID) {
		return false
	}
	return account == nil || !plusDispatchPaused(groupID, account.ID)
}

func (s *GeminiMessagesCompatService) tryStickySessionHit(
	ctx context.Context,
	groupID *int64,
	sessionHash, cacheKey, requestedModel string,
	excludedIDs map[int64]struct{},
	platform string,
	useMixedScheduling bool,
) *Account {
	account := s.tryStickySessionHitUpstream(ctx, groupID, sessionHash, cacheKey, requestedModel, excludedIDs, platform, useMixedScheduling)
	if account != nil && plusDispatchPaused(groupID, account.ID) {
		return nil
	}
	return account
}

// PlusAccountUpstreamModel 按账号的模型映射换算出真正发给上游的模型名（账号直连检测用，和网关行为一致）
func PlusAccountUpstreamModel(platform, accountType string, credentials map[string]any, requestedModel string) string {
	a := &Account{Platform: platform, Type: accountType, Credentials: credentials}
	return a.GetMappedModel(requestedModel)
}
