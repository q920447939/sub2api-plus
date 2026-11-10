package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ---------- 判分方式（Strategy） ----------

// Matcher 从回答里提取答案并判对错
type Matcher interface {
	Key() string
	Name() string
	Grade(reply string, answers []string) (got string, ok bool)
}

var (
	numRe     = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	boldNumRe = regexp.MustCompile(`\*\*\s*[^*\d-]*(-?\d+(?:\.\d+)?)[^*]*\*\*`)
	boxedRe   = regexp.MustCompile(`\\boxed\{\s*(-?\d+(?:\.\d+)?)\s*\}`)
)

type numMatcher struct{}

func (numMatcher) Key() string  { return "num" }
func (numMatcher) Name() string { return "数值相等" }

// 优先取 \boxed{}、加粗里的数字，否则取最后一个数字
func (numMatcher) Grade(reply string, answers []string) (string, bool) {
	got := ""
	if m := boxedRe.FindAllStringSubmatch(reply, -1); len(m) > 0 {
		got = m[len(m)-1][1]
	} else if m := boldNumRe.FindAllStringSubmatch(reply, -1); len(m) > 0 {
		got = m[len(m)-1][1]
	} else if m := numRe.FindAllString(reply, -1); len(m) > 0 {
		got = m[len(m)-1]
	}
	if got == "" {
		return "", false
	}
	g, err := strconv.ParseFloat(got, 64)
	if err != nil {
		return got, false
	}
	for _, a := range answers {
		if v, err := strconv.ParseFloat(strings.TrimSpace(a), 64); err == nil && math.Abs(v-g) < 1e-9 {
			return got, true
		}
	}
	return got, false
}

type exactMatcher struct{}

func (exactMatcher) Key() string  { return "exact" }
func (exactMatcher) Name() string { return "完全一致" }
func (exactMatcher) Grade(reply string, answers []string) (string, bool) {
	t := strings.TrimSpace(reply)
	for _, a := range answers {
		if t == strings.TrimSpace(a) {
			return t, true
		}
	}
	return truncateRunes(t, 80), false
}

type containsMatcher struct{}

func (containsMatcher) Key() string  { return "contains" }
func (containsMatcher) Name() string { return "包含" }
func (containsMatcher) Grade(reply string, answers []string) (string, bool) {
	for _, a := range answers {
		if a = strings.TrimSpace(a); a != "" && strings.Contains(reply, a) {
			return a, true
		}
	}
	return "", false
}

type regexMatcher struct{}

func (regexMatcher) Key() string  { return "regex" }
func (regexMatcher) Name() string { return "正则匹配" }
func (regexMatcher) Grade(reply string, answers []string) (string, bool) {
	for _, a := range answers {
		re, err := regexp.Compile(a)
		if err != nil {
			continue
		}
		if m := re.FindString(reply); m != "" {
			return m, true
		}
	}
	return "", false
}

var matchers = []Matcher{numMatcher{}, exactMatcher{}, containsMatcher{}, regexMatcher{}}

func matcherByKey(k string) (Matcher, bool) {
	for _, m := range matchers {
		if m.Key() == k {
			return m, true
		}
	}
	return nil, false
}

// MatcherOptions 给前端的判分方式列表
func MatcherOptions() []map[string]string {
	out := make([]map[string]string, 0, len(matchers))
	for _, m := range matchers {
		out = append(out, map[string]string{"key": m.Key(), "name": m.Name()})
	}
	return out
}

// ---------- 判分题 ----------

// GradedParams 判分题参数（检测项上的默认值；计划可以整体覆盖 prompt / answers / match）
type GradedParams struct {
	Prompt    string   `json:"prompt"`
	Answers   []string `json:"answers"`
	Match     string   `json:"match"`
	MaxTokens int      `json:"max_tokens"`
}

func (p *GradedParams) normalize() error {
	p.Prompt = strings.TrimSpace(p.Prompt)
	if p.Prompt == "" {
		return fmt.Errorf("%w：题面不能为空", ErrInvalid)
	}
	ans := p.Answers[:0]
	for _, a := range p.Answers {
		if a = strings.TrimSpace(a); a != "" {
			ans = append(ans, a)
		}
	}
	p.Answers = ans
	if len(p.Answers) == 0 {
		return fmt.Errorf("%w：至少要有一个标准答案", ErrInvalid)
	}
	if p.Match == "" {
		p.Match = "num"
	}
	if _, ok := matcherByKey(p.Match); !ok {
		return fmt.Errorf("%w：不认识的判分方式 %q", ErrInvalid, p.Match)
	}
	if p.Match == "regex" {
		for _, a := range p.Answers {
			if _, err := regexp.Compile(a); err != nil {
				return fmt.Errorf("%w：正则 %q 写法有误", ErrInvalid, a)
			}
		}
	}
	if p.MaxTokens <= 0 {
		p.MaxTokens = 8000
	}
	return nil
}

