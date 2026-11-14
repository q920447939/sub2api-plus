//go:build unit

package hook

import (
	"context"
	"testing"
	"time"
)

func cands(ids ...int64) []Candidate {
	out := make([]Candidate, len(ids))
	for i, id := range ids {
		out[i] = Candidate{ID: id, Supports: true}
	}
	return out
}

func kept(d Decision, cs []Candidate) []int64 {
	var out []int64
	for i, c := range cs {
		if d.Keep[i] {
			out = append(out, c.ID)
		}
	}
	return out
}

func eq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newReq 分组 1 上某个模型的一次新请求
func newReq(model string) Request { return Request{GroupID: 1, Model: model} }

func TestPauseDropsAndSoftPauseKeepsLastOne(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0}})
	cs := cands(10, 11)
	if got := kept(e.Filter(nil, newReq("m"), cs), cs); !eq(got, []int64{11}) {
		t.Fatalf("paused account must be dropped: %v", got)
	}
	only := cands(10)
	if got := kept(e.Filter(nil, newReq("m"), only), only); !eq(got, []int64{10}) {
		t.Fatalf("soft pause keeps the only account: %v", got)
	}
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0, Hard: true}})
	if got := kept(e.Filter(nil, newReq("m"), only), only); len(got) != 0 {
		t.Fatalf("hard pause empties the lane: %v", got)
	}
}

func TestShareDraw(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.6}})
	cs := cands(10, 11, 12)
	e.SetRandom(func() float64 { return 0.5 })
	if got := kept(e.Filter(nil, newReq("m"), cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("draw < share picks the account: %v", got)
	}
	e.SetRandom(func() float64 { return 0.7 })
	if got := kept(e.Filter(nil, newReq("m"), cs), cs); !eq(got, []int64{11, 12}) {
		t.Fatalf("draw ≥ share goes to the others: %v", got)
	}
}

func TestModelRuleBeatsAllModels(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0}, {GroupID: 1, AccountID: 10, Model: "a", Share: 1}})
	cs := cands(10, 11)
	e.SetRandom(func() float64 { return 0.9 })
	if got := kept(e.Filter(nil, newReq("a"), cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("model rule wins: %v", got)
	}
	if got := kept(e.Filter(nil, newReq("b"), cs), cs); !eq(got, []int64{11}) {
		t.Fatalf("all-models pause applies to other models: %v", got)
	}
	if !e.Paused(1, "b", 10) || e.Paused(1, "a", 10) {
		t.Fatal("paused lookup")
	}
}

func TestUnsupportedAccountsAreNotReceivers(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.3}})
	cs := []Candidate{{ID: 10, Supports: true}, {ID: 11, Supports: false}}
	e.SetRandom(func() float64 { return 0.9 })
	// 没有能接的账号：设了比例的那个拿走全部
	if got := kept(e.Filter(nil, newReq("m"), cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("got %v", got)
	}
}

type ctxKey struct{}

// 真实流量测试发现的问题：上游每次选号都会用 WithValue 派生新的 ctx，按 ctx 本身认重试永远认不出来，
// 重试又抽中刚失败（已被排除）的账号，名单被清空，客户端直接拿到 502。改成按 Done 通道 / 请求 ID 认。
func TestRetryIsRecognisedAcrossDerivedContexts(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.6}, {GroupID: 1, AccountID: 12, Share: 0}})
	base, cancel := context.WithCancel(context.Background())
	cs := cands(10, 11, 12)
	e.SetRandom(func() float64 { return 0.1 })
	req := Request{GroupID: 1, Model: "m", Key: "req-1"}
	if got := kept(e.Filter(context.WithValue(base, ctxKey{}, 1), req, cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("first call draws: %v", got)
	}
	if got := kept(e.Filter(context.WithValue(base, ctxKey{}, 2), req, cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("retry with a new derived ctx keeps everyone except paused: %v", got)
	}
	other, cancelOther := context.WithCancel(context.Background())
	defer cancelOther()
	if got := kept(e.Filter(other, Request{GroupID: 1, Model: "m", Key: "req-2"}, cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("another request draws again: %v", got)
	}
	cancel()
	waitGone(t, e, "req-1")
	waitGone(t, e, base.Done())
	if got := kept(e.Filter(context.WithValue(context.Background(), ctxKey{}, 3), req, cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("records are cleared when the request ends: %v", got)
	}
}

func TestRetryWithoutRequestKeyUsesDoneChannel(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.6}})
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	cs := cands(10, 11)
	e.SetRandom(func() float64 { return 0.1 })
	if got := kept(e.Filter(context.WithValue(base, ctxKey{}, 1), newReq("m"), cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("first call draws: %v", got)
	}
	if got := kept(e.Filter(context.WithValue(base, ctxKey{}, 2), newReq("m"), cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("same Done channel = retry: %v", got)
	}
}

// 上游以后要是在选号时派生了可取消的 ctx，Done 通道会变，请求 ID 不会：照样认成重试
func TestRetryWithANewCancelableContextIsRecognisedByRequestKey(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.6}})
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	cs := cands(10, 11)
	e.SetRandom(func() float64 { return 0.1 })
	req := Request{GroupID: 1, Model: "m", Key: "req-4"}
	e.Filter(base, req, cs)
	inner, cancelInner := context.WithTimeout(base, time.Minute)
	defer cancelInner()
	if got := kept(e.Filter(inner, req, cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("retry on a new Done channel: %v", got)
	}
}

