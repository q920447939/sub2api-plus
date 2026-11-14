package dispatch

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/dispatch/hook"
)

// 用例只依赖这些端口，实现在 repo_*.go（原生 SQL），单测可以替换。

// ConfigStore 全局配置
type ConfigStore interface {
	LoadConfig(ctx context.Context) (Config, int64, error)
	SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error)
}

// SetupStore 检测项 / 计划
type SetupStore interface {
	ListItems(ctx context.Context) ([]Item, error)
	SaveItem(ctx context.Context, it Item) (int64, error)
	DeleteItem(ctx context.Context, id int64) error
	ListPlans(ctx context.Context) ([]Plan, error)
	SavePlan(ctx context.Context, p Plan) (int64, error)
	DeletePlan(ctx context.Context, id int64) error
}

// RunFilter 检测记录查询
type RunFilter struct {
	ItemID    int64
	PlanID    int64
	AccountID int64
	CredKeys  []string
	Model     string
	Outcome   string
	Since     time.Time
	Limit     int
	BeforeID  int64
}

// RunStat 一段时间内的检测次数和花费（检测项 × 账号 × 模型 × 结果）
type RunStat struct {
	ItemID    int64  `json:"item_id"`
	AccountID int64  `json:"account_id"`
	Model     string `json:"model"`
	Outcome   string `json:"outcome"`
	N         int    `json:"n"`
	Tokens    int64  `json:"tokens"`
}

// RunStore 检测记录 + 队列
type RunStore interface {
	Enqueue(ctx context.Context, r Run) (int64, bool, error) // 同一 检测项 × 凭证 × 模型 已在排队 / 运行时不重复排
	ClaimDuePlans(ctx context.Context, now time.Time, limit int) ([]Plan, error)
	StartQueued(ctx context.Context, limit int) ([]Run, error)
	CountRunning(ctx context.Context) (int, error)
	FinishRun(ctx context.Context, id int64, res ExecResult, durationMs int) error
	FailStale(ctx context.Context, startedBefore time.Time, msg string) (int64, error)
	GetRun(ctx context.Context, id int64) (*Run, error)
	ListRuns(ctx context.Context, f RunFilter) ([]Run, error)
	// RecentRuns 每个 检测项 × 凭证 × 模型 最近 perKey 条已结束的记录（不含题面和回答）
	RecentRuns(ctx context.Context, perKey int) ([]Run, error)
	RunStats(ctx context.Context, since time.Time) ([]RunStat, error)
	LastRunAt(ctx context.Context) (map[string]time.Time, error) // 检测项|凭证|模型 → 最近一次排队时间
	// PruneRuns 清理检测记录：存档成功的作品每个 检测项 × 凭证 × 模型 只留最近 keepWorks 份（不看时间），其余的按时间清理
	PruneRuns(ctx context.Context, before time.Time, keepWorks int) error
}

// Ignore 标为误报
type Ignore struct {
	CredKey   string    `json:"-"`
	Model     string    `json:"model"`
	ItemID    int64     `json:"item_id"`
	UpToRun   int64     `json:"up_to_run"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"created_at"`
}

// IgnoreStore 误报标记
type IgnoreStore interface {
	ListIgnores(ctx context.Context) ([]Ignore, error)
	SetIgnore(ctx context.Context, ig Ignore, actor Actor) error
	DeleteIgnore(ctx context.Context, credKey, model string, itemID int64) error
}

// Rule 权重设置（account × group × model）
type Rule struct {
	AccountID int64     `json:"account_id"`
	GroupID   int64     `json:"group_id"`
	Model     string    `json:"model"`         // 空 = 这个分组里它的全部模型
	Share     float64   `json:"share"`         // 0 = 暂停
	Hard      bool      `json:"hard"`          // 暂停时连只有它能跑的部分也停
	Pct       *float64  `json:"pct,omitempty"` // 保存时它在这个分组预计承接的百分比
	RecordID  *int64    `json:"record_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (r Rule) key() ruleKey { return ruleKey{r.AccountID, r.GroupID, r.Model} }

type ruleKey struct {
	Account int64
	Group   int64
	Model   string
}

// RuleState 记录里某个分组「改之前 / 改之后」的设置；nil = 正常
type RuleState struct {
	Share float64  `json:"share"`
	Hard  bool     `json:"hard"`
	Pct   *float64 `json:"pct,omitempty"`
}

// RecordGroup 一次调整在一个分组上的变化
type RecordGroup struct {
	GroupID int64      `json:"group_id"`
	From    *RuleState `json:"from"`
	To      *RuleState `json:"to"`
}

// RecordSide 同组里被连带改动的其他账号
type RecordSide struct {
	AccountID int64      `json:"account_id"`
	GroupID   int64      `json:"group_id"`
	Model     string     `json:"model"`
	From      *RuleState `json:"from"`
	To        *RuleState `json:"to"`
}

// RecordPredict 保存时的预计
type RecordPredict struct {
	GroupID int64   `json:"group_id"`
	Now     float64 `json:"now"`
	Next    float64 `json:"next"`
}

// Record 调整记录
type Record struct {
	ID         int64           `json:"id"`
	AccountID  int64           `json:"account_id"`
	Model      string          `json:"model"`
	Kind       string          `json:"kind"` // adjust | restore | undo
	Note       string          `json:"note"`
	Groups     []RecordGroup   `json:"groups"`
	Side       []RecordSide    `json:"side"`
	Predicted  []RecordPredict `json:"predicted"`
	UndoneAt   *time.Time      `json:"undone_at,omitempty"`
	OperatorID *int64          `json:"-"`
	Operator   string          `json:"operator"`
	CreatedAt  time.Time       `json:"created_at"`
}

// RuleStore 权重设置 + 调整记录
type RuleStore interface {
	ListRules(ctx context.Context) ([]Rule, error)
	// ApplyRecord 事务内：写记录 + 把规则改成 next（只动 touched 里的键），可选把 undoID 标为已恢复
	ApplyRecord(ctx context.Context, rec Record, touched []ruleKey, next []Rule, undoID int64) (int64, error)
	ListRecords(ctx context.Context, accountID int64, limit int) ([]Record, error)
	GetRecord(ctx context.Context, id int64) (*Record, error)
	hook.RulesLoader
}

// AuditEntry 操作记录
type AuditEntry struct {
	ID         int64     `json:"id"`
	OperatorID *int64    `json:"operator_id,omitempty"`
	Operator   string    `json:"operator"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"` // weight | detect | config
	TargetID   string    `json:"target_id"`
	Summary    string    `json:"summary"`
	Before     any       `json:"before,omitempty"`
	After      any       `json:"after,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditFilter 操作记录查询
type AuditFilter struct {
	TargetType string
	Keyword    string
	Page       int
	PageSize   int
}

// AuditStore 操作记录
type AuditStore interface {
	AddAudit(ctx context.Context, e AuditEntry) error
	ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, int64, map[string]int64, error)
	OperatorEmail(ctx context.Context, userID int64) string
	PruneAudit(ctx context.Context, before time.Time) error
}