// GradedRule 判分题的判定规则
type GradedRule struct {
	Window    int    `json:"window"`     // 看最近几次
	How       string `json:"how"`        // fixed（固定次数）| peer（和同组同模型其他账号比）
	WarnBelow int    `json:"warn_below"` // 答对少于几次记可疑
	BadBelow  int    `json:"bad_below"`
	PeerWarn  int    `json:"peer_warn"` // 比同组平均少几次记可疑
	PeerBad   int    `json:"peer_bad"`
}

func (r *GradedRule) normalize() error {
	if r.Window <= 0 {
		r.Window = 10
	}
	if r.Window > 50 {
		r.Window = 50
	}
	if r.How != "peer" {
		r.How = "fixed"
	}
	if r.WarnBelow == 0 && r.BadBelow == 0 {
		r.WarnBelow, r.BadBelow = 6, 3
	}
	if r.PeerWarn == 0 && r.PeerBad == 0 {
		r.PeerWarn, r.PeerBad = 3, 5
	}
	if r.BadBelow > r.WarnBelow || r.PeerBad < r.PeerWarn {
		return fmt.Errorf("%w：「异常」的阈值要比「可疑」更严", ErrInvalid)
	}
	return nil
}

// GradedDetector 判分题：发一道有标准答案的题，从回答里提取答案并自动判对错
type GradedDetector struct{ LLM Completer }

var _ ActiveDetector = GradedDetector{}

func (GradedDetector) Spec() TypeSpec {
	return TypeSpec{
		Key: "graded", Name: "判分题", Active: true, Judges: true, Comparable: true,
		Description: "发一道有标准答案的题，从回答里提取答案并自动判对错。",
		Supports:    "所有平台",
		JudgeDesc:   "近 N 次答对次数（固定次数，或和同组同模型其他账号比）",
		PlanFields:  []string{"models", "pick", "every", "timeout"},
		Outcomes: []OutcomeSpec{
			{Key: OutcomePass, Label: "答对", Tone: "ok"},
			{Key: OutcomeFail, Label: "答错", Tone: "bad"},
			{Key: OutcomeError, Label: "请求出错", Tone: "err"},
		},
	}
}

func (GradedDetector) NormalizeItem(params, rule json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var p GradedParams
	if err := decodeInto(params, &p); err != nil {
		return nil, nil, err
	}
	if err := p.normalize(); err != nil {
		return nil, nil, err
	}
	var r GradedRule
	if err := decodeInto(rule, &r); err != nil {
		return nil, nil, err
	}
	if err := r.normalize(); err != nil {
		return nil, nil, err
	}
	return mustJSON(p), mustJSON(r), nil
}

func (GradedDetector) Supports(context.Context, string, string) (bool, string) { return true, "" }

// gradedParams 题目参数（计划不再改题，直接用检测项上的题目）
func gradedParams(item Item, _ *Plan) GradedParams {
	var p GradedParams
	_ = json.Unmarshal(item.Params, &p)
	_ = p.normalize()
	return p
}

func (d GradedDetector) Execute(ctx context.Context, in ExecInput) ExecResult {
	p := gradedParams(in.Item, in.Plan)
	comp, err := d.LLM.Complete(ctx, in.Target, p.Prompt, p.MaxTokens)
	if err != nil {
		r := errResult(p.Prompt, err)
		r.Reply = truncateRunes(comp.Text, 4000)
		return r
	}
	m, _ := matcherByKey(p.Match)
	got, ok := m.Grade(comp.Text, p.Answers)
	res := ExecResult{
		Prompt: p.Prompt, Reply: truncateRunes(comp.Text, 20000),
		InputTokens: comp.InputTokens, OutputTokens: comp.OutputTokens,
		Detail: map[string]any{"answer": got, "expected": p.Answers, "match": p.Match},
	}
	if ok {
		res.Outcome, res.Summary = OutcomePass, "答对："+got
	} else {
		res.Outcome = OutcomeFail
		if got == "" {
			res.Summary = "答错：没提取到答案"
		} else {
			res.Summary = fmt.Sprintf("答错：答 %s（标准 %s）", got, strings.Join(p.Answers, " / "))
		}
	}
	return res
}

func (GradedDetector) Classify(_ Item, r Run) string { return r.Outcome }

// TryGrade 试判：贴一段回答，看会提取到什么、判对还是判错
func TryGrade(params GradedParams, reply string) (string, bool, error) {
	if err := params.normalize(); err != nil {
		return "", false, err
	}
	m, _ := matcherByKey(params.Match)
	got, ok := m.Grade(reply, params.Answers)
	return got, ok, nil
}

