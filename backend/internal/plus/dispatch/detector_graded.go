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

// GradedRule 判分题的判定规则：最近 window 次有效回答里答对少于 bad_below 次 → 未通过（没攒满时按比例折算）
type GradedRule struct {
	WindowRule
	BadBelow int `json:"bad_below"`
}

func (r *GradedRule) normalize() error {
	r.WindowRule.normalize(5)
	if r.BadBelow <= 0 {
		r.BadBelow = 3
	}
	if r.BadBelow > r.Window {
		return fmt.Errorf("%w：答对次数的下限不能超过看的次数", ErrInvalid)
	}
	return nil
}

// GradedDetector 判分题：发一道有标准答案的题，从回答里提取答案并自动判对错
type GradedDetector struct{ LLM Completer }

var _ Detector = GradedDetector{}

func (GradedDetector) Spec() TypeSpec {
	return TypeSpec{
		Key: "graded", Name: "判分题", Judges: true, HasPrompt: true, HasAnswers: true, DefaultEvery: 60,
		Description: "发一道有标准答案的题，从回答里提取答案并自动判对错。",
		RuleText:    "最近 {window} 次有效回答里答对少于 {bad_below} 次算未通过，满 {min_valid} 次才下结论",
		RuleFields:  []RuleField{{Key: "window", Min: 1, Max: 50}, {Key: "bad_below", Min: 1, Max: 50}, {Key: "min_valid", Min: 1, Max: 50}},
		Outcomes: []OutcomeSpec{
			{Key: OutcomePass, Label: "答对", Tone: "ok"},
			{Key: OutcomeFail, Label: "答错", Tone: "bad"},
			{Key: OutcomeIgnored, Label: "已标误报", Tone: "few"},
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

func (GradedDetector) Judge(item Item, rs []Run) *Verdict {
	var r GradedRule
	_ = json.Unmarshal(item.Rule, &r)
	_ = r.normalize()
	valid, ignored := validRuns(rs, r.Window)
	if len(valid) < r.MinValid {
		v := fewVerdict(len(valid), r.MinValid)
		v.Ignored = ignored > 0
		return v
	}
	ok := 0
	for _, x := range valid {
		if x.Outcome == OutcomePass {
			ok++
		}
	}
	n := len(valid)
	v := &Verdict{Valid: n, Bad: n - ok, Ignored: ignored > 0}
	// 阈值按「最近 window 次」定；还没攒满时按比例折算，否则攒满前永远够不到线
	if float64(ok) < float64(r.BadBelow)*float64(n)/float64(r.Window) {
		v.Level, v.Short, v.Long = LevelBad, fmt.Sprintf("%d 次只答对 %d 次", n, ok), fmt.Sprintf("最近 %d 次有效回答里只答对 %d 次", n, ok)
	} else {
		v.Level, v.Short, v.Long = LevelOK, fmt.Sprintf("%d 次答对 %d 次", n, ok), fmt.Sprintf("最近 %d 次有效回答里答对 %d 次", n, ok)
	}
	return v
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
