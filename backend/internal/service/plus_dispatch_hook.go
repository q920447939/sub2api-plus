// plus_dispatch_hook.go — [plus] 上游调度台的选号钩子。
//
// 上游只改了 7 个函数声明（改名为 xxxUpstream，标 [plus]），原名在这里重新实现：
// 先调用上游原函数，再按 plus 调度台的「权重设置」（分组 × 模型 × 账号）改写结果。没有注册过滤器、
// 总开关关闭或分组没有调整时，原样返回，行为和上游完全一致。
//
//   - 候选名单（暂停 + 按比例抽签）：GatewayService.listSchedulableAccounts、OpenAIGatewayService.listSchedulableAccounts、
//     GeminiMessagesCompatService.listSchedulableAccountsOnce
//   - Claude legacy 粘性（拿不到模型，只认整个分组暂停）：GatewayService.isAccountInGroup
//   - 粘性会话（按模型认暂停，暂停的让它换号）：GeminiMessagesCompatService.tryStickySessionHit、
//     OpenAIGatewayService.getStickySessionAccountID（OpenAI 所有粘性路径都从这里读绑定）、
//     OpenAIGatewayService.resolveAccountByPreviousResponseIDForCapability（previous_response 续链）
//
// Claude 的粘性层只在候选名单里找账号，所以暂停由名单层处理；名单层收到 handler 预取的粘性账号（Bound）时不抽签，
// 老会话不会被比例打散。
//
// OpenAI 的「账号在不在分组」（openAIAccountMatchesSchedulingGroup）不挂钩子：它不只用在粘性上，选中后查库复核每个候选都会调，
// 在那里认暂停会把「只剩暂停的、照常保留」的候选全部拒掉（真实流量测出来的：两个账号都暂停时请求全部 503）。
// 新选号由名单层处理，粘住的会话由上面两个读绑定的钩子处理。
package service

