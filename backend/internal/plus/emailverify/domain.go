// Package emailverify — [plus] 邮箱验证：证明「这个用户能收到这个邮箱的信」。
//
// 职责边界：
//   - 只管验证本身：验证码、一次性邀请 token、验证记录、验证奖励、站内提醒、无效邮箱
//   - 不发营销 / 邀请邮件：单发邀请通过 InviteMailer 交给触达中心（组合根注入）
//   - 不认识活动、批次；触达中心只通过 Service 暴露的少数方法（签发 token、标记无效、读奖励积分）
//     和数据库视图 plus_email_status 使用本模块
//
// 数据模型：
//   - 「已验证」是邮箱的属性：plus_email_verifications 每个 用户 × 邮箱 一行；
//     当前邮箱有记录 = 已验证。换邮箱后自然回到待验证，不需要感知上游的换邮箱流程
//   - 上游注册 / 绑定邮箱 / 第三方登录时已经用验证码证明了邮箱归属，通过 service 包的 [plus] 回调补记
//   - 管理员一律免验证，不记录，不发奖励
package emailverify

import "time"

// EmailStatus 当前邮箱的验证状态
type EmailStatus string

const (
	StatusExempt   EmailStatus = "exempt"   // 管理员，免验证
	StatusVerified EmailStatus = "verified" // 当前邮箱已验证
	StatusPending  EmailStatus = "pending"  // 当前邮箱未验证
	StatusInvalid  EmailStatus = "invalid"  // 当前邮箱发信硬退信
)

// 验证来源
const (
	SourceRegister = "register" // 注册时填了验证码（上游）
	SourceBind     = "bind"     // 绑定 / 更换邮箱时填了验证码（上游）
	SourceOAuth    = "oauth"    // 第三方登录注册（上游）
	SourceCode     = "code"     // 站内验证码
	SourceInvite   = "invite"   // 邮件里的一次性验证链接
)

// SourceLabel 来源的中文名
var SourceLabel = map[string]string{
	SourceRegister: "注册",
	SourceBind:     "绑定邮箱",
	SourceOAuth:    "第三方登录",
	SourceCode:     "站内验证码",
	SourceInvite:   "邀请链接",
}

// UserState 某用户当前邮箱的状态（由 plus_email_status 视图 + 用户级状态得出）
type UserState struct {
	VerifiedAt      *time.Time
	InvalidAt       *time.Time
	RewardClaimedAt *time.Time
}

func (s UserState) Status() EmailStatus {
	switch {
	case s.InvalidAt != nil:
		return StatusInvalid
	case s.VerifiedAt != nil:
		return StatusVerified
	}
	return StatusPending
}

// UserEmailStatusView 用户视角的邮箱状态
type UserEmailStatusView struct {
	Status         EmailStatus `json:"status"`
	Email          string      `json:"email"`
	RewardEligible bool        `json:"reward_eligible"` // 现在验证能拿到奖励
	RewardClaimed  bool        `json:"reward_claimed"`
	RewardPoints   int64       `json:"reward_points"`
	// Remind 登录后是否弹出验证提醒：当前邮箱待验证或无效。不记录用户关闭过，每次登录都检查。
	Remind bool `json:"remind"`
}

// AdminOverviewStats 管理端概览（不含管理员）
type AdminOverviewStats struct {
	Verified    int64            `json:"verified"`
	Pending     int64            `json:"pending"`
	Invalid     int64            `json:"invalid"`
	PendingPaid int64            `json:"pending_paid"` // 待验证且充值过
	PendingFree int64            `json:"pending_free"` // 待验证未充值
	BySource    map[string]int64 `json:"by_source"`    // 验证记录按来源计数
	RewardTotal int64            `json:"reward_total"` // 累计发放的验证奖励积分
}

// Record 一条验证记录
type Record struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	Current      bool      `json:"current"` // 仍是用户当前邮箱（换过邮箱则为 false）
	Source       string    `json:"source"`
	RewardPoints int64     `json:"reward_points"`
	VerifiedAt   time.Time `json:"verified_at"`
}

// RecordQuery 验证记录筛选
type RecordQuery struct {
	Page     int
	PageSize int
	Q        string // 邮箱 / 用户名模糊匹配
	Source   string
}

// VerifyResult 验证成功的结果
type VerifyResult struct {
	Status        EmailStatus
	RewardGranted bool
	RewardPoints  int64
}

// RewardReason 积分来源标识，写入 plus_points_ledger
const RewardReason = "email_verify_reward"

// AdminRoleValue 管理员角色值，与上游 domain.RoleAdmin 对应
const AdminRoleValue = "admin"
