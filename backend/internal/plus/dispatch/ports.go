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

// SetupStore 维度 / 检测项 / 计划
type SetupStore interface {
	ListDimensions(ctx context.Context) ([]Dimension, error)
	SaveDimension(ctx context.Context, d Dimension) (int64, error)
	DeleteDimension(ctx context.Context, id int64) error
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
	// RecentRunsForJudge 每个 检测项 × 凭证 × 模型 最近 perKey 条已结束的记录（不含题面和回答）
	RecentRunsForJudge(ctx context.Context, perKey int) ([]Run, error)
	CountOutcomes(ctx context.Context, f RunFilter) (map[string]int, error)
	PruneRuns(ctx context.Context, before time.Time) error
}

// VerdictStore 判定 + 变化历史
type VerdictStore interface {
	LoadVerdicts(ctx context.Context) ([]StoredVerdict, error)
	ReplaceVerdicts(ctx context.Context, vs []StoredVerdict, changes []VerdictChange) error
	ListChanges(ctx context.Context, accountID int64, since time.Time, limit int) ([]VerdictChange, error)
}

// VerdictChange 判定变化
type VerdictChange struct {
	ID        int64     `json:"id"`
	GroupID   int64     `json:"group_id"`
	AccountID int64     `json:"account_id"`
	Model     string    `json:"model"`
	ItemID    int64     `json:"item_id"`
	From      Level     `json:"from"`
	To        Level     `json:"to"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// EventStore 待处理
type EventStore interface {
	OpenEvents(ctx context.Context) ([]Event, error) // 所有未结束的（state <> resolved）
	UpsertEvent(ctx context.Context, e Event) error  // 按 dedup_key 合并到未结束的那条
	ResolveEvents(ctx context.Context, keys []string, at time.Time) error
	SetEventState(ctx context.Context, id int64, state string, by int64) (Event, Event, error)
	ListEvents(ctx context.Context, states []string, limit int) ([]Event, error)
	GetEvent(ctx context.Context, id int64) (*Event, error)
	LastResolved(ctx context.Context, groupID, accountID int64) (*time.Time, error)
}

// Adjustment 分组 × 账号的当前调整
type Adjustment struct {
	GroupID   int64     `json:"group_id"`
	AccountID int64     `json:"account_id"`
	Action    string    `json:"action"` // pause | factor
	Factor    float64   `json:"factor"`
	Source    string    `json:"source"`
	Reason    string    `json:"reason"`
	LogID     *int64    `json:"log_id,omitempty"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AdjustmentLog 调整历史
type AdjustmentLog struct {
	ID         int64          `json:"id"`
	GroupID    int64          `json:"group_id"`
	AccountID  int64          `json:"account_id"`
	FromAction string         `json:"from_action"`
	FromFactor float64        `json:"from_factor"`
	ToAction   string         `json:"to_action"`
	ToFactor   float64        `json:"to_factor"`
	Source     string         `json:"source"`
	Reason     string         `json:"reason"`
	Snapshot   map[string]any `json:"snapshot"`
	OperatorID *int64         `json:"operator_id,omitempty"`
	Operator   string         `json:"operator"`
	CreatedAt  time.Time      `json:"created_at"`
}

// AdjustStore 调整
type AdjustStore interface {
	ListAdjustments(ctx context.Context) ([]Adjustment, error)
	// ApplyAdjustment 事务内：写历史 + 改当前状态（action=normal 时删除）
	ApplyAdjustment(ctx context.Context, a Adjustment, log AdjustmentLog) (int64, error)
	ListAdjustmentLogs(ctx context.Context, groupID, accountID int64, limit int) ([]AdjustmentLog, error)
	GetAdjustmentLog(ctx context.Context, id int64) (*AdjustmentLog, error)
	HookHits(ctx context.Context, groupID int64, since time.Time) (map[int64]int64, error)
	hook.HitSink
}

// AuditEntry 操作记录
type AuditEntry struct {
	ID         int64     `json:"id"`
	OperatorID *int64    `json:"operator_id,omitempty"`
	Operator   string    `json:"operator"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Summary    string    `json:"summary"`
	Before     any       `json:"before,omitempty"`
	After      any       `json:"after,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditFilter 操作记录查询
type AuditFilter struct {
	OperatorID int64
	TargetType string
	TargetID   string
	Keyword    string
	Since      time.Time
	Until      time.Time
	Page       int
	PageSize   int
}

// AuditStore 操作记录
type AuditStore interface {
	AddAudit(ctx context.Context, e AuditEntry) error
	ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, int64, error)
	OperatorEmail(ctx context.Context, userID int64) string
}
