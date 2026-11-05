// Package iq — [plus] 端口接口：用例只依赖这些接口，实现可替换、可 mock。
//
// 扩展点（后续智能网关）：
//   - Channel：本期 GatewayChannel（网关 + 分组 Key）；以后加 AccountDirectChannel 只需换 Endpoint/APIKey/Model
//   - Challenge：本期 PelicanChallenge；以后加糖果题、知识题等判分题型
//   - Attributor：请求 → 实际账号链路，智能调度也可以复用
//   - RunReader：只读查询检测记录，给以后的调度/质量评估使用
package iq

import (
	"context"
	"time"
)

// Target 一次请求打到哪里
type Target struct {
	Endpoint string
	APIKey   string
	Model    string
	Format   Format // 已解析，不会是 FormatAuto
}

// Channel 把任务解析成具体的请求目标
type Channel interface {
	Resolve(ctx context.Context, task Task, group GroupInfo, cfg Config) (Target, error)
}

// Artifact 从模型输出中提取出的作品
type Artifact struct {
	HTML         string
	ExternalRefs []string
}

// Challenge 题型：出题 + 从原始输出中提取作品
type Challenge interface {
	Prompt(cfg Config) string
	// Extract 提取失败返回 error（错误信息直接展示给管理员）
	Extract(raw string) (Artifact, error)
}

// CompletionRequest 一次生成请求
type CompletionRequest struct {
	Target    Target
	Prompt    string
	MaxTokens int
	Thinking  Thinking
}

// Completion 生成结果。即使出错，ClientRequestID / StatusCode 也尽量填上，便于反查账号。
type Completion struct {
	Text            string
	Thinking        string
	InputTokens     *int
	OutputTokens    *int
	ClientRequestID string
	StatusCode      int
}

// LLMClient 多协议生成客户端
type LLMClient interface {
	Complete(ctx context.Context, req CompletionRequest) (Completion, error)
}

// Attributor 根据网关的 client request id 反查实际命中的账号链路。
// found=false 表示还没查到（用量日志可能延迟写入），调用方可以稍后重试。
type Attributor interface {
	Attribute(ctx context.Context, clientRequestID string, since time.Time) (hops []AccountHop, found bool, err error)
}

// ConfigStore 配置存储（单行 + 乐观锁）
type ConfigStore interface {
	LoadConfig(ctx context.Context) (Config, int64, error)
	SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error)
	PreviewSecret(ctx context.Context) ([]byte, error)
}

// TaskStore 检测任务
type TaskStore interface {
	ListTasks(ctx context.Context) ([]Task, error)
	GetTask(ctx context.Context, id int64) (*Task, error)
	CreateTask(ctx context.Context, t Task) (int64, error)
	UpdateTask(ctx context.Context, t Task) error
	DeleteTask(ctx context.Context, id int64) error
}

// RunFilter 检测记录查询条件
type RunFilter struct {
	GroupID   int64
	TaskID    int64
	Status    RunStatus
	Trigger   Trigger
	AccountID int64
	Page      int
	PageSize  int
}

// RunReader 只读查询检测记录（以后的智能调度也读这个接口）
type RunReader interface {
	GetRun(ctx context.Context, id int64) (*Run, error)
	ListRuns(ctx context.Context, f RunFilter) ([]Run, int64, error)
	// RecentRuns 每个任务最近 limit 条已结束的记录；successOnly 只取成功样本
	RecentRuns(ctx context.Context, taskIDs []int64, limit int, successOnly bool) (map[int64][]Run, error)
	RunHTML(ctx context.Context, id int64) (html string, status RunStatus, groupID int64, err error)
	Stats(ctx context.Context, since time.Time) (RunStats, error)
	// ActiveByTask 正在排队 / 运行的任务及其状态
	ActiveByTask(ctx context.Context) (map[int64]RunStatus, error)
}

// RunStats 管理端概览
type RunStats struct {
	Finished int64 `json:"finished"`
	Success  int64 `json:"success"`
	Running  int64 `json:"running"`
	Queued   int64 `json:"queued"`
}

// RunQueue 检测记录的写入与排队（落库，多实例共享）
type RunQueue interface {
	// Enqueue 为任务加一条排队记录；该任务已有排队或运行中的记录时返回 0, nil
	Enqueue(ctx context.Context, task Task, trigger Trigger, by *int64, promptVersion int) (int64, error)
	// ClaimDue 抢占到期的定时任务（推进 next_run_at），返回抢到的任务
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]Task, error)
	// StartQueued 把最多 limit 条排队记录改为运行中并返回
	StartQueued(ctx context.Context, limit int) ([]Run, error)
	CountRunning(ctx context.Context) (int, error)
	Finish(ctx context.Context, runID int64, apiKeyID *int64, out RunOutcome) error
	SaveAttribution(ctx context.Context, runID int64, hops []AccountHop) error
	// Prune 每个状态（成功 / 失败）各只保留最新 keep 条
	Prune(ctx context.Context, taskID int64, keep int) error
	// FailStale 把超时未结束的运行记录标为失败（服务重启、进程崩溃）
	FailStale(ctx context.Context, startedBefore time.Time, message string) (int64, error)
}

// GroupCatalog 分组目录（只读上游 groups 表）
type GroupCatalog interface {
	ListGroups(ctx context.Context) ([]GroupInfo, error)
	AccountNames(ctx context.Context, ids []int64) (map[int64]string, error)
	// AccountCounts 每个分组绑定的未删除账号数
	AccountCounts(ctx context.Context) (map[int64]int, error)
}

// KeyOption 可选的探测 Key（只给管理端看脱敏值）
type KeyOption struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Masked    string `json:"masked"`
	UserID    int64  `json:"user_id"`
	UserEmail string `json:"user_email"`
	Active    bool   `json:"active"`
}

// KeyStore 读取 API Key（只读上游 api_keys）
type KeyStore interface {
	// ResolveKey 返回可用 Key 的明文与所属分组；不可用时返回 ErrKeyUnavailable
	ResolveKey(ctx context.Context, keyID int64) (key string, groupID int64, err error)
	ListGroupKeys(ctx context.Context, groupID int64) ([]KeyOption, error)
	// KeyInfos 按 ID 批量读取 Key 信息（含已停用的），用于管理端展示
	KeyInfos(ctx context.Context, ids []int64) (map[int64]KeyOption, error)
}

// KeyProvisioner 自动生成分组专用 Key（同名已存在则复用）
type KeyProvisioner interface {
	EnsureGroupKey(ctx context.Context, userID, groupID int64, name string) (keyID int64, err error)
	FindGroupKey(ctx context.Context, userID, groupID int64, name string) (keyID int64, found bool, err error)
}

// Visibility 用户可见的分组：和用户登录后能使用的分组一致
type Visibility interface {
	VisibleGroups(ctx context.Context, userID int64) (map[int64]bool, error)
}
