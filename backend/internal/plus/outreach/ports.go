// Package outreach — 端口：service / worker 只依赖这些接口，实现见 adapters.go
package outreach

import (
	"context"
	"database/sql"
)

// DBTX 被 *sql.DB 和 *sql.Tx 同时满足（与 points.DBTX 结构兼容）
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Mailer 发送一封 HTML 邮件
type Mailer interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// SiteInfo 站点名称与对外地址（每次调用实时读取）
type SiteInfo interface {
	SiteName(ctx context.Context) string
	FrontendURL(ctx context.Context) string
}

// Verification 邮箱验证模块对触达中心开放的能力（由组合根注入 emailverify.Service）。
// 触达中心不读邮箱验证的表和配置：邮箱是否已验证 / 无效只读视图 plus_email_status，其余都走这个接口。
type Verification interface {
	// IssueInviteToken 签发一次性验证 token（有效期由邮箱验证模块决定）
	IssueInviteToken(ctx context.Context, userID int64, email string) (string, error)
	// MarkInvalid 硬退信时把这个邮箱标为无效
	MarkInvalid(ctx context.Context, userID int64, email string) error
	// RewardPoints 验证奖励积分（邮件文案变量）
	RewardPoints(ctx context.Context) int64
}
