//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/plus/dispatch/hook"
	"github.com/stretchr/testify/require"
)

// plusHookAdapter 和 plus/dispatch 模块里的转接一样：用真实的钩子引擎
type plusHookAdapter struct{ *hook.Engine }

func (a plusHookAdapter) FilterAccounts(ctx context.Context, q PlusDispatchQuery, ids []int64, supports []bool) ([]bool, bool) {
	return a.Engine.FilterAccounts(ctx, hook.Request{GroupID: q.GroupID, Model: q.Model, Key: q.Key, Bound: q.Bound}, ids, supports)
}

func usePlusRules(t *testing.T, rules ...hook.Rule) {
	t.Helper()
	e := hook.New()
	e.SetEnabled(true)
	e.Replace(rules)
	SetPlusDispatchFilter(plusHookAdapter{e})
	t.Cleanup(func() { plusDispatch.Store(nil) })
}

func withModel(model string) context.Context {
	return context.WithValue(context.Background(), ctxkey.Model, model)
}

// 真实流量测试发现的问题：OpenAI legacy 负载感知选号的粘性层自己读绑定、只认整个分组暂停，
// gpt-6-sol 在 xg-gpt-1 上暂停后，撞上老会话的请求照样落到 xg-gpt-1。
func TestPlusDispatchOpenAIStickyPausedOnModelMovesSession(t *testing.T) {
	groupID := int64(9105)
	usePlusRules(t,
		hook.Rule{GroupID: groupID, AccountID: 1, Model: "gpt-6-sol", Share: 0},
		hook.Rule{GroupID: groupID, AccountID: 1, Model: "gpt-6.1-sol", Share: 0.8},
	)
	repo := groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, AccountGroups: []AccountGroup{{GroupID: groupID}}},
		{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, AccountGroups: []AccountGroup{{GroupID: groupID}}},
	}}}
	for _, tc := range []struct {
		model string
		want  int64
	}{
		{"gpt-6-sol", 2},   // 绑定的账号在这个模型上暂停：换号，并改绑到接住它的账号
		{"gpt-6.1-sol", 1}, // 只是调了比例：老会话照常粘住
		{"", 1},            // 不知道模型：只认整个分组暂停
	} {
		cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:sess": 1}}
		svc := &OpenAIGatewayService{accountRepo: repo, cache: cache, concurrencyService: NewConcurrencyService(stubConcurrencyCache{})}
		sel, err := svc.SelectAccountWithLoadAwareness(withModel(tc.model), &groupID, "sess", tc.model, nil)
		require.NoError(t, err, tc.model)
		require.NotNil(t, sel)
		require.Equal(t, tc.want, sel.Account.ID, tc.model)
		require.Equal(t, tc.want, cache.sessionBindings["openai:sess"], "%s: session must stay on / move to the serving account", tc.model)
		if sel.ReleaseFunc != nil {
			sel.ReleaseFunc()
		}
	}
}

// 真实流量测试发现的问题：OpenAI 的「账号在不在分组」不只用在粘性上，选中后查库复核每个候选都会调。之前在那里认暂停，
// 名单层「只剩暂停的、照常保留」留下的候选在复核时全部被拒，两个账号都暂停（没勾一起停掉）时请求全部 503。
func TestPlusDispatchOpenAISoftPausedLastAccountsStillServe(t *testing.T) {
	groupID := int64(9108)
	usePlusRules(t,
		hook.Rule{GroupID: groupID, AccountID: 1, Share: 0},
		hook.Rule{GroupID: groupID, AccountID: 2, Share: 0},
	)
	a1 := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, AccountGroups: []AccountGroup{{GroupID: groupID}}}
	a2 := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, AccountGroups: []AccountGroup{{GroupID: groupID}}}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*a1, *a2}},
		cache:              &schedulerTestGatewayCache{},
		schedulerSnapshot:  &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{snapshotAccounts: []*Account{a1, a2}, accountsByID: map[int64]*Account{1: a1, 2: a2}}},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	sel, err := svc.SelectAccountWithLoadAwareness(withModel("gpt-6-sol"), &groupID, "", "gpt-6-sol", nil)
	require.NoError(t, err)
	require.NotNil(t, sel)
	require.NotNil(t, sel.Account)
	if sel.ReleaseFunc != nil {
		sel.ReleaseFunc()
	}
}

