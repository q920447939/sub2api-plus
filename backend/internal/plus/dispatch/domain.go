// Package dispatch — [plus] 上游调度台。
//
// 分层（与需求文档的七层抽象一一对应）：
//
//	维度 Dimension      要回答的问题（模型身份、智力、稳定…），带「作用」：门槛 / 参与调度 / 仅展示
//	检测方式 Detector  检测的办法（代码插件，见 detector_*.go），自己声明能力：要不要计划、结果有哪些、怎么判
//	检测项 Item          某个类型的一个实例（糖果题 = 判分题 + 题面 + 答案），判定规则也在这里
//	检测计划 Plan        主动检测项 × 一个平台 × 若干模型 × 账号范围 × 频率
//	检测记录 Run         每一次账号直连的检测
//	判定 Verdict         （分组 × 账号 × 模型 × 检测项）的当前结论，维度结论 = 下面最差的检测项
//	使用方               待处理（events）、调度建议（simulator）、调度调整（adjust + hook）
//
// 页面、表格列、计划表单都由注册表和配置生成，代码里不出现具体题目名或指标名。
package dispatch

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalid         = errors.New("参数不正确")
	ErrNotFound        = errors.New("记录不存在")
	ErrVersionMismatch = errors.New("配置已被他人修改")
	ErrConflict        = errors.New("冲突")
)

// Level 结论等级
type Level string

const (
	LevelOK   Level = "ok"   // 正常
	LevelWarn Level = "warn" // 可疑
	LevelBad  Level = "bad"  // 异常
	LevelFew  Level = "few"  // 数据不足
	LevelNone Level = "none" // 未检测
	LevelInfo Level = "info" // 只展示，不判定
)

// Rank 越大越严重；用于「取最差」
func (l Level) Rank() float64 {
	switch l {
	case LevelBad:
		return 4
	case LevelWarn:
		return 3
	case LevelOK:
		return 2
	case LevelFew:
		return 1
	case LevelInfo:
		return 0.5
	}
	return 0
}

// Problem 是否需要关注（可疑或异常）
func (l Level) Problem() bool { return l == LevelWarn || l == LevelBad }

// Role 维度的作用
type Role string

const (
	RoleGate    Role = "gate"    // 门槛：异常时按维度上的设置减少或停止分配
	RoleScore   Role = "score"   // 参与调度：有问题时按调度策略的系数减少份额
	RoleDisplay Role = "display" // 仅展示
)

func (r Role) valid() bool { return r == RoleGate || r == RoleScore || r == RoleDisplay }

// Gate 门槛维度判为异常后的处理
type Gate struct {
	Before    string  `json:"before"`     // 还没确认：cut（先降到 x%）| none（只提醒）| stop（直接停止分配）
	BeforePct float64 `json:"before_pct"` //
	After     string  `json:"after"`      // 管理员确认后：stop | cut
	AfterPct  float64 `json:"after_pct"`
}

func defaultGate() Gate { return Gate{Before: "cut", BeforePct: 3, After: "stop"} }

func (g *Gate) normalize() {
	if g.Before != "cut" && g.Before != "none" && g.Before != "stop" {
		g.Before = "cut"
	}
	if g.After != "stop" && g.After != "cut" {
		g.After = "stop"
	}
	g.BeforePct = clampF(g.BeforePct, 0, 100)
	g.AfterPct = clampF(g.AfterPct, 0, 100)
}

// Dimension 维度
type Dimension struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Role    Role      `json:"role"`
	Gate    Gate      `json:"gate"`
	Sort    int       `json:"sort"`
	Enabled bool      `json:"enabled"`
	Updated time.Time `json:"updated_at"`
}

