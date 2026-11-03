// Package slotmachine — [plus] 幸运九宫格数据访问
package slotmachine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

var (
	ErrVersionMismatch = errors.New("slotmachine: config version mismatch")
	ErrIdempotencyKey  = errors.New("slotmachine: idempotency key must be 8-64 chars of [A-Za-z0-9_-]")
	ErrPotInsufficient = errors.New("slotmachine: pot balance insufficient")
)

// ─── 配置（单行 + 乐观锁） ───────────────────────────────────────────────

func (r *Repository) LoadConfig(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT version, data FROM plus_slot_config WHERE id = 1`).Scan(&ver, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultConfig(), 0, nil
	}
	if err != nil {
		return Config{}, 0, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, 0, err
	}
	return cfg, ver, nil
}

func (r *Repository) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var newVer int64
	if expectVer == 0 {
		err = r.db.QueryRowContext(ctx,
			`INSERT INTO plus_slot_config (id, version, data, updated_at)
			 VALUES (1, 1, $1, NOW())
			 ON CONFLICT (id) DO NOTHING
			 RETURNING version`, data).Scan(&newVer)
	} else {
		err = r.db.QueryRowContext(ctx,
			`UPDATE plus_slot_config
			 SET version = version + 1, data = $1, updated_at = NOW()
			 WHERE id = 1 AND version = $2
			 RETURNING version`, data, expectVer).Scan(&newVer)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return newVer, err
}

// ─── 摇奖记录 ────────────────────────────────────────────────────────────

type SpinRecord struct {
	ID           int64     `json:"id"`
	Cost         int64     `json:"cost"`
	TierIndex    int       `json:"tier_index"`
	PrizeType    string    `json:"prize_type"`
	PrizePoints  int64     `json:"prize_points,omitempty"`
	PrizeBalance float64   `json:"prize_balance,omitempty"`
	Label        string    `json:"label,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	// RedeemCode 余额奖凭证（新记录随机生成）；对外统一经 spinRedeemCode 输出，兼容历史空值
	RedeemCode string `json:"-"`
}

func (r *Repository) FindSpinByKey(ctx context.Context, tx *sql.Tx, userID int64, key string) (*SpinRecord, error) {
	var rec SpinRecord
	err := tx.QueryRowContext(ctx,
		`SELECT id, cost, tier_index, prize_type, COALESCE(prize_points, 0), COALESCE(prize_balance, 0)::float8, created_at, COALESCE(redeem_code, '')
		 FROM plus_slot_spins WHERE user_id = $1 AND idempotency_key = $2`,
		userID, key).Scan(&rec.ID, &rec.Cost, &rec.TierIndex, &rec.PrizeType,
		&rec.PrizePoints, &rec.PrizeBalance, &rec.CreatedAt, &rec.RedeemCode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find spin by key: %w", err)
	}
	return &rec, nil
}

// spinInsert 一次摇奖要落库的字段
type spinInsert struct {
	UserID    int64
	Key       string
	Cost      int64
	TierIndex int
	Tier      SlotTier
	LedgerID  int64
	Excluded  []int
	Boosted   bool
	PotPayout int64
}

func (r *Repository) InsertSpin(ctx context.Context, tx *sql.Tx, in spinInsert) (int64, time.Time, error) {
	var (
		id        int64
		createdAt time.Time
		pts       sql.NullInt64
		bal       sql.NullFloat64
		pot       sql.NullString
	)
	switch in.Tier.PrizeType {
	case PrizeTypePoints:
		pts = sql.NullInt64{Int64: in.Tier.PrizePoints, Valid: true}
	case PrizeTypeBalance:
		bal = sql.NullFloat64{Float64: in.Tier.PrizeBalance, Valid: true}
	}
	if in.Tier.Pot != "" {
		pot = sql.NullString{String: in.Tier.Pot, Valid: true}
	}
	excluded := in.Excluded
	if excluded == nil {
		excluded = []int{}
	}
	err := tx.QueryRowContext(ctx,
		`INSERT INTO plus_slot_spins
		   (user_id, idempotency_key, cost, tier_index, prize_type, prize_points, prize_balance, ledger_id,
		    excluded_tiers, boosted, pot, pot_payout, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		 RETURNING id, created_at`,
		in.UserID, in.Key, in.Cost, in.TierIndex, in.Tier.PrizeType, pts, bal, in.LedgerID,
		pq.Array(excluded), in.Boosted, pot, in.PotPayout).Scan(&id, &createdAt)
	return id, createdAt, err
}

func (r *Repository) TodaySpinCount(ctx context.Context, userID int64, dayStart time.Time) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plus_slot_spins WHERE user_id = $1 AND created_at >= $2`,
		userID, dayStart).Scan(&count)
	return count, err
}

func (r *Repository) LastSpinTime(ctx context.Context, userID int64) (sql.NullTime, error) {
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(created_at) FROM plus_slot_spins WHERE user_id = $1`,
		userID).Scan(&t)
	return t, err
}

