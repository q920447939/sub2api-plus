// Package hook — [plus] 上游调度台的执行层：在请求选号的热路径上按「权重设置」改写候选名单。
//
// 流量是在「分组里的一个模型」（一条线路）上分的。设置挂在 账号 × 分组 × 模型 上（模型为空 = 这个分组里它的全部模型，
// 同一个分组里模型上的设置优先）：
//   - Share = 0：暂停。从候选里拿掉；如果线路上只剩被暂停的账号，没勾「一起停掉」(Hard) 的照常保留
//   - 0 < Share ≤ 1：这条线路上给它多少。每个新请求抽一次签：
//     抽中设了比例的账号 → 这次只给它；抽到剩下的部分 → 只给没设比例的账号
//
// 设计约束：
//   - 热路径零 IO：规则表是不可变快照，由后台定时整体替换（atomic.Pointer）
//   - 只给新会话抽签：会话已经粘住某个账号（Request.Bound）且它没被暂停时不抽签，名单只去掉暂停的，
//     交给上游粘性逻辑命中它；不这样做，Claude 的粘性层只在名单里找账号，老会话会被抽签打散（缓存全丢）
//   - 同一个请求失败重试时不再抽签，只去掉暂停的账号，避免把重试堵死。认重试靠请求的 Done 通道或请求 ID（见 again）——
//     上游每次选号都会派生新的 ctx，不能拿 ctx 本身认
//   - 总开关关闭时直接返回，规则保留
//   - 不依赖上游 service 包，只处理账号 ID（由 service/plus_dispatch_hook.go 适配）
package hook

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Rule 某个账号在某个分组（某个模型）上的设置
type Rule struct {
	GroupID   int64
	AccountID int64
	Model     string  // 空 = 这个分组里它的全部模型
	Share     float64 // 0 = 暂停；(0,1] = 在这条线路上给它的比例
	Hard      bool    // 暂停时连「只有它能跑」的线路也停掉
}

type lane map[int64]Rule        // account → rule
type groupRules map[string]lane // model（"" = 全部模型）→ …
type table map[int64]groupRules // group → …

// Engine 规则快照
type Engine struct {
	enabled atomic.Bool
	rules   atomic.Pointer[table]
	rnd     func() float64
	seen    sync.Map // 已经取过名单的请求：Done 通道 → struct{}；请求 ID → 第一个带这个 ID 的请求的 Done 通道
}

// Request 一次选号请求
type Request struct {
	GroupID int64
	Model   string // 客户端请求的模型（流量按「分组 × 模型」分配）
	Key     string // 请求 ID：同一个请求失败重试时相同
	Bound   int64  // 会话已经粘住的账号（0 = 新会话或不知道）
}

// New 创建引擎（默认关闭、无规则）
func New() *Engine {
	e := &Engine{rnd: rand.Float64}
	empty := table{}
	e.rules.Store(&empty)
	return e
}

// SetRandom 单测注入随机源
func (e *Engine) SetRandom(f func() float64) { e.rnd = f }

// SetEnabled 总开关
func (e *Engine) SetEnabled(on bool) { e.enabled.Store(on) }

// Enabled 总开关状态
func (e *Engine) Enabled() bool { return e.enabled.Load() }

// Replace 整体替换规则表
func (e *Engine) Replace(rules []Rule) {
	t := table{}
	for _, r := range rules {
		if r.Share < 0 || r.Share > 1 || r.GroupID <= 0 || r.AccountID <= 0 {
			continue
		}
		g := t[r.GroupID]
		if g == nil {
			g = groupRules{}
			t[r.GroupID] = g
		}
		l := g[r.Model]
		if l == nil {
			l = lane{}
			g[r.Model] = l
		}
		l[r.AccountID] = r
	}
	e.rules.Store(&t)
}

func (e *Engine) group(groupID int64) groupRules {
	if groupID <= 0 || !e.enabled.Load() {
		return nil
	}
	t := e.rules.Load()
	if t == nil {
		return nil
	}
	return (*t)[groupID]
}

// ruleOf 模型上的设置优先于「全部模型」
func (g groupRules) ruleOf(model string, accountID int64) (Rule, bool) {
	if model != "" {
		if r, ok := g[model][accountID]; ok {
			return r, true
		}
	}
	r, ok := g[""][accountID]
	return r, ok
}

// Candidate 候选账号
type Candidate struct {
	ID       int64
	Supports bool // 这个账号支不支持这次请求的模型（不支持的上游后面会自己过滤掉，这里不让它们当「接收方」）
}

// Decision 改写结果。Keep[i] = 第 i 个候选是否保留；Changed = false 表示原样放行
type Decision struct {
	Keep    []bool
	Changed bool
}