// 请求 ID 客户端可以自己带（X-Request-ID）。另一个请求带了同一个 ID 时宁可多认（这次不抽签），
// 它先结束也不能把原请求的记录清掉
func TestSameRequestKeyFromAnotherRequest(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.6}})
	cs := cands(10, 11)
	e.SetRandom(func() float64 { return 0.1 })
	a, cancelA := context.WithCancel(context.Background())
	b, cancelB := context.WithCancel(context.Background())
	req := Request{GroupID: 1, Model: "m", Key: "client-id"}
	e.Filter(a, req, cs)
	if got := kept(e.Filter(b, req, cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("another request reusing the id is treated as a retry: %v", got)
	}
	cancelB()
	waitGone(t, e, b.Done())
	if got := kept(e.Filter(context.WithoutCancel(a), req, cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("the first request's record survives the other request ending: %v", got)
	}
	cancelA()
	waitGone(t, e, "client-id")
}

// 拿不到 Done 的 ctx（WithoutCancel 派生的探测）只按请求 ID 查、不记：没来过就照常抽签，也不会留下清不掉的记录
func TestContextWithoutCancelOnlyLooksUp(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.6}})
	cs := cands(10, 11)
	e.SetRandom(func() float64 { return 0.1 })
	probe := context.WithoutCancel(context.Background())
	req := Request{GroupID: 1, Model: "m", Key: "probe"}
	for i := 0; i < 2; i++ {
		if got := kept(e.Filter(probe, req, cs), cs); !eq(got, []int64{10}) {
			t.Fatalf("probe call %d: %v", i, got)
		}
	}
	if _, ok := e.seen.Load("probe"); ok {
		t.Fatal("no record without a Done channel")
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	req.Key = "req-3"
	e.Filter(base, req, cs)
	if got := kept(e.Filter(context.WithoutCancel(base), req, cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("probe after the request's first call: %v", got)
	}
}

// 真实流量测试发现的问题：Claude 的粘性层只在名单里找账号，老会话每次都被重新抽签，在账号之间来回跳（缓存全丢）。
func TestBoundSessionIsNotRedrawn(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0.5}, {GroupID: 1, AccountID: 12, Share: 0}})
	cs := cands(10, 11, 12)
	e.SetRandom(func() float64 { return 0.1 }) // 新会话会抽中 10
	if got := kept(e.Filter(nil, Request{GroupID: 1, Model: "m", Bound: 11}, cs), cs); !eq(got, []int64{10, 11}) {
		t.Fatalf("old session on 11 keeps everyone except paused: %v", got)
	}
	if got := kept(e.Filter(nil, Request{GroupID: 1, Model: "m", Bound: 12}, cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("old session on a paused account is drawn like a new one: %v", got)
	}
	if got := kept(e.Filter(nil, Request{GroupID: 1, Model: "m", Bound: 99}, cs), cs); !eq(got, []int64{10}) {
		t.Fatalf("bound account not on this lane: %v", got)
	}
	unsupported := []Candidate{{ID: 10, Supports: true}, {ID: 11, Supports: false}, {ID: 13, Supports: true}}
	if got := kept(e.Filter(nil, Request{GroupID: 1, Model: "m", Bound: 11}, unsupported), unsupported); !eq(got, []int64{10, 11}) {
		t.Fatalf("bound account that does not support the model is drawn like a new one: %v", got)
	}
}

func TestDisabledAndOtherGroups(t *testing.T) {
	e := New()
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Share: 0}})
	if e.Filter(nil, newReq("m"), cands(10, 11)).Changed || e.Paused(1, "m", 10) {
		t.Fatal("disabled engine must not filter")
	}
	e.SetEnabled(true)
	if e.Filter(nil, Request{GroupID: 2, Model: "m"}, cands(10, 11)).Changed {
		t.Fatal("rule must be scoped to its group")
	}
}

// 真实流量测试发现的问题：账号只在某个模型上暂停时，拿不到模型的分组校验（OpenAI 每个候选都会调）不能把它当成暂停，
// 否则另一个模型抽签只留下它时名单被清空，请求全部失败。
func TestModelPauseDoesNotLeakIntoModellessCheck(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{
		{GroupID: 5, AccountID: 1, Share: 0.2},
		{GroupID: 5, AccountID: 1, Model: "gpt-6.1-sol", Share: 0.8},
		{GroupID: 5, AccountID: 1, Model: "gpt-6-sol", Share: 0},
	})
	if e.Paused(5, "", 1) {
		t.Fatal("只在 gpt-6-sol 上暂停，不知道模型时不能算暂停")
	}
	if !e.Paused(5, "gpt-6-sol", 1) || e.Paused(5, "gpt-6.1-sol", 1) {
		t.Fatal("知道模型时按模型判断")
	}
	e.Replace([]Rule{{GroupID: 5, AccountID: 1, Share: 0}})
	if !e.Paused(5, "", 1) {
		t.Fatal("整个分组暂停时，不知道模型也算暂停")
	}
}

// waitGone 等请求结束后的清理回调跑完（context.AfterFunc 在另一个 goroutine 里执行）
func waitGone(t *testing.T, e *Engine, key any) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if _, ok := e.seen.Load(key); !ok {
			return
		}
	}
	t.Fatalf("record %v not cleared", key)
}