// RecentRecharge 近期真实充值金额：用户兑换的余额卡密 + 已完成的在线充值订单。
// 在线支付履约生成的 recharge_code 卡密按订单口径计入、此处排除，避免重复；系统奖励码不是充值。
func (r *Repository) RecentRecharge(ctx context.Context, userID int64, since time.Time) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE((SELECT SUM(rc.value) FROM redeem_codes rc
		     WHERE rc.used_by = $1 AND rc.type = 'balance' AND rc.status = 'used'
		       AND rc.used_at >= $2 AND rc.value > 0
		       AND rc.code NOT LIKE 'LOTTERY-%' AND rc.code NOT LIKE 'SLOT-%'
		       AND NOT EXISTS (SELECT 1 FROM payment_orders po WHERE po.recharge_code = rc.code)), 0)
		 + COALESCE((SELECT SUM(po.amount) FROM payment_orders po
		     WHERE po.user_id = $1 AND po.order_type = 'balance' AND po.status = 'COMPLETED'
		       AND po.completed_at >= $2), 0)`,
		userID, since).Scan(&total)
	return total, err
}

// ListHistory 用户抽奖记录分页；winOnly 只看中奖
func (r *Repository) ListHistory(ctx context.Context, userID int64, winOnly bool, page, size int) ([]SpinRecord, int64, error) {
	where := `WHERE user_id = $1`
	if winOnly {
		where += ` AND prize_type <> 'none'`
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_slot_spins `+where, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, cost, tier_index, prize_type, COALESCE(prize_points, 0), COALESCE(prize_balance, 0)::float8, created_at, COALESCE(redeem_code, '')
		 FROM plus_slot_spins `+where+`
		 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`,
		userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	list := []SpinRecord{}
	for rows.Next() {
		var rec SpinRecord
		if err := rows.Scan(&rec.ID, &rec.Cost, &rec.TierIndex, &rec.PrizeType,
			&rec.PrizePoints, &rec.PrizeBalance, &rec.CreatedAt, &rec.RedeemCode); err != nil {
			return nil, 0, err
		}
		list = append(list, rec)
	}
	return list, total, rows.Err()
}

// AdminSpin 管理端摇奖记录
type AdminSpin struct {
	SpinRecord
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Excluded  []int  `json:"excluded_tiers"`
	Boosted   bool   `json:"boosted"`
	Pot       string `json:"pot,omitempty"`
	PotPayout int64  `json:"pot_payout"`
}

// SpinQuery 管理端摇奖记录筛选
type SpinQuery struct {
	Keyword   string // 邮箱/用户名模糊，纯数字匹配用户 ID
	PrizeType string // none / points / balance，空=全部
	Since     time.Time
	Until     time.Time
	Page      int
	PageSize  int
}

func (r *Repository) ListAdminSpins(ctx context.Context, q SpinQuery) ([]AdminSpin, int64, error) {
	where := ` WHERE TRUE`
	var args []any
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		if id, err := strconv.ParseInt(kw, 10, 64); err == nil {
			args = append(args, id)
			where += fmt.Sprintf(" AND s.user_id = $%d", len(args))
		} else {
			args = append(args, "%"+strings.ToLower(kw)+"%")
			where += fmt.Sprintf(" AND (LOWER(u.email) LIKE $%d OR LOWER(u.username) LIKE $%d)", len(args), len(args))
		}
	}
	if q.PrizeType != "" {
		args = append(args, q.PrizeType)
		where += fmt.Sprintf(" AND s.prize_type = $%d", len(args))
	}
	if !q.Since.IsZero() {
		args = append(args, q.Since)
		where += fmt.Sprintf(" AND s.created_at >= $%d", len(args))
	}
	if !q.Until.IsZero() {
		args = append(args, q.Until)
		where += fmt.Sprintf(" AND s.created_at < $%d", len(args))
	}
	from := ` FROM plus_slot_spins s LEFT JOIN users u ON u.id = s.user_id` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := r.db.QueryContext(ctx,
		`SELECT s.id, s.cost, s.tier_index, s.prize_type, COALESCE(s.prize_points, 0), COALESCE(s.prize_balance, 0)::float8,
		        s.created_at, s.user_id, COALESCE(u.email, ''), s.excluded_tiers, s.boosted, COALESCE(s.pot, ''), s.pot_payout`+
			from+fmt.Sprintf(` ORDER BY s.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	list := []AdminSpin{}
	for rows.Next() {
		var a AdminSpin
		var excl pq.Int64Array
		if err := rows.Scan(&a.ID, &a.Cost, &a.TierIndex, &a.PrizeType, &a.PrizePoints, &a.PrizeBalance,
			&a.CreatedAt, &a.UserID, &a.Email, &excl, &a.Boosted, &a.Pot, &a.PotPayout); err != nil {
			return nil, 0, err
		}
		a.Excluded = make([]int, len(excl))
		for i, v := range excl {
			a.Excluded[i] = int(v)
		}
		list = append(list, a)
	}
	return list, total, rows.Err()
}

