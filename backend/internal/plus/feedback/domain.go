// Package feedback — [plus] 用户反馈中心：领域模型
//
// 反馈中心是「问题和建议的沉淀与分析」，不是工单：没有 SLA、不分派、不需要用户确认关闭。
// 一条反馈有类型（问题/建议）、场景（从属于类型）、状态、管理员定级和标签，
// 以及一条只追加的时间线（用户补充、管理员回复/追问/备注、系统事件）。
package feedback

import (
	"encoding/json"
	"errors"
	"time"
)

// Kind 反馈类型，用户第一步必选
type Kind string

const (
	KindIssue      Kind = "issue"      // 问题
	KindSuggestion Kind = "suggestion" // 建议
)

func (k Kind) Valid() bool { return k == KindIssue || k == KindSuggestion }

// Status 反馈状态
type Status string

const (
	StatusPending   Status = "pending"   // 待处理：新提交，或用户刚补充过
	StatusAwaiting  Status = "awaiting"  // 已回复：管理员回复了，等用户回应
	StatusResolved  Status = "resolved"  // 已解决（问题）
	StatusAdopted   Status = "adopted"   // 已采纳（建议）
	StatusDeclined  Status = "declined"  // 不处理
	StatusDuplicate Status = "duplicate" // 重复（已下线，只用于历史数据）
)

// Open 未结案（用户还能补充）
func (s Status) Open() bool { return s == StatusPending || s == StatusAwaiting }

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusAwaiting, StatusResolved, StatusAdopted, StatusDeclined, StatusDuplicate:
		return true
	}
	return false
}

// Severity 严重等级：只由管理员判定，只用于分析和排序，与积分无关
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
	SeverityInvalid  Severity = "invalid"
)

func (s Severity) Valid() bool {
	switch s {
	case SeverityCritical, SeverityMajor, SeverityMinor, SeverityInvalid:
		return true
	}
	return false
}

// Severities 展示顺序
var Severities = []Severity{SeverityCritical, SeverityMajor, SeverityMinor, SeverityInvalid}

// AuthorType 时间线条目的作者
type AuthorType string

const (
	AuthorUser   AuthorType = "user"
	AuthorAdmin  AuthorType = "admin"
	AuthorSystem AuthorType = "system"
)

// MsgKind 时间线条目类型
type MsgKind string

const (
	MsgSupplement MsgKind = "supplement" // 用户补充
	MsgReply      MsgKind = "reply"      // 管理员回复（不改状态）
	MsgAsk        MsgKind = "ask"        // 管理员追问（已并入回复，只用于历史数据）
	MsgNote       MsgKind = "note"       // 内部备注（用户不可见）
	MsgEvent      MsgKind = "event"      // 系统事件
)

// EventType 系统事件
type EventType string

const (
	EventClosed    EventType = "closed"     // 结案（含自动结案）
	EventReopened  EventType = "reopened"   // 用户补充后回到待处理
	EventReward    EventType = "reward"     // 发放积分
	EventReclass   EventType = "reclass"    // 修正类型/场景
	EventSeverity  EventType = "severity"   // 定级
	EventTags      EventType = "tags"       // 改标签
	EventMergedIn  EventType = "merged_in"  // 有其他反馈合并进来
	EventFollowUp  EventType = "follow_up"  // 用户提了一条关联到这里的新反馈
	EventAutoClose EventType = "auto_close" // 待补充超时
)

// Event 系统事件的结构化内容（存 JSONB，前端按 type 渲染）
type Event struct {
	Type     EventType `json:"type"`
	Status   Status    `json:"status,omitempty"`
	Points   int64     `json:"points,omitempty"`
	Kind     Kind      `json:"kind,omitempty"`
	Scenario string    `json:"scenario,omitempty"`
	Severity Severity  `json:"severity,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	RefID    int64     `json:"ref_id,omitempty"`
}

// Feedback 一条反馈
type Feedback struct {
	ID             int64           `json:"id"`
	UserID         int64           `json:"user_id"`
	KindUser       Kind            `json:"kind_user"`
	Kind           Kind            `json:"kind"`
	ScenarioUser   string          `json:"scenario_user"`
	Scenario       string          `json:"scenario"`
	Content        string          `json:"content"`
	Extra          json.RawMessage `json:"extra"`
	Context        json.RawMessage `json:"context,omitempty"`
	Attachments    []int64         `json:"attachments"`
	Status         Status          `json:"status"`
	Severity       Severity        `json:"severity,omitempty"`
	Tags           []string        `json:"tags"`
	CloseReason    string          `json:"close_reason,omitempty"`
	MergedIntoID   *int64          `json:"merged_into_id,omitempty"`
	PrevID         *int64          `json:"prev_id,omitempty"`
	RewardPoints   int64           `json:"reward_points"`
	RewardLedgerID *int64          `json:"-"`
	UserUnread     bool            `json:"user_unread"`
	AskedAt        *time.Time      `json:"-"`
	LastActivityAt time.Time       `json:"last_activity_at"`
	CreatedAt      time.Time       `json:"created_at"`
	ClosedAt       *time.Time      `json:"closed_at,omitempty"`
}

// Message 时间线条目
type Message struct {
	ID          int64      `json:"id"`
	FeedbackID  int64      `json:"feedback_id"`
	AuthorType  AuthorType `json:"author_type"`
	AuthorID    *int64     `json:"author_id,omitempty"`
	Kind        MsgKind    `json:"kind"`
	Body        string     `json:"body"`
	Attachments []int64    `json:"attachments"`
	Event       *Event     `json:"event,omitempty"`
	Internal    bool       `json:"internal,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Attachment 附件元数据
type Attachment struct {
	ID         int64
	UploaderID int64
	FeedbackID *int64
	Path       string
	Mime       string
	Size       int64
}

var (
	ErrDisabled        = errors.New("feedback: disabled")
	ErrNotFound        = errors.New("feedback: not found")
	ErrClosed          = errors.New("feedback: already closed")
	ErrVersionMismatch = errors.New("feedback: config version mismatch")
	ErrRewardDisabled  = errors.New("feedback: points module unavailable")
)

// InputError 可直接展示给调用方的参数/规则错误
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func inputErr(msg string) error { return &InputError{Msg: msg} }

// RateError 触发防刷限制
type RateError struct{ Msg string }

func (e *RateError) Error() string { return e.Msg }
