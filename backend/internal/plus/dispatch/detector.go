package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// OutcomeSpec 单次执行可能的结果（由检测方式声明，检测记录页的筛选、计数、颜色都读它）
type OutcomeSpec struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Tone  string `json:"tone"` // ok | bad | few | err | info
}

// TypeSpec 检测方式的能力声明
type TypeSpec struct {
	Key         string        `json:"key"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Active      bool          `json:"active"`      // 主动检测：要建计划、会发请求、花钱
	Judges      bool          `json:"judges"`      // 是否产生「正常 / 可疑 / 异常」判定
	Comparable  bool          `json:"comparable"`  // 能否在「指标对比」里横向比较
	Supports    string        `json:"supports"`    // 能测什么（给人看）
	JudgeDesc   string        `json:"judge_desc"`  // 怎么判（给人看）
	Outcomes    []OutcomeSpec `json:"outcomes"`    // 单次执行的结果枚举（主动类型）
	PlanFields  []string      `json:"plan_fields"` // 计划表单里出现哪些字段
	Gallery     bool          `json:"gallery"`     // 结果是作品（检测记录里显示缩略图）
}

// Detector 检测方式（Strategy + Registry）。新增一种检测办法只需实现它并 Register。
type Detector interface {
	Spec() TypeSpec
	// NormalizeItem 校验并补默认值：新建 / 修改检测项时调用
	NormalizeItem(params, rule json.RawMessage) (json.RawMessage, json.RawMessage, error)
	// Judge 给一个对象下结论
	Judge(env *JudgeEnv, item Item, s Subject) Verdict
}

// ActiveDetector 需要发请求的检测器
type ActiveDetector interface {
	Detector
	// Supports 这个类型能不能测这个平台 / 模型；不能时给出原因
	Supports(ctx context.Context, platform, model string) (bool, string)
	// Execute 执行一次检测。错误也通过 ExecResult 返回（Outcome = error）
	Execute(ctx context.Context, in ExecInput) ExecResult
	// Classify 按当前规则重新给一次执行归类（规则改了以后历史记录也按新规则显示）
	Classify(item Item, r Run) string
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

// Active 主动检测器；不是主动类型返回 nil
func (r *Registry) Active(key string) ActiveDetector {
	if d, ok := r.byKey[key].(ActiveDetector); ok {
		return d
	}
	return nil
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

// ---------- 判定环境：判定时需要的只读数据，一次加载、所有检测项共用 ----------

type runKey struct {
	item  int64
	cred  string
	model string
}

// JudgeEnv 判定环境
type JudgeEnv struct {
	Now      time.Time
	Catalog  *Catalog
	Traffic  *TrafficSet
	Registry *Registry
	runs     map[runKey][]Run // 新 → 旧
}

// NewJudgeEnv runs 必须按 id 倒序
func NewJudgeEnv(now time.Time, cat *Catalog, tr *TrafficSet, reg *Registry, runs []Run) *JudgeEnv {
	m := map[runKey][]Run{}
	for _, r := range runs {
		k := runKey{r.ItemID, r.CredKey, r.Model}
		m[k] = append(m[k], r)
	}
	for k := range m {
		rs := m[k]
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].ID > rs[j].ID })
	}
	return &JudgeEnv{Now: now, Catalog: cat, Traffic: tr, Registry: reg, runs: m}
}

// RunsOf 某个凭证 × 模型在某个检测项上已结束的执行（新 → 旧）
func (e *JudgeEnv) RunsOf(itemID int64, cred, model string) []Run {
	return e.runs[runKey{itemID, cred, model}]
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

func staleNote(rs []Run) string {
	n := leadingErrors(rs)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("（最近 %d 次请求都出错，结论可能已过期）", n)
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

func fptr(v float64) *float64 { return &v }
