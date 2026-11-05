// Package iq — [plus] 管理端 DTO（只经过 /plus/admin/iq 路由，带账号、Key、错误等排查信息）
package iq

import "time"

// AdminConfigView 设置页
type AdminConfigView struct {
	Config        Config      `json:"config"`
	Version       int64       `json:"version"`
	Groups        []GroupInfo `json:"groups"`
	DefaultPrompt string      `json:"default_prompt"`
}

// HopView 账号链路中的一个账号（带名称）
type HopView struct {
	AccountHop
	AccountName string `json:"account_name"`
}

// RunView 检测记录
type RunView struct {
	Run
	GroupName  string    `json:"group_name"`
	Chain      []HopView `json:"chain"`
	PreviewURL string    `json:"preview_url,omitempty"`
	KeyName    string    `json:"key_name,omitempty"`
	KeyMasked  string    `json:"key_masked,omitempty"`
}

// TaskView 检测任务
type TaskView struct {
	Task
	GroupName string     `json:"group_name"`
	Platform  string     `json:"platform"`
	Key       *KeyOption `json:"key"`
	Recent    []RunView  `json:"recent"` // 最近 5 次（已结束）
	Active    *RunStatus `json:"active"` // 正在排队 / 运行
}

// TasksOverview 任务页
type TasksOverview struct {
	Tasks []TaskView    `json:"tasks"`
	Stats OverviewStats `json:"stats"`
}

// OverviewStats 任务页顶部统计
type OverviewStats struct {
	EnabledTasks   int      `json:"enabled_tasks"`
	TotalTasks     int      `json:"total_tasks"`
	DailyRuns      int      `json:"daily_runs"`
	SuccessRate24h *float64 `json:"success_rate_24h"`
	Running        int64    `json:"running"`
	Queued         int64    `json:"queued"`
	Concurrency    int      `json:"concurrency"`
	SchedulerOn    bool     `json:"scheduler_on"`
}

// TaskInput 新建 / 编辑任务
type TaskInput struct {
	GroupID         int64    `json:"group_id"`
	Model           string   `json:"model"`
	KeyMode         KeyMode  `json:"key_mode"`
	APIKeyID        int64    `json:"api_key_id"`
	Format          Format   `json:"format"`
	Thinking        Thinking `json:"thinking"`
	IntervalMinutes int      `json:"interval_minutes"`
	Retention       int      `json:"retention"`
	Enabled         bool     `json:"enabled"`
}

// AdminGalleryTask 管理端画廊中的一个模型
type AdminGalleryTask struct {
	TaskID          int64     `json:"task_id"`
	Model           string    `json:"model"`
	Enabled         bool      `json:"enabled"`
	IntervalMinutes int       `json:"interval_minutes"`
	NextRunAt       time.Time `json:"next_run_at"`
	Runs            []RunView `json:"runs"`
}

// AdminGalleryGroup 管理端画廊的一行
type AdminGalleryGroup struct {
	GroupInfo
	AccountCount int                `json:"account_count"`
	Tasks        []AdminGalleryTask `json:"tasks"`
}

// TestResult 测试连接
type TestResult struct {
	OK              bool   `json:"ok"`
	LatencyMs       int    `json:"latency_ms"`
	Message         string `json:"message"`
	Reply           string `json:"reply"`
	ClientRequestID string `json:"client_request_id"`
}

// KeyChoices 新建任务时的 Key 选项
type KeyChoices struct {
	AutoName  string      `json:"auto_name"`
	AutoKeyID int64       `json:"auto_key_id"` // 当前管理员名下同名 Key 已存在时为其 ID
	Keys      []KeyOption `json:"keys"`
}
