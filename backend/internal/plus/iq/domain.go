// Package iq — [plus] 领域模型
package iq

import (
	"errors"
	"strings"
	"time"
)

const (
	MinInterval  = 5
	MaxInterval  = 7 * 24 * 60
	MaxRetention = 500
)

var (
	ErrInvalid         = errors.New("iq: invalid input")
	ErrNotFound        = errors.New("iq: not found")
	ErrVersionMismatch = errors.New("iq: config version mismatch")
	ErrDisabled        = errors.New("iq: disabled for users")
	ErrKeyUnavailable  = errors.New("iq: api key unavailable")
	ErrDuplicateTask   = errors.New("iq: task already exists for group and model")
)

type invalidErr struct{ msg string }

func (e invalidErr) Error() string { return e.msg }
func (e invalidErr) Unwrap() error { return ErrInvalid }

func invalid(msg string) error { return invalidErr{msg: msg} }

// Format 请求格式（发往网关的哪个接口）
type Format string

const (
	FormatAuto            Format = ""                 // 按分组平台自动选择
	FormatAnthropic       Format = "anthropic"        // POST /v1/messages
	FormatOpenAIChat      Format = "openai_chat"      // POST /v1/chat/completions
	FormatOpenAIResponses Format = "openai_responses" // POST /v1/responses
	FormatGemini          Format = "gemini"           // POST /v1beta/models/{model}:streamGenerateContent
)

func (f Format) valid() bool {
	switch f {
	case FormatAuto, FormatAnthropic, FormatOpenAIChat, FormatOpenAIResponses, FormatGemini:
		return true
	}
	return false
}

// ResolveFormat 自动格式：按平台选最贴近用户真实调用的格式（与 channelstatus 一致）
func ResolveFormat(f Format, platform string) Format {
	if f != FormatAuto {
		return f
	}
	switch strings.ToLower(platform) {
	case "anthropic", "antigravity", "kiro":
		return FormatAnthropic
	case "openai":
		return FormatOpenAIResponses
	case "gemini":
		return FormatGemini
	default:
		return FormatOpenAIChat
	}
}

// Thinking 思考强度
type Thinking string

const (
	ThinkingOff    Thinking = "off"
	ThinkingLow    Thinking = "low"
	ThinkingMedium Thinking = "medium"
	ThinkingHigh   Thinking = "high"
)

func (t Thinking) valid() bool {
	switch t {
	case ThinkingOff, ThinkingLow, ThinkingMedium, ThinkingHigh:
		return true
	}
	return false
}

// budget 思考 token 预算（anthropic / gemini 用）
func (t Thinking) budget() int {
	switch t {
	case ThinkingLow:
		return 2048
	case ThinkingMedium:
		return 8192
	case ThinkingHigh:
		return 16384
	}
	return 0
}

// KeyMode 探测 Key 来源
type KeyMode string

const (
	KeyAuto   KeyMode = "auto"   // 自动生成的分组专用 Key（前缀 + 分组名，挂在创建人名下）
	KeyManual KeyMode = "manual" // 管理员指定的已有 Key
)

// Task 检测任务：分组 × 模型
type Task struct {
	ID              int64     `json:"id"`
	GroupID         int64     `json:"group_id"`
	Model           string    `json:"model"`
	KeyMode         KeyMode   `json:"key_mode"`
	APIKeyID        int64     `json:"api_key_id"`
	Format          Format    `json:"format"`
	Thinking        Thinking  `json:"thinking"`
	IntervalMinutes int       `json:"interval_minutes"`
	Retention       int       `json:"retention"`
	Enabled         bool      `json:"enabled"`
	CreatedBy       int64     `json:"created_by"`
	NextRunAt       time.Time `json:"next_run_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RunStatus 一次检测的状态
type RunStatus string

const (
	RunQueued  RunStatus = "queued"
	RunRunning RunStatus = "running"
	RunSuccess RunStatus = "success"
	RunFailed  RunStatus = "failed"
)

// Trigger 触发方式
type Trigger string

const (
	TriggerCron   Trigger = "cron"
	TriggerManual Trigger = "manual"
)

// AccountHop 一次请求经过的一个上游账号（发生故障切换时有多个）
type AccountHop struct {
	AccountID  int64  `json:"account_id"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Run 一次检测记录
type Run struct {
	ID              int64        `json:"id"`
	TaskID          int64        `json:"task_id"`
	GroupID         int64        `json:"group_id"`
	Model           string       `json:"model"`
	Trigger         Trigger      `json:"trigger"`
	TriggeredBy     *int64       `json:"triggered_by,omitempty"`
	Status          RunStatus    `json:"status"`
	PromptVersion   int          `json:"prompt_version"`
	APIKeyID        *int64       `json:"api_key_id,omitempty"`
	ClientRequestID string       `json:"client_request_id"`
	QueuedAt        time.Time    `json:"queued_at"`
	StartedAt       *time.Time   `json:"started_at,omitempty"`
	FinishedAt      *time.Time   `json:"finished_at,omitempty"`
	DurationMs      *int         `json:"duration_ms,omitempty"`
	InputTokens     *int         `json:"input_tokens,omitempty"`
	OutputTokens    *int         `json:"output_tokens,omitempty"`
	HasHTML         bool         `json:"has_html"`
	ExternalRefs    []string     `json:"external_refs,omitempty"`
	Error           string       `json:"error,omitempty"`
	RawExcerpt      string       `json:"raw_excerpt,omitempty"`
	AccountChain    []AccountHop `json:"account_chain"`
	AttributedAt    *time.Time   `json:"attributed_at,omitempty"`
}

// RunOutcome 执行结束时写回的结果
type RunOutcome struct {
	Status          RunStatus
	ClientRequestID string
	DurationMs      int
	InputTokens     *int
	OutputTokens    *int
	HTML            string
	ExternalRefs    []string
	Error           string
	RawExcerpt      string
}

// GroupInfo 分组只读信息
type GroupInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}