// Filter 决定这次请求的候选名单。ctx 只用来在请求结束时清掉重试记录。
func (e *Engine) Filter(ctx context.Context, req Request, cands []Candidate) Decision {
	g := e.group(req.GroupID)
	if len(g) == 0 || len(cands) == 0 {
		return Decision{}
	}
	model := req.Model
	type cand struct {
		i    int
		rule Rule
		has  bool
	}
	var paused, targeted, free []cand
	touched := false
	for i, c := range cands {
		r, ok := g.ruleOf(model, c.ID)
		x := cand{i: i, rule: r, has: ok}
		switch {
		case !ok:
			if c.Supports {
				free = append(free, x)
			}
		case r.Share <= 0:
			touched = true
			paused = append(paused, x)
		default:
			touched = true
			if c.Supports {
				targeted = append(targeted, x)
			}
		}
	}
	if !touched {
		return Decision{}
	}
	keep := make([]bool, len(cands))
	// 不支持这个模型的账号原样留着（上游自己会过滤），暂停的拿掉
	for i, c := range cands {
		if !c.Supports {
			keep[i] = true
		}
	}
	set := func(cs []cand) {
		for _, c := range cs {
			keep[c.i] = true
		}
	}
	if len(targeted) == 0 && len(free) == 0 {
		// 线路上只剩被暂停的：没说一起停掉的照常保留
		for _, p := range paused {
			if !p.rule.Hard && cands[p.i].Supports {
				keep[p.i] = true
			}
		}
		return Decision{Keep: keep, Changed: true}
	}
	// 老会话（粘住的账号还在线路上、没被暂停）或失败重试：不再抽签，只去掉暂停的
	has := func(cs []cand, id int64) bool {
		for _, c := range cs {
			if cands[c.i].ID == id {
				return true
			}
		}
		return false
	}
	stuck := req.Bound > 0 && (has(targeted, req.Bound) || has(free, req.Bound))
	if stuck || e.again(ctx, req.Key) {
		set(targeted)
		set(free)
		return Decision{Keep: keep, Changed: true}
	}
	if len(targeted) == 0 {
		set(free)
		return Decision{Keep: keep, Changed: true}
	}
	sum := 0.0
	for _, t := range targeted {
		sum += t.rule.Share
	}
	r := e.rnd()
	scale := 1.0
	if sum > 1 || len(free) == 0 {
		scale = 1 / sum // 比例加起来超过 1，或者没有别的账号接剩下的：设了比例的之间按比例分
	} else if r >= sum {
		set(free) // 抽到剩下的部分：交给没设比例的账号
		return Decision{Keep: keep, Changed: true}
	}
	acc := 0.0
	for _, t := range targeted {
		acc += t.rule.Share * scale
		if r < acc {
			keep[t.i] = true
			return Decision{Keep: keep, Changed: true}
		}
	}
	keep[targeted[len(targeted)-1].i] = true
	return Decision{Keep: keep, Changed: true}
}

// again 这个请求是不是已经来取过名单（第二次来 = 上一个账号失败了在重试）。第一次来时记下，请求结束时清掉。
// 两种认法，认出一种就算：请求的 Done 通道（上游每次选号都会用 WithValue 派生新的 ctx，Done 通道不变）和请求 ID
// （上游以后要是在选号时派生了可取消的 ctx，Done 通道会变，请求 ID 不会）。宁可多认：把新请求认成重试只是这次不抽签；
// 把重试认成新请求，会抽中刚失败的账号、名单被清空，客户端直接拿到错误。
// 拿不到 Done 的 ctx（WithoutCancel 派生的探测）只按请求 ID 查、不记。
func (e *Engine) again(ctx context.Context, key string) bool {
	if ctx == nil {
		return false
	}
	done := ctx.Done()
	if done == nil {
		if key == "" {
			return false
		}
		_, ok := e.seen.Load(key)
		return ok
	}
	_, byDone := e.seen.LoadOrStore(done, struct{}{})
	byKey, ownKey := false, false
	if key != "" {
		_, byKey = e.seen.LoadOrStore(key, done)
		ownKey = !byKey
	}
	if !byDone || ownKey {
		context.AfterFunc(ctx, func() {
			e.seen.Delete(done)
			if ownKey {
				e.seen.CompareAndDelete(key, done) // 同一个请求 ID 后来被别的请求记下时不删它的
			}
		})
	}
	return byDone || byKey
}

// Paused 「账号在不在这个分组」的校验用：这个账号在这条线路上是否被暂停。
// model 为空（上游这条路径拿不到模型）时，只认「整个分组暂停」。
// 注意：OpenAI 选号会对每个候选都做这个校验，不只是粘性会话。如果这里把「在别的模型上暂停」也算进来，
// 抽签只留下它时名单会被清空、请求直接失败（真实流量测试里出现过：gpt-6-sol 暂停后 gpt-6.1-sol 抽中它的 39 次全部 503）。
// 模型上的暂停由拿得到模型的几层执行：候选名单、OpenAI 读粘性绑定 / previous_response 续链、Gemini 粘性命中
// （见 service/plus_dispatch_hook.go）。
func (e *Engine) Paused(groupID int64, model string, accountID int64) bool {
	g := e.group(groupID)
	if len(g) == 0 {
		return false
	}
	if model == "" {
		r, ok := g[""][accountID]
		return ok && r.Share <= 0
	}
	r, ok := g.ruleOf(model, accountID)
	return ok && r.Share <= 0
}

// FilterAccounts 按账号 ID 改写名单（dispatch 模块把它接到 service.PlusDispatchFilter）
func (e *Engine) FilterAccounts(ctx context.Context, req Request, ids []int64, supports []bool) ([]bool, bool) {
	cs := make([]Candidate, len(ids))
	for i, id := range ids {
		cs[i] = Candidate{ID: id, Supports: i >= len(supports) || supports[i]}
	}
	d := e.Filter(ctx, req, cs)
	return d.Keep, d.Changed
}
