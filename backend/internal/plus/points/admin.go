// Package points — [plus] 管理端：用户积分账户列表、流水查询、手动加减积分
package points

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var (
	ErrUserNotFound = errors.New("points: user not found")
	ErrInvalidInput = errors.New("points: invalid input")
)

// AdminAdjustNote 用户端流水里看到的说明；管理员填写的原因只存 remark，仅管理端可见
const AdminAdjustNote = "积分调整"

// AdminAccount 管理端积分账户行
type AdminAccount struct {
	UserID       int64      `json:"user_id"`
	Email        string     `json:"email"`
	Username     string     `json:"username,omitempty"`
	Balance      int64      `json:"balance"`
	TotalEarned  int64      `json:"total_earned"`
	TotalSpent   int64      `json:"total_spent"`
	LastActiveAt time.Time  `json:"last_active_at"`
	ExpireAt     *time.Time `json:"expire_at,omitempty"`
}

// AdminLedgerItem 管理端流水：多出用户、操作人与内部备注
type AdminLedgerItem struct {
	LedgerItem
	UserID        int64  `json:"user_id"`
	Email         string `json:"email"`
	OperatorID    int64  `json:"operator_id,omitempty"`
	OperatorEmail string `json:"operator_email,omitempty"`
	Remark        string `json:"remark,omitempty"`
}

// AccountQuery 账户列表筛选
type AccountQuery struct {
	Keyword  string // 邮箱 / 用户名模糊匹配，或纯数字时匹配用户 ID
	Sort     string // balance（默认）/ last_active
	Page     int
	PageSize int
}

// LedgerQuery 管理端流水筛选；UserID=0 表示全站
type LedgerQuery struct {
	UserID   int64
	Keyword  string
	Reason   string
	Page     int
	PageSize int
}

func normPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return page, size
}

// userFilter 生成按关键字匹配用户的条件，返回 SQL 片段与追加后的参数
func userFilter(alias, keyword string, args []any) (string, []any) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", args
	}
	if id, err := strconv.ParseInt(keyword, 10, 64); err == nil {
		args = append(args, id)
		return fmt.Sprintf(" AND %s.id = $%d", alias, len(args)), args
	}
	args = append(args, "%"+strings.ToLower(keyword)+"%")
	n := len(args)
	return fmt.Sprintf(" AND (LOWER(%s.email) LIKE $%d OR LOWER(%s.username) LIKE $%d)", alias, n, alias, n), args
}

// AdminListAccounts 积分账户列表（只含有过积分变动的用户）
func (s *Service) AdminListAccounts(ctx context.Context, q AccountQuery) ([]AdminAccount, int64, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, 0, err
	}
	page, size := normPage(q.Page, q.PageSize)
	cond, args := userFilter("u", q.Keyword, nil)
	from := ` FROM plus_points_accounts a JOIN users u ON u.id = a.user_id WHERE u.deleted_at IS NULL` + cond

	var total int64
	if err := s.repo.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := `a.balance DESC, a.user_id`
	if q.Sort == "last_active" {
		order = `a.last_active_at DESC, a.user_id`
	}
	args = append(args, size, (page-1)*size)
	rows, err := s.repo.db.QueryContext(ctx,
		`SELECT a.user_id, u.email, u.username, a.balance, a.last_active_at,
		        COALESCE((SELECT SUM(delta) FROM plus_points_ledger l WHERE l.user_id = a.user_id AND l.delta > 0), 0),
		        COALESCE((SELECT -SUM(delta) FROM plus_points_ledger l WHERE l.user_id = a.user_id AND l.delta < 0 AND l.reason <> 'expire'), 0)`+
			from+fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, order, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []AdminAccount{}
	for rows.Next() {
		var a AdminAccount
		if err := rows.Scan(&a.UserID, &a.Email, &a.Username, &a.Balance, &a.LastActiveAt, &a.TotalEarned, &a.TotalSpent); err != nil {
			return nil, 0, err
		}
		if a.Balance > 0 {
			a.ExpireAt = cfg.ExpiresAt(a.LastActiveAt)
		}
		list = append(list, a)
	}
	return list, total, rows.Err()
}