// ─── 奖金罐 ──────────────────────────────────────────────────────────────

// 奖金罐流水原因
const (
	PotReasonInflow   = "inflow"    // 摇奖注入
	PotReasonPayout   = "payout"    // 中奖支出
	PotReasonAdminAdd = "admin_add" // 管理员加水
	PotReasonAdminSub = "admin_sub" // 管理员抽水
)

// LockPots 在事务内锁住两个罐并返回余额（同时串行化全站摇奖的扣罐）
func (r *Repository) LockPots(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT pot, balance FROM plus_slot_pots ORDER BY pot FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var pot string
		var bal int64
		if err := rows.Scan(&pot, &bal); err != nil {
			return nil, err
		}
		out[pot] = bal
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, ok := out[PotSmall]; !ok {
		return nil, errors.New("slotmachine: pot rows missing, run migrations")
	}
	return out, nil
}

// MovePot 调整某个罐的余额并写流水；余额不足返回 ErrPotInsufficient
func (r *Repository) MovePot(ctx context.Context, tx *sql.Tx, pot string, delta int64, reason string, spinID, operatorID int64, note string) (int64, error) {
	var bal int64
	err := tx.QueryRowContext(ctx,
		`UPDATE plus_slot_pots SET balance = balance + $2, updated_at = NOW()
		 WHERE pot = $1 AND balance + $2 >= 0 RETURNING balance`, pot, delta).Scan(&bal)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrPotInsufficient
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO plus_slot_pot_ledger (pot, delta, balance, reason, spin_id, operator_id, note, created_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, 0), NULLIF($6, 0), $7, NOW())`,
		pot, delta, bal, reason, spinID, operatorID, note)
	return bal, err
}

// PotLedgerItem 奖金罐流水（管理端只展示人工调整与大额支出之外的汇总）
type PotLedgerItem struct {
	ID         int64     `json:"id"`
	Pot        string    `json:"pot"`
	Delta      int64     `json:"delta"`
	Balance    int64     `json:"balance"`
	Reason     string    `json:"reason"`
	SpinID     int64     `json:"spin_id,omitempty"`
	OperatorID int64     `json:"operator_id,omitempty"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListPotLedger 奖金罐流水；manualOnly 只看管理员调整
