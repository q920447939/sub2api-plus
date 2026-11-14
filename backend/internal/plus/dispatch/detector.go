package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// OutcomeSpec 单次执行可能的结果（由检测方式声明，检测记录页的筛选、计数、颜色都读它）
type OutcomeSpec struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Tone  string `json:"tone"` // ok | bad | few | err | info
}

// RuleField 判定规则里的一个数字（编辑弹层按它渲染，代码里不写死字段）
type RuleField struct {
	Key  string  `json:"key"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Unit string  `json:"unit,omitempty"`
}

// TypeSpec 检测方式的能力声明
type TypeSpec struct {
	Key          string        `json:"key"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Judges       bool          `json:"judges"`        // 会不会给出「通过 / 未通过」（存档题只存作品）
	Gallery      bool          `json:"gallery"`       // 结果是作品（检测记录里显示预览）
	Identity     bool          `json:"identity"`      // 判的是「模型是不是它声称的那个」：未通过就是「模型异常」
	HasPrompt    bool          `json:"has_prompt"`    // 检测项上要填题面
	HasAnswers   bool          `json:"has_answers"`   // 要填标准答案
	RuleText     string        `json:"rule_text"`     // 判定规则的句子，{key} 位置放输入框
	RuleFields   []RuleField   `json:"rule_fields"`   //
	DefaultEvery int           `json:"default_every"` // 新建计划的默认频率（分钟）
	Outcomes     []OutcomeSpec `json:"outcomes"`
}

// Detector 检测方式（Strategy + Registry）。新增一种检测办法只需实现它并 Register。
type Detector interface {
	Spec() TypeSpec
	// NormalizeItem 校验并补默认值：新建 / 修改检测项时调用
	NormalizeItem(params, rule json.RawMessage) (json.RawMessage, json.RawMessage, error)
	// Supports 这个方式能不能测这个模型；不能时给出原因
	Supports(ctx context.Context, platform, model string) (bool, string)
	// Execute 执行一次检测。错误也通过 ExecResult 返回（Outcome = error）
	Execute(ctx context.Context, in ExecInput) ExecResult
	// Classify 按当前规则重新给一次执行归类（存下来的是事实，算不算数由规则说了算）
	Classify(item Item, r Run) string
	// Judge 给结论。rs 是最近的记录（新 → 旧，已按当前规则归类，被标为误报的未通过已改成 ignored）
	Judge(item Item, rs []Run) *Verdict
}

// Describer 可选：按当前归类重写一句话结论（比如指纹识别改了置信度门槛后，历史记录的说法跟着变）
type Describer interface {
	Describe(outcome string, detail map[string]any) string
}

// ExecInput 一次执行的输入
type ExecInput struct {
	Item     Item
	Plan     *Plan
	Target   Target
	Platform string
}

// ExecResult 一次执行的结果
type ExecResult struct {
	Outcome      string
	Summary      string
	Detail       map[string]any
	Prompt       string
	Reply        string
	Error        string
	InputTokens  *int
	OutputTokens *int
}

func errResult(prompt string, err error) ExecResult {
	return ExecResult{Outcome: OutcomeError, Summary: "请求出错", Prompt: prompt, Error: err.Error()}
}

// 通用结果键
const (
	OutcomePass    = "pass"
	OutcomeFail    = "fail"
	OutcomeLow     = "low"
	OutcomeError   = "error"
	OutcomeSaved   = "saved"
	OutcomeSkipped = "skipped"
	OutcomeIgnored = "ignored" // 未通过，但被标为误报
)

// Registry 检测器注册表
type Registry struct {
	byKey map[string]Detector
	order []string
}

func NewRegistry(ds ...Detector) *Registry {
	r := &Registry{byKey: map[string]Detector{}}
	for _, d := range ds {
		r.Register(d)
	}
	return r
}

func (r *Registry) Register(d Detector) {
	k := d.Spec().Key
	if _, ok := r.byKey[k]; !ok {
		r.order = append(r.order, k)
	}
	r.byKey[k] = d
}

func (r *Registry) Get(key string) (Detector, bool) {
	d, ok := r.byKey[key]
	return d, ok
}

func (r *Registry) Specs() []TypeSpec {
	out := make([]TypeSpec, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.byKey[k].Spec())
	}
	return out
}

func (r *Registry) Spec(key string) TypeSpec {
	if d, ok := r.byKey[key]; ok {
		return d.Spec()
	}
	return TypeSpec{Key: key, Name: key}
}

// ---------- 通用判定：最近 window 次有效结果 ----------

// WindowRule 判定规则里各类型共用的部分
type WindowRule struct {
	Window   int `json:"window"`    // 看最近几次有效结果
	MinValid int `json:"min_valid"` // 有效结果不到几次不下结论
}

func (r *WindowRule) normalize(defMin int) {
	if r.Window <= 0 {
		r.Window = 10
	}
	if r.Window > 50 {
		r.Window = 50
	}
	if r.MinValid <= 0 {
		r.MinValid = defMin
	}
	if r.MinValid > r.Window {
		r.MinValid = r.Window
	}
}

// validRuns 最近 window 次有效结果（通过 / 未通过），新 → 旧；ignored 计数单独返回
func validRuns(rs []Run, window int) (valid []Run, ignored int) {
	for _, r := range rs {
		switch r.Outcome {
		case OutcomePass, OutcomeFail:
			valid = append(valid, r)
		case OutcomeIgnored:
			ignored++
		}
		if len(valid) >= window {
			break
		}
	}
	return valid, ignored
}

// leadingErrors 最近连续出错的次数
func leadingErrors(rs []Run) int {
	n := 0
	for _, r := range rs {
		if r.Outcome != OutcomeError {
			break
		}
		n++
	}
	return n
}

// faultVerdict 检测请求连续出错（测不了）。已经判了未通过的不覆盖
func faultVerdict(v *Verdict, rs []Run, faultRuns int) *Verdict {
	if faultRuns <= 0 || leadingErrors(rs) < faultRuns || (v != nil && v.Level == LevelBad) {
		return v
	}
	msg := rs[0].Error
	if msg == "" {
		msg = "请求出错"
	}
	out := &Verdict{Level: LevelFault, Short: truncateRunes(msg, 44), Long: fmt.Sprintf("最近 %d 次检测请求都出错：%s", leadingErrors(rs), msg)}
	if v != nil {
		out.Valid, out.Bad, out.Ignored = v.Valid, v.Bad, v.Ignored
	}
	return out
}

func fewVerdict(valid, minValid int) *Verdict {
	short := "还没有有效结果"
	if valid > 0 {
		short = fmt.Sprintf("有效结果 %d 次", valid)
	}
	return &Verdict{Level: LevelFew, Short: short, Valid: valid, Long: fmt.Sprintf("有效结果 %d 次，满 %d 次后给结论", valid, minValid)}
}

// mostCommon 出现最多的值和次数
func mostCommon(vs []string) (string, int) {
	n := map[string]int{}
	for _, v := range vs {
		if v != "" {
			n[v]++
		}
	}
	keys := make([]string, 0, len(n))
	for k := range n {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return n[keys[i]] > n[keys[j]] || (n[keys[i]] == n[keys[j]] && keys[i] < keys[j]) })
	if len(keys) == 0 {
		return "", 0
	}
	return keys[0], n[keys[0]]
}

func decodeInto(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%w：%v", ErrInvalid, err)
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
