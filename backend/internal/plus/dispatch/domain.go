// Package dispatch — [plus] 上游调度台 v2。
//
// 两个对象：
//
//	体检对象 = 账号 × 模型：流量跨分组合并，健康分和渠道状态同一套算法；主动检测（指纹 / 判分题 / 存档题）也按账号 × 模型
//	分流对象 = 分组 × 模型（线路）：候选是这个分组里支持这个模型的账号；权重设置挂在 账号 × 分组 ×（模型）上
//
// 分层：
//
//	检测方式 Detector  代码插件（detector_*.go），自己声明能力和判定办法
//	检测项 Item          某个检测方式的一个实例（糖果题 = 判分题 + 题面 + 答案），判定规则也在这里
//	检测计划 Plan        检测项 × 一个平台 × 若干模型 × 指定账号（空 = 全部）× 每 N 分钟
//	检测记录 Run         每一次账号直连的检测；结论（Verdict）按检测项规则从最近的记录现算
//	权重 Rule            账号在某条线路上承接多少；钩子（hook）按它改写候选名单
//	调整记录 Record      每一次保存权重，效果对比挂在这上面
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

// Level 主动检测的结论
type Level string

const (
	LevelOK    Level = "ok"    // 通过
	LevelBad   Level = "bad"   // 未通过
	LevelFew   Level = "few"   // 有效结果还不够
	LevelFault Level = "fault" // 检测请求连续出错（测不了，不是账号没通过）
	LevelInfo  Level = "info"  // 只存档，不判对错
)

// Item 检测项
type Item struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Params    json.RawMessage `json:"params"`
	Rule      json.RawMessage `json:"rule"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Plan 检测计划
type Plan struct {
	ID              int64     `json:"id"`
	ItemID          int64     `json:"item_id"`
	Platform        string    `json:"platform"`
	Models          []string  `json:"models"`
	AccountIDs      []int64   `json:"account_ids"` // 空 = 调度开着、自己提供这个模型的全部账号
	IntervalMinutes int       `json:"interval_minutes"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	Enabled         bool      `json:"enabled"`
	NextRunAt       time.Time `json:"next_run_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const (
	minIntervalMinutes = 1
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
	CredKey      string         `json:"-"`
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
	PreviewURL   string         `json:"preview_url,omitempty"` // 存档题作品的预览链接（签名，1~2 小时有效）
}

// Verdict 一个检测项对「账号 × 模型」的当前结论
type Verdict struct {
	Level   Level  `json:"level"`
	Short   string `json:"short"`
	Long    string `json:"long"`
	Valid   int    `json:"valid"`             // 参与判定的有效结果次数
	Bad     int    `json:"bad"`               // 其中没通过的次数
	Ignored bool   `json:"ignored,omitempty"` // 有被标为误报的结果
}

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