// Item 检测项
type Item struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	DimensionID int64           `json:"dimension_id"`
	DisplayOnly bool            `json:"display_only"`
	Params      json.RawMessage `json:"params"`
	Rule        json.RawMessage `json:"rule"`
	Platforms   []string        `json:"platforms"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Judged 这个检测项是否参与维度判定
func (it Item) Judged(spec TypeSpec) bool { return spec.Judges && !it.DisplayOnly }

// Plan 检测计划（只有主动检测项才有）
type Plan struct {
	ID              int64       `json:"id"`
	ItemID          int64       `json:"item_id"`
	Platform        string      `json:"platform"`
	Models          []string    `json:"models"`
	Scopes          []PlanScope `json:"scopes"`      // 测哪些账号：多条规则，命中任意一条就测
	ExcludeIDs      []int64     `json:"exclude_ids"` // 不测这些账号（优先于上面的规则）
	IntervalMinutes int         `json:"interval_minutes"`
	TimeoutSeconds  int         `json:"timeout_seconds"`
	Enabled         bool        `json:"enabled"`
	NextRunAt       time.Time   `json:"next_run_at"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// defaultNewDays 没有计划选「新接入的账号」时，账号创建后几天内算「新接入」
const defaultNewDays = 3

const (
	minIntervalMinutes = 10
	maxIntervalMinutes = 7 * 24 * 60
)

// RunStatus 执行状态
type RunStatus string

const (
	RunQueued  RunStatus = "queued"
	RunRunning RunStatus = "running"
	RunDone    RunStatus = "done"
)

// Run 一次主动检测
type Run struct {
	ID           int64          `json:"id"`
	PlanID       *int64         `json:"plan_id"`
	ItemID       int64          `json:"item_id"`
	AccountID    int64          `json:"account_id"`
	CredKey      string         `json:"cred_key"`
	Platform     string         `json:"platform"`
	Model        string         `json:"model"`
	Trigger      string         `json:"trigger"`
	TriggeredBy  *int64         `json:"triggered_by,omitempty"`
	Status       RunStatus      `json:"status"`
	Outcome      string         `json:"outcome"`
	Summary      string         `json:"summary"`
	Detail       map[string]any `json:"detail"`
	Prompt       string         `json:"prompt,omitempty"`
	Reply        string         `json:"reply,omitempty"`
	Error        string         `json:"error"`
	DurationMs   *int           `json:"duration_ms"`
	InputTokens  *int           `json:"input_tokens"`
	OutputTokens *int           `json:"output_tokens"`
	QueuedAt     time.Time      `json:"queued_at"`
	StartedAt    *time.Time     `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at"`
}

// Subject 被判定的对象：分组里的某个账号的某个模型
type Subject struct {
	GroupID   int64
	AccountID int64
	Model     string
	Platform  string
	CredKey   string
}

// Verdict 一个检测项对一个对象的结论
type Verdict struct {
	Level    Level          `json:"level"`
	Short    string         `json:"short"`
	Long     string         `json:"long"`
	Value    *float64       `json:"value,omitempty"`
	Ref      *float64       `json:"ref,omitempty"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// StoredVerdict 落库的判定
type StoredVerdict struct {
	GroupID   int64          `json:"group_id"`
	AccountID int64          `json:"account_id"`
	Model     string         `json:"model"`
	ItemID    int64          `json:"item_id"`
	Level     Level          `json:"level"`
	Short     string         `json:"short"`
	Long      string         `json:"long"`
	Value     *float64       `json:"value,omitempty"`
	Ref       *float64       `json:"ref,omitempty"`
	Evidence  map[string]any `json:"evidence,omitempty"`
	Since     time.Time      `json:"since"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Event 待处理
type Event struct {
	ID          int64      `json:"id"`
	Kind        string     `json:"kind"` // dim | fault | gap
	DedupKey    string     `json:"-"`
	GroupID     int64      `json:"group_id"`
	AccountID   int64      `json:"account_id"`
	Model       string     `json:"model"`
	DimensionID int64      `json:"dimension_id"`
	ItemID      int64      `json:"item_id"`
	Level       Level      `json:"level"`
	State       string     `json:"state"` // todo | confirmed | false | ignored | resolved
	Title       string     `json:"title"`
	Evidence    []string   `json:"evidence"`
	FirstAt     time.Time  `json:"first_at"`
	LastAt      time.Time  `json:"last_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	HandledBy   *int64     `json:"handled_by,omitempty"`
	HandledAt   *time.Time `json:"handled_at,omitempty"`
}

const (
	EventTodo      = "todo"
	EventConfirmed = "confirmed"
	EventFalse     = "false"
	EventIgnored   = "ignored"
	EventResolved  = "resolved"
)

// Actor 操作人
type Actor struct {
	ID    int64
	Email string
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