// AdminLedger 管理端流水（全站或指定用户）
func (s *Service) AdminLedger(ctx context.Context, q LedgerQuery) ([]AdminLedgerItem, int64, error) {
	page, size := normPage(q.Page, q.PageSize)
	where := ` WHERE TRUE`
	var args []any
	if q.UserID > 0 {
		args = append(args, q.UserID)
		where += fmt.Sprintf(" AND l.user_id = $%d", len(args))
	}
	if q.Reason != "" {
		args = append(args, q.Reason)
		where += fmt.Sprintf(" AND l.reason = $%d", len(args))
	}
	cond, args := userFilter("u", q.Keyword, args)
	from := ` FROM plus_points_ledger l JOIN users u ON u.id = l.user_id
	          LEFT JOIN users op ON op.id = l.operator_id` + where + cond

	var total int64
	if err := s.repo.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := s.repo.db.QueryContext(ctx,
		`SELECT l.id, l.delta, l.balance, l.reason, COALESCE(l.ref_id, 0), COALESCE(l.note, ''), l.created_at,
		        l.user_id, u.email, COALESCE(l.operator_id, 0), COALESCE(op.email, ''), COALESCE(l.remark, '')`+
			from+fmt.Sprintf(` ORDER BY l.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []AdminLedgerItem{}
	for rows.Next() {
		var it AdminLedgerItem
		if err := rows.Scan(&it.ID, &it.Delta, &it.Balance, &it.Reason, &it.RefID, &it.Note, &it.CreatedAt,
			&it.UserID, &it.Email, &it.OperatorID, &it.OperatorEmail, &it.Remark); err != nil {
			return nil, 0, err
		}
		list = append(list, it)
	}
	return list, total, rows.Err()
}

// AdminAdjust 管理员手动加减积分：delta>0 加、<0 减；扣减不允许超过当前余额。返回调整后余额
func (s *Service) AdminAdjust(ctx context.Context, userID, delta, operatorID int64, remark string) (int64, error) {
	remark = strings.TrimSpace(remark)
	if delta == 0 || delta > 10_000_000 || delta < -10_000_000 {
		return 0, fmt.Errorf("%w: 调整数量需为非零且绝对值不超过 1000 万", ErrInvalidInput)
	}
	if remark == "" || utf8.RuneCountInString(remark) > 200 {
		return 0, fmt.Errorf("%w: 请填写 1-200 字的调整原因", ErrInvalidInput)
	}
	var bal int64
	err := s.repo.withTx(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		var entry LedgerEntry
		if delta > 0 {
			entry, err = Credit(ctx, tx, userID, delta, ReasonAdminAdjust, 0, AdminAdjustNote)
		} else {
			entry, err = Debit(ctx, tx, userID, -delta, ReasonAdminAdjust, 0, AdminAdjustNote)
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE plus_points_ledger SET operator_id = $1, remark = $2 WHERE id = $3`,
			operatorID, remark, entry.ID); err != nil {
			return fmt.Errorf("update adjust remark: %w", err)
		}
		bal = entry.Balance
		return nil
	})
	return bal, err
}

// ReasonAmount 某个来源/去向的积分合计
type ReasonAmount struct {
	Reason string `json:"reason"`
	Amount int64  `json:"amount"` // 正数
}

// PointsDayStat 某天的积分发放与消耗
type PointsDayStat struct {
	Date     string `json:"date"`
	Issued   int64  `json:"issued"`
	Consumed int64  `json:"consumed"` // 不含过期清零
	Expired  int64  `json:"expired"`
}

// PointsStats 积分概况
type PointsStats struct {
	Outstanding   int64           `json:"outstanding"` // 全站未使用积分
	Accounts      int64           `json:"accounts"`    // 有积分余额的用户数
	TodayIssued   []ReasonAmount  `json:"today_issued"`
	TodayConsumed []ReasonAmount  `json:"today_consumed"`
	Trend         []PointsDayStat `json:"trend"`
	TopHolders    []TopHolder     `json:"top_holders"` // 剩余积分最多的前 TopHolderLimit 名
}

// TopHolder 积分余额排行
type TopHolder struct {
	Rank         int        `json:"rank"`
	UserID       int64      `json:"user_id"`
	Email        string     `json:"email"`
	Username     string     `json:"username,omitempty"`
	Balance      int64      `json:"balance"`
	LastActiveAt time.Time  `json:"last_active_at"`
	ExpireAt     *time.Time `json:"expire_at,omitempty"`
}