func (r *Repository) ListPotLedger(ctx context.Context, manualOnly bool, limit int) ([]PotLedgerItem, error) {
	q := `SELECT id, pot, delta, balance, reason, COALESCE(spin_id, 0), COALESCE(operator_id, 0), COALESCE(note, ''), created_at
	      FROM plus_slot_pot_ledger`
	if manualOnly {
		q += ` WHERE reason IN ('admin_add', 'admin_sub')`
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := []PotLedgerItem{}
	for rows.Next() {
		var it PotLedgerItem
		if err := rows.Scan(&it.ID, &it.Pot, &it.Delta, &it.Balance, &it.Reason, &it.SpinID, &it.OperatorID, &it.Note, &it.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

// PotStat 某个罐的统计
type PotStat struct {
	Pot         string `json:"pot"`
	Balance     int64  `json:"balance"`
	TodayInflow int64  `json:"today_inflow"`
	TodayPayout int64  `json:"today_payout"`
	TotalInflow int64  `json:"total_inflow"`
	TotalPayout int64  `json:"total_payout"`
	TodayManual int64  `json:"today_manual"` // 今日人工调整净额
}

// SpinStat 摇奖消耗统计
type SpinStat struct {
	TodaySpins int64 `json:"today_spins"`
	TodayCost  int64 `json:"today_cost"`
	TotalSpins int64 `json:"total_spins"`
	TotalCost  int64 `json:"total_cost"`
}

func (r *Repository) PotStats(ctx context.Context, dayStart time.Time) ([]PotStat, SpinStat, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.pot, p.balance,
		   COALESCE(SUM(l.delta) FILTER (WHERE l.reason = 'inflow' AND l.created_at >= $1), 0),
		   COALESCE(-SUM(l.delta) FILTER (WHERE l.reason = 'payout' AND l.created_at >= $1), 0),
		   COALESCE(SUM(l.delta) FILTER (WHERE l.reason = 'inflow'), 0),
		   COALESCE(-SUM(l.delta) FILTER (WHERE l.reason = 'payout'), 0),
		   COALESCE(SUM(l.delta) FILTER (WHERE l.reason IN ('admin_add', 'admin_sub') AND l.created_at >= $1), 0)
		 FROM plus_slot_pots p LEFT JOIN plus_slot_pot_ledger l ON l.pot = p.pot
		 GROUP BY p.pot, p.balance ORDER BY p.pot DESC`, dayStart)
	if err != nil {
		return nil, SpinStat{}, err
	}
	defer func() { _ = rows.Close() }()
	stats := []PotStat{}
	for rows.Next() {
		var s PotStat
		if err := rows.Scan(&s.Pot, &s.Balance, &s.TodayInflow, &s.TodayPayout, &s.TotalInflow, &s.TotalPayout, &s.TodayManual); err != nil {
			return nil, SpinStat{}, err
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, SpinStat{}, err
	}
	var ss SpinStat
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE created_at >= $1), COALESCE(SUM(cost) FILTER (WHERE created_at >= $1), 0),
		        COUNT(*), COALESCE(SUM(cost), 0) FROM plus_slot_spins`, dayStart).
		Scan(&ss.TodaySpins, &ss.TodayCost, &ss.TotalSpins, &ss.TotalCost)
	return stats, ss, err
}

// ─── 中奖记录（跑马灯，真实与虚拟同表） ──────────────────────────────────

type FeedItem struct {
	DisplayName string    `json:"display_name"`
	TierIndex   int       `json:"tier_index"`
	Label       string    `json:"label"`
	Emoji       string    `json:"emoji"`
	CreatedAt   time.Time `json:"created_at"`
}

// AdminFeedItem 管理端可见是否虚拟
type AdminFeedItem struct {
	FeedItem
	ID        int64 `json:"id"`
	IsVirtual bool  `json:"is_virtual"`
}

// ListFeed 今日（since 之后）最新 limit 条
func (r *Repository) ListFeed(ctx context.Context, since time.Time, limit int) ([]FeedItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT display_name, tier_index, label, emoji, created_at FROM plus_slot_feed
		 WHERE created_at >= $1 AND created_at <= NOW() ORDER BY created_at DESC, id DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := []FeedItem{}
	for rows.Next() {
		var it FeedItem
		if err := rows.Scan(&it.DisplayName, &it.TierIndex, &it.Label, &it.Emoji, &it.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

func (r *Repository) ListAdminFeed(ctx context.Context, limit int) ([]AdminFeedItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, display_name, tier_index, label, emoji, is_virtual, created_at FROM plus_slot_feed
		 WHERE created_at <= NOW() ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := []AdminFeedItem{}
	for rows.Next() {
		var it AdminFeedItem
		if err := rows.Scan(&it.ID, &it.DisplayName, &it.TierIndex, &it.Label, &it.Emoji, &it.IsVirtual, &it.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

// InsertFeed 写一条记录；virtualSlot>0 表示虚拟记录，同一分钟冲突时跳过
func (r *Repository) InsertFeed(ctx context.Context, dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, it FeedItem, virtualSlot int64) error {
	var slot sql.NullInt64
	if virtualSlot > 0 {
		slot = sql.NullInt64{Int64: virtualSlot, Valid: true}
	}
	_, err := dbtx.ExecContext(ctx,
		`INSERT INTO plus_slot_feed (display_name, tier_index, label, emoji, is_virtual, virtual_slot, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (virtual_slot) DO NOTHING`,
		it.DisplayName, it.TierIndex, it.Label, it.Emoji, slot.Valid, slot, it.CreatedAt)
	return err
}

// EmailSuffixWhitelist 上游注册邮箱后缀白名单，用于虚拟昵称
func (r *Repository) EmailSuffixWhitelist(ctx context.Context) ([]string, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`,
		service.SettingKeyRegistrationEmailSuffixWhitelist).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return service.ParseRegistrationEmailSuffixWhitelist(raw), nil
}

// CheckinStreak 当前有效连签天数：今天或昨天签过则取最近一条的 streak，否则 0
func (r *Repository) CheckinStreak(ctx context.Context, userID int64, today string) (int, error) {
	var streak int
	err := r.db.QueryRowContext(ctx,
		`SELECT streak FROM plus_checkin_logs
		 WHERE user_id = $1 AND checked_date >= ($2::date - 1)
		 ORDER BY checked_date DESC LIMIT 1`, userID, today).Scan(&streak)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return streak, err
}

// ─── 工具 ──────────────────────────────────────────────────────────────

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func (r *Repository) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// MaskEmail 脱敏：a***@g***.com（与上游邀请返利风格一致）
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	dot := strings.LastIndex(domain, ".")
	if dot <= 0 || dot >= len(domain)-1 {
		return maskSegment(local) + "@" + maskSegment(domain)
	}
	return maskSegment(local) + "@" + maskSegment(domain[:dot]) + domain[dot:]
}

func maskSegment(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return "***"
	}
	return string(r[0]) + "***"
}
