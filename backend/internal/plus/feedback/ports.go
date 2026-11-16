// Package feedback — [plus] 端口：服务层只依赖这些接口，外部模块的实现见 adapters.go
package feedback

import (
	"context"
	"database/sql"
	"io"
	"time"
)

// DBTX 被 *sql.DB 和 *sql.Tx 同时满足（与 points.DBTX 结构一致，可直接传入积分记账）
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Rewarder 发积分；在结案事务内调用，保证结案和入账同时成功或同时失败
type Rewarder interface {
	Grant(ctx context.Context, tx DBTX, userID, feedbackID, points int64) (ledgerID int64, err error)
}

// AccountSnapshot 提交时的账号快照（写进 context，事后排查用）
type AccountSnapshot struct {
	Balance       float64  `json:"balance"`
	Subscriptions []string `json:"subscriptions,omitempty"`
}

// UserCard 管理端详情页的用户卡片
type UserCard struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	Balance       float64   `json:"balance"`
	Subscriptions []string  `json:"subscriptions"`
	RegisteredAt  time.Time `json:"registered_at"`
	Feedbacks     int64     `json:"feedbacks"`
	Accepted      int64     `json:"accepted"` // 已解决 + 已采纳
	RewardTotal   int64     `json:"reward_total"`
}

// AccountReader 读取用户账号信息（只读）
type AccountReader interface {
	Snapshot(ctx context.Context, userID int64) (AccountSnapshot, error)
	Card(ctx context.Context, userID int64) (*UserCard, error)
}

// ErrorLog 用户在某段时间内的一次失败请求
type ErrorLog struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	StatusCode int       `json:"status_code"`
	Model      string    `json:"model"`
	Group      string    `json:"group"`
	Platform   string    `json:"platform"`
	Path       string    `json:"path"`
	Phase      string    `json:"phase"`
	Message    string    `json:"message"`
}

// ErrorLogReader 读取网关错误日志（只给管理端用）
type ErrorLogReader interface {
	Around(ctx context.Context, userID int64, from, to time.Time, limit int) ([]ErrorLog, error)
}

// BlobStore 附件文件存储
type BlobStore interface {
	Put(ctx context.Context, data []byte, ext string) (path string, err error)
	Open(path string) (io.ReadCloser, error)
	Remove(path string) error
}
