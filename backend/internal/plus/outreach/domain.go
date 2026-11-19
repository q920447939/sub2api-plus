// Package outreach — [plus] 用户触达中心：圈选人群、写邮件、分批执行、统计效果。
//
// 概念（每个只回答一个问题）：
//   - 目的 Purpose（purpose.go）：为什么发、发给什么状态的人、按钮去哪、看什么指标
//   - 人群 Audience（segment.go）：发给谁，规约组合
//   - 内容（content.go / variables.go / render.go）：外壳 + 模板 + 变量 + 渲染管线 + 合规区
//   - 行动 Action（action.go）：唯一的按钮 {{cta_url}} 的去向
//   - 权益 Benefit（benefit.go）：附带福利，领取时发放
//   - 投递：活动 → 批次（快照）→ 收件人 → 台账（repository.go / repo_send.go / worker.go）
//   - 指标 Metric（metric.go）：共性统计 + 目的声明的特性指标
//
// 设计原则：
//   - 全部数据在独立表，不改动上游表；只通过 ports.go 的接口依赖外部能力
//   - 管理员永远不会进入任何人群
//   - 活动本身不发信；只有「开始执行」批次才发；批次持有快照，改活动 / 模板 / 外壳不影响已有批次
package outreach

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// 批次状态
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusDone      = "done"
	StatusCancelled = "cancelled"
)

// 收件人状态
const (
	RcptQueued  = "queued"
	RcptSending = "sending"
	RcptSent    = "sent"
	RcptFailed  = "failed"
	RcptSkipped = "skipped"
)

// 批次选人方式
const (
	PickAll    = "all"
	PickRandom = "random"
	PickUsers  = "users"
)

// RewardReason 积分来源标识，写入积分流水
const RewardReason = "outreach_reward"

const maxPickUsers = 500

// DefaultClaimDays 附带权益时默认的领取有效期
const DefaultClaimDays = 30

// Segment 运营设置的人群条件（空值 = 不限制）。邮箱状态由目的决定，不在这里。
type Segment struct {
	Recharged      string  `json:"recharged"`       // "" | yes | no
	InactiveDays   int     `json:"inactive_days"`   // 最近活跃 / 登录距今 ≥ N 天
	MinBalance     float64 `json:"min_balance"`     // 余额 > N
	RegisteredDays int     `json:"registered_days"` // 注册满 N 天
}

func (s Segment) validate() error {
	if s.Recharged != "" && s.Recharged != "yes" && s.Recharged != "no" {
		return errors.New("充值条件只能是 不限 / 充值过 / 未充值")
	}
	if s.InactiveDays < 0 || s.InactiveDays > 3650 || s.RegisteredDays < 0 || s.RegisteredDays > 3650 {
		return errors.New("天数需在 0 到 3650 之间")
	}
	if s.MinBalance < 0 {
		return errors.New("余额条件不能为负")
	}
	return nil
}

func marshalJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// Campaign 触达活动：定义，不发信
type Campaign struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	Purpose    string        `json:"purpose"`
	TemplateID *int64        `json:"template_id"`
	LayoutID   *int64        `json:"layout_id"`
	Subject    string        `json:"subject"`
	BodyMD     string        `json:"body_md"`
	Segment    Segment       `json:"segment"`
	Benefits   []BenefitSpec `json:"benefits"`
	LandingURL string        `json:"landing_url"`
	ClaimDays  int           `json:"claim_days"` // 附带权益时：发出后多少天内可领
	CreatedBy  int64         `json:"created_by"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// CampaignInput 创建 / 更新入参
type CampaignInput struct {
	Name       string        `json:"name"`
	Purpose    string        `json:"purpose"`
	TemplateID *int64        `json:"template_id"`
	LayoutID   *int64        `json:"layout_id"`
	Subject    string        `json:"subject"`
	BodyMD     string        `json:"body_md"`
	Segment    Segment       `json:"segment"`
	Benefits   []BenefitSpec `json:"benefits"`
	LandingURL string        `json:"landing_url"`
	ClaimDays  int           `json:"claim_days"`
}

// InputError 参数错误，handler 映射成 400
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func badInput(msg string) error { return &InputError{Msg: msg} }

// normalize 不依赖注册表的静态校验；目的相关的校验在 Service.checkContent
func (in *CampaignInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)
	in.BodyMD = strings.TrimSpace(in.BodyMD)
	if in.Benefits == nil {
		in.Benefits = []BenefitSpec{}
	}
	if in.LayoutID != nil && *in.LayoutID <= 0 {
		in.LayoutID = nil
	}
	if in.TemplateID != nil && *in.TemplateID <= 0 {
		in.TemplateID = nil
	}
	switch {
	case in.Name == "" || len([]rune(in.Name)) > 120:
		return badInput("活动名称不能为空，且不超过 120 字")
	case in.Subject == "" || len([]rune(in.Subject)) > 200:
		return badInput("邮件主题不能为空，且不超过 200 字")
	case in.BodyMD == "" || len(in.BodyMD) > 50_000:
		return badInput("邮件正文不能为空，且不超过 5 万字符")
	case len(in.Benefits) > 10:
		return badInput("附带权益最多 10 项")
	}
	// 领取有效期是权益的属性：没有权益时不生效，存缺省值
	if in.ClaimDays == 0 || len(in.Benefits) == 0 {
		in.ClaimDays = DefaultClaimDays
	}
	if in.ClaimDays < 1 || in.ClaimDays > 365 {
		return badInput("领取有效期需在 1 到 365 天之间")
	}
	if err := in.Segment.validate(); err != nil {
		return badInput(err.Error())
	}
	return nil
}

// CampaignDetail 详情：活动 + 完整人群规则 + 来源名称
type CampaignDetail struct {
	Campaign
	Rules        []string `json:"rules"`
	TemplateName string   `json:"template_name"`
	LayoutName   string   `json:"layout_name"`
	BatchCount   int      `json:"batch_count"`
}

// CampaignRow 列表行
type CampaignRow struct {
	Campaign
	BatchCount int         `json:"batch_count"`
	Running    bool        `json:"running"`
	Sent       int         `json:"sent"`
	Primary    *MetricView `json:"primary"` // 核心指标
}

// BatchRequest 生成批次的入参
type BatchRequest struct {
	Name   string   `json:"name"`
	Mode   string   `json:"mode"`
	Scope  string   `json:"scope"`
	Count  int      `json:"count"`
	Emails []string `json:"emails"`
}

func (r *BatchRequest) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	if len([]rune(r.Name)) > 120 {
		return badInput("批次名称不超过 120 字")
	}
	if r.Scope == "" {
		r.Scope = ScopeFresh
	}
	if !validScope(r.Scope) {
		return badInput("选人范围无效")
	}
	switch r.Mode {
	case "":
		r.Mode = PickAll
	case PickAll:
	case PickRandom:
		if r.Count < 1 || r.Count > 1_000_000 {
			return badInput("随机人数需在 1 到 1000000 之间")
		}
	case PickUsers:
		seen := map[string]bool{}
		clean := make([]string, 0, len(r.Emails))
		for _, e := range r.Emails {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" && !seen[e] {
				seen[e] = true
				clean = append(clean, e)
			}
		}
		if len(clean) == 0 {
			return badInput("请填写至少一个邮箱")
		}
		if len(clean) > maxPickUsers {
			return badInput("指定邮箱一次最多 500 个")
		}
		r.Emails = clean
	default:
		return badInput("选人方式无效")
	}
	return nil
}

// Batch 批次。内容字段是生成时的快照。
type Batch struct {
	ID           int64         `json:"id"`
	CampaignID   int64         `json:"campaign_id"`
	CampaignName string        `json:"campaign_name"`
	Name         string        `json:"name"`
	Status       string        `json:"status"`
	PauseReason  string        `json:"pause_reason"`
	PickMode     string        `json:"pick_mode"`
	PickCount    int           `json:"pick_count"`
	Scope        string        `json:"scope"`
	Purpose      string        `json:"purpose"`
	Subject      string        `json:"subject"`
	BodyMD       string        `json:"body_md"`
	Layout       LayoutConfig  `json:"layout"`
	LandingURL   string        `json:"landing_url"`
	Benefits     []BenefitSpec `json:"benefits"`
	ClaimDays    int           `json:"claim_days"`
	Segment      Segment       `json:"segment"`
	Total        int           `json:"total"`
	CreatedBy    int64         `json:"created_by"`
	StartedAt    *time.Time    `json:"started_at"`
	FinishedAt   *time.Time    `json:"finished_at"`
	CreatedAt    time.Time     `json:"created_at"`
}

// BatchRow 列表行：只有投递结果（效果看活动）
type BatchRow struct {
	Batch
	Delivery DeliveryStats `json:"delivery"`
}

// BatchCreated 生成批次的结果
type BatchCreated struct {
	Batch     *Batch `json:"batch"`
	Requested int    `json:"requested"`
	Matched   int    `json:"matched"`
}

// AudiencePreview 人群预估
type AudiencePreview struct {
	Matched  int64        `json:"matched"`  // 命中运营条件的正常用户（非管理员）
	Eligible int64        `json:"eligible"` // 目标人群
	Excluded ExcludeStats `json:"excluded"` // 不在目标人群里的原因（已从 eligible 扣除）
	Notes    AudienceNote `json:"notes"`    // 参考信息（不扣人数）
	Rules    []string     `json:"rules"`    // 完整人群规则
	Samples  []SampleUser `json:"samples"`
}

type ExcludeStats struct {
	Invalid      int64 `json:"invalid"`
	WrongState   int64 `json:"wrong_state"`
	Unsubscribed int64 `json:"unsubscribed"`
}

type AudienceNote struct {
	RecentMail     int64 `json:"recent_mail"`     // 在频控期内：进名单，发送时跳过
	Reached        int64 `json:"reached"`         // 本活动已发送过
	Queued         int64 `json:"queued"`          // 正在本活动未结束的批次里排队
	AvailableFresh int64 `json:"available_fresh"` // 「只发未收到的人」可选人数
	AvailableAgain int64 `json:"available_again"` // 「再发一轮」可选人数
}

type SampleUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// RecipientRow 批次名单
type RecipientRow struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Email     string     `json:"email"`
	Status    string     `json:"status"`
	Bounced   bool       `json:"bounced"`
	Error     string     `json:"error"`
	SentAt    *time.Time `json:"sent_at"`
	ClickedAt *time.Time `json:"clicked_at"`
	ClaimedAt *time.Time `json:"claimed_at"`
	UnsubAt   *time.Time `json:"unsub_at"`
}

// RecipientVars 用户变量快照
type RecipientVars struct {
	Username       string `json:"username"`
	Balance        string `json:"balance"`
	LastActiveDays int    `json:"last_active_days"`
}

// PurposeMeta 一个目的的说明（编辑器 / 模板页 / 设置页使用）
type PurposeMeta struct {
	Key            string        `json:"key"`
	Label          string        `json:"label"`
	Description    string        `json:"description"`
	Category       string        `json:"category"`
	StateText      string        `json:"state_text"`
	RequiresCTA    bool          `json:"requires_cta"`
	AllowsBenefits bool          `json:"allows_benefits"`
	AllowsLanding  bool          `json:"allows_landing"`
	DefaultGapDays int           `json:"default_gap_days"`
	Variables      []VariableDoc `json:"variables"` // 不含权益变量（权益变量见 BenefitMeta）
}

// BenefitMeta 一种可用权益
type BenefitMeta struct {
	Kind      string         `json:"kind"`
	Label     string         `json:"label"`
	Fields    []BenefitField `json:"fields"`
	Variables []VariableDoc  `json:"variables"`
}

// MetaView 编辑器所需的元数据
type MetaView struct {
	Purposes []PurposeMeta `json:"purposes"`
	Benefits []BenefitMeta `json:"benefits"`
}