func TestPlusDispatchQueryFromContext(t *testing.T) {
	g := int64(9106)
	ctx := context.WithValue(withModel("claude-x"), ctxkey.RequestID, "rid-1")
	require.Equal(t, PlusDispatchQuery{GroupID: g, Model: "claude-x", Key: "rid-1"}, plusDispatchQuery(ctx, &g))

	ctx = WithPrefetchedStickySession(ctx, 8, g, false)
	q := plusDispatchQuery(ctx, &g)
	require.Equal(t, int64(8), q.Bound)
	other := g + 1
	require.Zero(t, plusDispatchQuery(ctx, &other).Bound, "prefetch for another group is ignored")
	require.Equal(t, PlusDispatchQuery{}, plusDispatchQuery(nil, nil))
}

// 真实流量测试发现的问题：Claude 的粘性层只在名单里找账号，老会话每次被重新抽签、在账号之间来回跳；
// 换号重试时又抽中刚失败的账号，名单被清空，客户端直接拿到 502。
func TestPlusDispatchBoundSessionAndRetryAreNotRedrawn(t *testing.T) {
	g := int64(9106)
	usePlusRules(t, hook.Rule{GroupID: g, AccountID: 7, Share: 0.5})
	accounts := []Account{{ID: 7, Platform: PlatformAnthropic}, {ID: 8, Platform: PlatformAnthropic}}
	base := withModel("claude-haiku-5-5")

	require.Len(t, plusApplyDispatch(base, &g, accounts), 1, "new session is drawn")
	require.Len(t, plusApplyDispatch(WithPrefetchedStickySession(base, 8, g, false), &g, accounts), 2, "old session keeps everyone so the sticky layer finds 8")

	// 换号重试：handler 在同一个请求 ctx 上再派生（写换号次数、改分组上下文……）后重新选号
	req, cancel := context.WithCancel(context.WithValue(base, ctxkey.RequestID, "rid-2"))
	defer cancel()
	require.Len(t, plusApplyDispatch(context.WithValue(req, ctxkey.Platform, "a"), &g, accounts), 1)
	require.Len(t, plusApplyDispatch(WithAccountSwitchCount(req, 1, false), &g, accounts), 2, "retry keeps everyone")
}

// Gemini 显式缓存的请求先取一次名单算排除名单，再选号；抽签会让两次对不上，请求只能去存缓存的账号，不改名单
func TestPlusDispatchSkipsGeminiCachedContentRequests(t *testing.T) {
	g := int64(9107)
	usePlusRules(t, hook.Rule{GroupID: g, AccountID: 7, Share: 0.5}, hook.Rule{GroupID: g, AccountID: 8, Share: 0})
	accounts := []Account{{ID: 7, Platform: PlatformGemini}, {ID: 8, Platform: PlatformGemini}, {ID: 9, Platform: PlatformGemini}}
	require.Len(t, plusApplyDispatch(WithGeminiCachedContentBound(withModel("gemini-x")), &g, accounts), 3)
	require.Len(t, plusApplyDispatch(withModel("gemini-x"), &g, accounts), 1, "other requests are still drawn")
}

func TestPlusDispatchPreviousResponsePausedOnModel(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9109)
	account := Account{
		ID: 1001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2,
		Extra: map[string]any{"openai_apikey_responses_websockets_v2_enabled": true},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = 3600
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, "resp_1", account.ID, time.Hour))
	resolve := func(model string) int64 {
		id, _, _, _ := svc.resolveAccountByPreviousResponseIDForCapability(withModel(model), &groupID, "resp_1", model, nil, "", false)
		return id
	}
	require.Equal(t, account.ID, resolve("gpt-5.1"), "no rules: continuation goes back to its account")
	usePlusRules(t, hook.Rule{GroupID: groupID, AccountID: account.ID, Model: "gpt-5.1", Share: 0})
	require.Zero(t, resolve("gpt-5.1"), "paused on this model: continuation leaves the account, like a whole-group pause")
	require.Equal(t, account.ID, resolve("gpt-5.2"), "other models are not affected")
}
