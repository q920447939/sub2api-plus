// Package emailverify — [plus] 端口：service 层只依赖这些接口，实现见 adapters.go
package emailverify

import (
	"context"
	"database/sql"
)

// DBTX 被 *sql.DB 和 *sql.Tx 同时满足
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// UserReader 读取验证业务需要的用户字段（不依赖 ent，只读必要列）
type UserReader interface {
	// GetEmailVerifyUser 返回用于验证判断的用户信息；用户不存在返回 nil, nil
	GetEmailVerifyUser(ctx context.Context, userID int64) (*EmailVerifyUser, error)
}

// EmailVerifyUser 验证业务需要的用户快照
type EmailVerifyUser struct {
	ID             int64
	Email          string
	Role           string
	TotalRecharged float64
}

// CodeSender 发送邮箱验证码（复用现有 EmailService）
type CodeSender interface {
	SendVerifyCode(ctx context.Context, email, siteName string, locale ...string) error
	VerifyCode(ctx context.Context, email, code string) error
}

// Rewarder 给用户发积分（在事务内调用保证原子性）
type Rewarder interface {
	Grant(ctx context.Context, tx DBTX, userID int64, points int64) (ledgerID int64, err error)
}

// InviteMailer 发送验证邀请邮件。由触达中心实现（统一模板、外壳、链接域名、台账），在组合根注入；
// 本模块不依赖触达中心的任何类型。
type InviteMailer interface {
	SendVerifyInvite(ctx context.Context, userID int64) error
}

// SiteNameReader 读取站点名称（用于邮件文案）
type SiteNameReader interface {
	GetSiteName(ctx context.Context) string
}