// TopHolderLimit 概览页排行条数
const TopHolderLimit = 10

// topHolders 余额 > 0 的未删除用户，按余额降序；同分按最近活动早的在前（先攒到的排前面）
func (s *Service) topHolders(ctx context.Context, limit int) ([]TopHolder, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.db.QueryContext(ctx,
		`SELECT a.user_id, u.email, u.username, a.balance, a.last_active_at
		 FROM plus_points_accounts a JOIN users u ON u.id = a.user_id
		 WHERE a.balance > 0 AND u.deleted_at IS NULL
		 ORDER BY a.balance DESC, a.last_active_at, a.user_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []TopHolder{}
	for rows.Next() {
		var h TopHolder
		if err := rows.Scan(&h.UserID, &h.Email, &h.Username, &h.Balance, &h.LastActiveAt); err != nil {
			return nil, err
		}
		h.Rank = len(list) + 1
		h.ExpireAt = cfg.ExpiresAt(h.LastActiveAt)
		list = append(list, h)
	}
	return list, rows.Err()
}

// AdminStats 统计近 days 天（含今天，东八区自然日）的积分发放与消耗
func (s *Service) AdminStats(ctx context.Context, days int) (*PointsStats, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	loc := timezone.Location()
	today := timezone.StartOfDay(time.Now().In(loc))
	start := today.AddDate(0, 0, -(days - 1))
	out := &PointsStats{TodayIssued: []ReasonAmount{}, TodayConsumed: []ReasonAmount{}, Trend: []PointsDayStat{}}

	if err := s.repo.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(balance), 0), COUNT(*) FILTER (WHERE balance > 0) FROM plus_points_accounts`).
		Scan(&out.Outstanding, &out.Accounts); err != nil {
		return nil, err
	}

	rows, err := s.repo.db.QueryContext(ctx,
		`SELECT reason, COALESCE(SUM(delta) FILTER (WHERE delta > 0), 0), COALESCE(-SUM(delta) FILTER (WHERE delta < 0), 0)
		 FROM plus_points_ledger WHERE created_at >= $1 GROUP BY reason ORDER BY reason`, today)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var reason string
		var in, outAmt int64
		if err := rows.Scan(&reason, &in, &outAmt); err != nil {
			rows.Close()
			return nil, err
		}
		if in > 0 {
			out.TodayIssued = append(out.TodayIssued, ReasonAmount{reason, in})
		}
		if outAmt > 0 {
			out.TodayConsumed = append(out.TodayConsumed, ReasonAmount{reason, outAmt})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.repo.db.QueryContext(ctx,
		`SELECT (created_at AT TIME ZONE $2)::date AS d,
		        COALESCE(SUM(delta) FILTER (WHERE delta > 0), 0),
		        COALESCE(-SUM(delta) FILTER (WHERE delta < 0 AND reason <> 'expire'), 0),
		        COALESCE(-SUM(delta) FILTER (WHERE delta < 0 AND reason = 'expire'), 0)
		 FROM plus_points_ledger WHERE created_at >= $1 GROUP BY d`, start, PgTimeZone())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]PointsDayStat{}
	for rows.Next() {
		var d time.Time
		var st PointsDayStat
		if err := rows.Scan(&d, &st.Issued, &st.Consumed, &st.Expired); err != nil {
			return nil, err
		}
		st.Date = d.Format("2006-01-02")
		byDate[st.Date] = st
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := days - 1; i >= 0; i-- {
		k := today.AddDate(0, 0, -i).Format("2006-01-02")
		st, ok := byDate[k]
		if !ok {
			st = PointsDayStat{Date: k}
		}
		out.Trend = append(out.Trend, st)
	}
	if out.TopHolders, err = s.topHolders(ctx, TopHolderLimit); err != nil {
		return nil, err
	}
	return out, nil
}

// PgTimeZone 给 SQL `AT TIME ZONE` 用的 IANA 时区名，与 timezone 包配置一致（未初始化时按东八区）
func PgTimeZone() string {
	if n := timezone.Name(); n != "" && n != "Local" {
		return n
	}
	return "Asia/Shanghai"
}