type gradedCount struct {
	n, pass int
}

func countGraded(rs []Run, window int) gradedCount {
	var c gradedCount
	for _, r := range rs {
		if r.Outcome != OutcomePass && r.Outcome != OutcomeFail {
			continue
		}
		c.n++
		if r.Outcome == OutcomePass {
			c.pass++
		}
		if c.n >= window {
			break
		}
	}
	return c
}

func (GradedDetector) Judge(env *JudgeEnv, item Item, s Subject) Verdict {
	var r GradedRule
	_ = json.Unmarshal(item.Rule, &r)
	_ = r.normalize()
	rs := env.RunsOf(item.ID, s.CredKey, s.Model)
	if len(rs) == 0 {
		return Verdict{Level: LevelNone, Short: "还没执行", Long: "计划已覆盖，还没执行过"}
	}
	me := countGraded(rs, r.Window)
	need := (r.Window + 1) / 2
	ev := map[string]any{"window": r.Window, "n": me.n, "pass": me.pass, "errors": leadingErrors(rs)}
	if me.n < need {
		return Verdict{Level: LevelFew, Short: fmt.Sprintf("%d 次有效", me.n), Evidence: ev,
			Long: fmt.Sprintf("有效回答 %d 次，满 %d 次后判定%s", me.n, need, staleNote(rs))}
	}
	// 同组同模型其他账号（按凭证去重）
	type peer struct {
		AccountID int64   `json:"account_id"`
		N         int     `json:"n"`
		Pass      int     `json:"pass"`
		Scaled    float64 `json:"-"`
	}
	var peers []peer
	seen := map[string]bool{s.CredKey: true}
	if g := env.Catalog.Groups[s.GroupID]; g != nil {
		for _, id := range g.Accounts {
			a := env.Catalog.Accounts[id]
			if a == nil || seen[a.CredKey] {
				continue
			}
			seen[a.CredKey] = true
			c := countGraded(env.RunsOf(item.ID, a.CredKey, s.Model), r.Window)
			if c.n >= need {
				peers = append(peers, peer{AccountID: id, N: c.n, Pass: c.pass, Scaled: float64(c.pass) / float64(c.n) * float64(me.n)})
			}
		}
	}
	var avg *float64
	if len(peers) > 0 {
		sum := 0.0
		for _, p := range peers {
			sum += p.Scaled
		}
		avg = fptr(sum / float64(len(peers)))
	}
	ev["peers"] = peers
	var lv Level
	var basis string
	// 阈值是按「近 Window 次」定的；有效回答还没攒满窗口时按比例换算，
	// 否则攒满前最多只能答对 n 次，永远够不到线（例如 5 次全对也会低于「10 次里少于 6 次」）
	scale := float64(me.n) / float64(r.Window)
	full := me.n >= r.Window
	line := func(v int) float64 { return float64(v) * scale }
	if r.How == "peer" && avg != nil {
		d := *avg - float64(me.pass)
		lv = pick(d >= line(r.PeerBad), d >= line(r.PeerWarn))
		if full {
			basis = fmt.Sprintf("比同组平均少 %.1f 次；少 %d 次记可疑，少 %d 次记异常", math.Max(0, d), r.PeerWarn, r.PeerBad)
		} else {
			basis = fmt.Sprintf("比同组平均少 %.1f 次；按 %d 次换算，少 %.1f 次记可疑，少 %.1f 次记异常", math.Max(0, d), me.n, line(r.PeerWarn), line(r.PeerBad))
		}
	} else {
		lv = pick(float64(me.pass) < line(r.BadBelow), float64(me.pass) < line(r.WarnBelow))
		if full {
			basis = fmt.Sprintf("少于 %d 次记可疑，少于 %d 次记异常", r.WarnBelow, r.BadBelow)
		} else {
			basis = fmt.Sprintf("规则是近 %d 次里少于 %d 次记可疑、少于 %d 次记异常，按 %d 次换算是少于 %.1f / %.1f 次", r.Window, r.WarnBelow, r.BadBelow, me.n, line(r.WarnBelow), line(r.BadBelow))
		}
	}
	long := fmt.Sprintf("近 %d 次答对 %d 次", me.n, me.pass)
	if avg != nil {
		long += fmt.Sprintf("，同组同模型其他账号平均 %.1f 次", *avg)
	}
	long += "（" + basis + "）" + staleNote(rs)
	return Verdict{Level: lv, Value: fptr(float64(me.pass) / float64(me.n) * 100), Ref: avg,
		Short: fmt.Sprintf("%d/%d 答对", me.pass, me.n), Long: long, Evidence: ev}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