import (
	"context"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// PlusDispatchQuery 一次选号请求。都从 ctx 里取，上游代码不用改。
type PlusDispatchQuery struct {
	GroupID int64
	Model   string // 客户端请求的模型（流量按「分组 × 模型」分配）
	Key     string // 请求 ID：同一个请求失败重试时相同（上游每次选号都会派生新的 ctx，不能拿 ctx 本身认）
	Bound   int64  // 会话已经粘住的账号（handler 预取的粘性账号，Claude / Gemini 入口会写）
}

// PlusDispatchFilter 由 plus/dispatch 实现
type PlusDispatchFilter interface {
	// FilterAccounts 返回每个候选是否保留；changed = false 表示原样放行。supports[i] = 第 i 个候选支不支持这个模型。
	FilterAccounts(ctx context.Context, q PlusDispatchQuery, ids []int64, supports []bool) (keep []bool, changed bool)
	// Paused 这个账号在这个分组的这个模型上是否被暂停（model 为空 = 不知道模型：只认整个分组暂停）
	Paused(groupID int64, model string, accountID int64) bool
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

// plusRequestModel 这次请求的模型：网关 handler 解析请求后放进了 ctx（ctxkey.Model）
func plusRequestModel(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if m, ok := ctx.Value(ctxkey.Model).(string); ok && m != "" {
		return m
	}
	m, _ := ctx.Value(ctxkey.RequestedPublicModel).(string)
	return m
}

func plusDispatchQuery(ctx context.Context, groupID *int64) PlusDispatchQuery {
	q := PlusDispatchQuery{GroupID: derefGroupID(groupID), Model: plusRequestModel(ctx)}
	if ctx == nil {
		return q
	}
	q.Key, _ = ctx.Value(ctxkey.RequestID).(string)
	q.Bound = prefetchedStickyAccountIDFromContext(ctx, groupID)
	return q
}

// plusApplyDispatch 改写候选名单。上游的名单可能来自共享快照，这里从不原地修改，只返回新切片。
// Gemini 显式缓存的请求只能去存缓存的那个账号，不改名单：它会先取一次名单算排除名单，抽签会让排除名单和后面的选号对不上，
// 请求直接失败；暂停时它也是「只有它能跑」的情况（这里拿不到是哪个账号，「一起停掉」对它不生效）。
func plusApplyDispatch(ctx context.Context, groupID *int64, accounts []Account) []Account {
	f := plusDispatchFilter()
	if f == nil || groupID == nil || *groupID <= 0 || len(accounts) == 0 || geminiCachedContentBoundFromContext(ctx) {
		return accounts
	}
	q := plusDispatchQuery(ctx, groupID)
	ids := make([]int64, len(accounts))
	supports := make([]bool, len(accounts))
	for i := range accounts {
		ids[i] = accounts[i].ID
		supports[i] = q.Model == "" || accounts[i].IsModelSupported(q.Model)
	}
	keep, changed := f.FilterAccounts(ctx, q, ids, supports)
	if !changed {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if keep[i] {
			out = append(out, accounts[i])
		}
	}
	return out
}

// plusDispatchPaused 账号在这次请求的线路（分组 × ctx 里的模型）上是否被暂停。ctx 为 nil 时只认整个分组暂停。
func plusDispatchPaused(ctx context.Context, groupID *int64, accountID int64) bool {
	f := plusDispatchFilter()
	return f != nil && groupID != nil && *groupID > 0 && accountID > 0 && f.Paused(*groupID, plusRequestModel(ctx), accountID)
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

// ---------- Claude legacy 粘性：账号在不在分组，拿不到模型，只认整个分组暂停 ----------

func (s *GatewayService) isAccountInGroup(account *Account, groupID *int64) bool {
	return s.isAccountInGroupUpstream(account, groupID) && !plusDispatchPaused(nil, groupID, account.ID)
}

// ---------- 粘性会话：绑定的账号在这个模型上被暂停时当作没绑定，走正常选号（名单层按模型处理，包括「只剩它、保留」），
// 选中新账号后上游会把会话改绑过去 ----------

func (s *GeminiMessagesCompatService) tryStickySessionHit(
	ctx context.Context,
	groupID *int64,
	sessionHash, cacheKey, requestedModel string,
	excludedIDs map[int64]struct{},
	platform string,
	useMixedScheduling bool,
) *Account {
	account := s.tryStickySessionHitUpstream(ctx, groupID, sessionHash, cacheKey, requestedModel, excludedIDs, platform, useMixedScheduling)
	if account != nil && plusDispatchPaused(ctx, groupID, account.ID) {
		return nil
	}
	return account
}

// getStickySessionAccountID OpenAI 所有粘性路径（legacy 负载感知 Layer 1、tryStickySessionHit、高级调度器、guardian、
// 利润门改绑判断）都从这里读会话绑定。ctx 里没有模型时只认整个分组暂停（和名单层一致）。
// 已知限制：线路上只剩被暂停的账号（照常保留）时，会话每次都重新选号，有两个以上这样的账号时会在它们之间换。
func (s *OpenAIGatewayService) getStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	accountID, err := s.getStickySessionAccountIDUpstream(ctx, groupID, sessionHash)
	if err == nil && accountID > 0 && plusDispatchPaused(ctx, groupID, accountID) {
		return 0, nil
	}
	return accountID, err
}

// resolveAccountByPreviousResponseIDForCapability previous_response 续链：和「整个分组暂停」一样，暂停后不再回到这个账号
// （续链断开由上游 previous_response_not_found 的恢复逻辑处理）。
func (s *OpenAIGatewayService) resolveAccountByPreviousResponseIDForCapability(
	ctx context.Context,
	groupID *int64,
	previousResponseID string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredCapability OpenAIEndpointCapability,
	requireCompact bool,
) (int64, *Account, string, OpenAIWSStateStore) {
	accountID, account, responseID, store := s.resolveAccountByPreviousResponseIDForCapabilityUpstream(ctx, groupID, previousResponseID, requestedModel, excludedIDs, requiredCapability, requireCompact)
	if accountID > 0 && plusDispatchPaused(ctx, groupID, accountID) {
		return 0, nil, "", nil
	}
	return accountID, account, responseID, store
}

// ---------- 账号模型判断（检测用） ----------

// PlusAccountUpstreamModel 按账号的模型映射换算出真正发给上游的模型名（账号直连检测用，和网关行为一致）
func PlusAccountUpstreamModel(platform, accountType string, credentials map[string]any, requestedModel string) string {
	a := &Account{Platform: platform, Type: accountType, Credentials: credentials}
	return a.GetMappedModel(requestedModel)
}

// PlusAccountSupportsModel 账号支不支持这个模型（和网关选号同一个判断：模型映射 / 透传 / 平台白名单）
func PlusAccountSupportsModel(platform, accountType string, credentials, extra map[string]any, model string) bool {
	a := &Account{Platform: platform, Type: accountType, Credentials: credentials, Extra: extra}
	return a.IsModelSupported(model)
}
