// Package feedback — [plus] 数据访问
//
// queries 绑定在 *sql.DB 或 *sql.Tx 上，同一套方法既能单独执行也能并入事务；
// Repository 是不带事务的入口，WithTx 在事务里拿到同一套方法。
package feedback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type queries struct{ db DBTX }

// Repository 反馈模块的数据访问入口
type Repository struct {
	queries
	sqlDB *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{queries: queries{db: db}, sqlDB: db}
}

// WithTx 在事务里执行 fn；fn 返回错误则回滚
func (r *Repository) WithTx(ctx context.Context, fn func(q *queries, tx *sql.Tx) error) error {
	tx, err := r.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&queries{db: tx}, tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ─── 配置（单行 + 乐观锁） ───────────────────────────────────────────────

func (q *queries) LoadConfig(ctx context.Context) (Config, int64, error) {
	var (
		ver  int64
		data []byte
	)
	err := q.db.QueryRowContext(ctx, `SELECT version, data FROM plus_feedback_config WHERE id = 1`).Scan(&ver, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultConfig(), 0, nil
	}
	if err != nil {
		return Config{}, 0, err
	}
	cfg := DefaultConfig()
	cfg.Scenarios = nil // 场景以保存的为准，不与默认值合并
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, 0, err
	}
	return cfg, ver, nil
}

func (q *queries) SaveConfig(ctx context.Context, cfg Config, expectVer int64) (int64, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, err
	}
	var newVer int64
	if expectVer == 0 {
		err = q.db.QueryRowContext(ctx,
			`INSERT INTO plus_feedback_config (id, version, data, updated_at) VALUES (1, 1, $1, NOW())
			 ON CONFLICT (id) DO NOTHING RETURNING version`, data).Scan(&newVer)
	} else {
		err = q.db.QueryRowContext(ctx,
			`UPDATE plus_feedback_config SET version = version + 1, data = $1, updated_at = NOW()
			 WHERE id = 1 AND version = $2 RETURNING version`, data, expectVer).Scan(&newVer)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVersionMismatch
	}
	return newVer, err
}

// ─── 反馈 ──────────────────────────────────────────────────────────────

const feedbackCols = `id, user_id, kind_user, kind, scenario_user, scenario, content, extra, context, attachments,
	status, COALESCE(severity, ''), tags, COALESCE(close_reason, ''), merged_into_id, prev_id,
	reward_points, reward_ledger_id, user_unread, asked_at, last_activity_at, created_at, closed_at`

func scanFeedback(row interface{ Scan(...any) error }) (*Feedback, error) {
	var (
		f           Feedback
		extra, ctx  []byte
		attachments []byte
		tags        pq.StringArray
	)
	err := row.Scan(&f.ID, &f.UserID, &f.KindUser, &f.Kind, &f.ScenarioUser, &f.Scenario, &f.Content, &extra, &ctx, &attachments,
		&f.Status, &f.Severity, &tags, &f.CloseReason, &f.MergedIntoID, &f.PrevID,
		&f.RewardPoints, &f.RewardLedgerID, &f.UserUnread, &f.AskedAt, &f.LastActivityAt, &f.CreatedAt, &f.ClosedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f.Extra, f.Context, f.Tags = extra, ctx, []string(tags)
	f.Attachments = []int64{}
	_ = json.Unmarshal(attachments, &f.Attachments)
	return &f, nil
}

func (q *queries) Get(ctx context.Context, id int64) (*Feedback, error) {
	return scanFeedback(q.db.QueryRowContext(ctx, `SELECT `+feedbackCols+` FROM plus_feedback WHERE id = $1`, id))
}

// Lock 事务内加行锁读取，状态变更都要先 Lock
func (q *queries) Lock(ctx context.Context, id int64) (*Feedback, error) {
	return scanFeedback(q.db.QueryRowContext(ctx, `SELECT `+feedbackCols+` FROM plus_feedback WHERE id = $1 FOR UPDATE`, id))
}

func (q *queries) Insert(ctx context.Context, f *Feedback) error {
	att, _ := json.Marshal(f.Attachments)
	return q.db.QueryRowContext(ctx,
		`INSERT INTO plus_feedback (user_id, kind_user, kind, scenario_user, scenario, content, extra, context, attachments, prev_id)
		 VALUES ($1, $2, $2, $3, $3, $4, $5, $6, $7, $8)
		 RETURNING id, status, last_activity_at, created_at`,
		f.UserID, f.Kind, f.Scenario, f.Content, jsonOr(f.Extra, "{}"), jsonOr(f.Context, "{}"), att, f.PrevID).
		Scan(&f.ID, &f.Status, &f.LastActivityAt, &f.CreatedAt)
}

func jsonOr(b json.RawMessage, def string) []byte {
	if len(b) == 0 {
		return []byte(def)
	}
	return b
}

// patch 只更新显式设置的列，统一刷新 updated_at
type patch struct {
	cols []string
	args []any
}

func (p *patch) set(col string, v any) *patch {
	p.args = append(p.args, v)
	p.cols = append(p.cols, fmt.Sprintf("%s = $%d", col, len(p.args)))
	return p
}

func (p *patch) raw(expr string) *patch {
	p.cols = append(p.cols, expr)
	return p
}

func (q *queries) Apply(ctx context.Context, id int64, p *patch) error {
	if len(p.cols) == 0 {
		return nil
	}
	args := append(p.args, id)
	_, err := q.db.ExecContext(ctx,
		`UPDATE plus_feedback SET `+strings.Join(p.cols, ", ")+fmt.Sprintf(", updated_at = NOW() WHERE id = $%d", len(args)), args...)
	return err
}

// ─── 时间线 ──────────────────────────────────────────────────────────────

func (q *queries) AddMessage(ctx context.Context, m *Message) error {
	att, _ := json.Marshal(nonNil(m.Attachments))
	var ev any // 无事件时写 NULL（nil []byte 会被当成空串，不是合法 JSON）
	if m.Event != nil {
		b, _ := json.Marshal(m.Event)
		ev = b
	}
	return q.db.QueryRowContext(ctx,
		`INSERT INTO plus_feedback_messages (feedback_id, author_type, author_id, kind, body, attachments, event, internal)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
		m.FeedbackID, m.AuthorType, m.AuthorID, m.Kind, m.Body, att, ev, m.Internal).Scan(&m.ID, &m.CreatedAt)
}

func nonNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

func (q *queries) Messages(ctx context.Context, feedbackID int64, withInternal bool) ([]Message, error) {
	query := `SELECT id, feedback_id, author_type, author_id, kind, body, attachments, event, internal, created_at
		FROM plus_feedback_messages WHERE feedback_id = $1`
	if !withInternal {
		query += ` AND NOT internal`
	}
	rows, err := q.db.QueryContext(ctx, query+` ORDER BY id`, feedbackID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Message{}
	for rows.Next() {
		var (
			m       Message
			att, ev []byte
		)
		if err := rows.Scan(&m.ID, &m.FeedbackID, &m.AuthorType, &m.AuthorID, &m.Kind, &m.Body, &att, &ev, &m.Internal, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Attachments = []int64{}
		_ = json.Unmarshal(att, &m.Attachments)
		if len(ev) > 0 {
			m.Event = &Event{}
			_ = json.Unmarshal(ev, m.Event)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ─── 列表 ──────────────────────────────────────────────────────────────

// ListItem 列表行（用户端和管理端共用，管理端多几个字段）
type ListItem struct {
	ID             int64     `json:"id"`
	Kind           Kind      `json:"kind"`
	Scenario       string    `json:"scenario"`
	Summary        string    `json:"summary"`
	Status         Status    `json:"status"`
	RewardPoints   int64     `json:"reward_points"`
	UserUnread     bool      `json:"user_unread,omitempty"`
	LastActivityAt time.Time `json:"last_activity_at"`
	CreatedAt      time.Time `json:"created_at"`

	UserID         int64    `json:"user_id,omitempty"`
	Email          string   `json:"email,omitempty"`
	Severity       Severity `json:"severity,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Impact         int64    `json:"impact,omitempty"` // 1 + 合并进来的条数
	HasAttachments bool     `json:"has_attachments,omitempty"`
}

const summarySQL = `LEFT(regexp_replace(f.content, '\s+', ' ', 'g'), 120)`

// UserQuery 用户端列表筛选
type UserQuery struct {
	UserID   int64
	Open     *bool // nil=全部，true=未结案，false=已结案
	Kind     Kind  // 空=全部
	Page     int
	PageSize int
}

func (q *queries) ListUser(ctx context.Context, uq UserQuery) ([]ListItem, int64, error) {
	where := ` WHERE f.user_id = $1`
	args := []any{uq.UserID}
	if uq.Open != nil {
		if *uq.Open {
			where += ` AND f.status IN ('pending','awaiting')`
		} else {
			where += ` AND f.status NOT IN ('pending','awaiting')`
		}
	}
	if uq.Kind != "" {
		args = append(args, string(uq.Kind))
		where += fmt.Sprintf(` AND f.kind = $%d`, len(args))
	}
	var total int64
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plus_feedback f`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, uq.PageSize, (uq.Page-1)*uq.PageSize)
	rows, err := q.db.QueryContext(ctx,
		`SELECT f.id, f.kind, f.scenario, `+summarySQL+`, f.status, f.reward_points, f.user_unread, f.last_activity_at, f.created_at
		 FROM plus_feedback f`+where+fmt.Sprintf(` ORDER BY f.user_unread DESC, f.last_activity_at DESC, f.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []ListItem{}
	for rows.Next() {
		var it ListItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Scenario, &it.Summary, &it.Status, &it.RewardPoints, &it.UserUnread, &it.LastActivityAt, &it.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// AdminQuery 管理端列表筛选
type AdminQuery struct {
	Status   string // 状态；open = 待处理+待补充；closed = 已结案；空 = 全部
	Kind     Kind
	Scenario string
	Severity string // 具体等级；none = 未定级
	Tag      string
	UserID   int64
	Content  string // 内容模糊（原文和补充）
	User     string // 邮箱/用户名模糊；纯数字同时匹配用户 ID
	ID       string // 反馈 ID 模糊，可带 #
	From, To *time.Time
	Rewarded *bool
	Page     int
	PageSize int
}

func (aq AdminQuery) where() (string, []any) {
	var (
		conds []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	switch aq.Status {
	case "":
	case "open":
		conds = append(conds, `f.status IN ('pending','awaiting')`)
	case "closed":
		conds = append(conds, `f.status NOT IN ('pending','awaiting')`)
	default:
		add(`f.status = $%d`, aq.Status)
	}
	if aq.Kind != "" {
		add(`f.kind = $%d`, string(aq.Kind))
	}
	if aq.Scenario != "" {
		add(`f.scenario = $%d`, aq.Scenario)
	}
	switch aq.Severity {
	case "":
	case "none":
		conds = append(conds, `f.severity IS NULL`)
	default:
		add(`f.severity = $%d`, aq.Severity)
	}
	if aq.Tag != "" {
		add(`$%d = ANY(f.tags)`, aq.Tag)
	}
	if aq.UserID > 0 {
		add(`f.user_id = $%d`, aq.UserID)
	}
	if v := strings.TrimSpace(aq.Content); v != "" {
		add(`(f.content ILIKE $%[1]d OR EXISTS (SELECT 1 FROM plus_feedback_messages x
		      WHERE x.feedback_id = f.id AND x.author_type = 'user' AND x.body ILIKE $%[1]d))`, "%"+likeEscape(v)+"%")
	}
	if v := strings.TrimSpace(aq.User); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			args = append(args, "%"+likeEscape(v)+"%", n)
			conds = append(conds, fmt.Sprintf(`(u.email ILIKE $%d OR u.username ILIKE $%[1]d OR f.user_id = $%d)`, len(args)-1, len(args)))
		} else {
			add(`(u.email ILIKE $%[1]d OR u.username ILIKE $%[1]d)`, "%"+likeEscape(v)+"%")
		}
	}
	if v := strings.TrimLeft(strings.TrimSpace(aq.ID), "#"); v != "" {
		add(`f.id::text LIKE $%d`, "%"+likeEscape(v)+"%")
	}
	if aq.From != nil {
		add(`f.created_at >= $%d`, *aq.From)
	}
	if aq.To != nil {
		add(`f.created_at < $%d`, *aq.To)
	}
	if aq.Rewarded != nil {
		if *aq.Rewarded {
			conds = append(conds, `f.reward_points > 0`)
		} else {
			conds = append(conds, `f.reward_points = 0`)
		}
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (q *queries) ListAdmin(ctx context.Context, aq AdminQuery) ([]ListItem, int64, error) {
	where, args := aq.where()
	from := ` FROM plus_feedback f JOIN users u ON u.id = f.user_id` + where
	var total int64
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, aq.PageSize, (aq.Page-1)*aq.PageSize)
	rows, err := q.db.QueryContext(ctx,
		`SELECT f.id, f.kind, f.scenario, `+summarySQL+`, f.status, f.reward_points, f.last_activity_at, f.created_at,
		        f.user_id, u.email, COALESCE(f.severity, ''), f.tags,
		        1 + (SELECT COUNT(*) FROM plus_feedback m WHERE m.merged_into_id = f.id),
		        jsonb_array_length(f.attachments) > 0
		           OR EXISTS (SELECT 1 FROM plus_feedback_messages x WHERE x.feedback_id = f.id AND jsonb_array_length(x.attachments) > 0)`+
			from+fmt.Sprintf(` ORDER BY f.last_activity_at DESC, f.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := []ListItem{}
	for rows.Next() {
		var (
			it   ListItem
			tags pq.StringArray
		)
		if err := rows.Scan(&it.ID, &it.Kind, &it.Scenario, &it.Summary, &it.Status, &it.RewardPoints, &it.LastActivityAt, &it.CreatedAt,
			&it.UserID, &it.Email, &it.Severity, &tags, &it.Impact, &it.HasAttachments); err != nil {
			return nil, 0, err
		}
		it.Tags = []string(tags)
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// StatusCounts 各状态数量（管理端页签）
func (q *queries) StatusCounts(ctx context.Context) (map[Status]int64, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM plus_feedback GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[Status]int64{}
	for rows.Next() {
		var (
			s Status
			n int64
		)
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

func (q *queries) count(ctx context.Context, query string, args ...any) (int64, error) {
	var n int64
	err := q.db.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

func (q *queries) PendingCount(ctx context.Context) (int64, error) {
	return q.count(ctx, `SELECT COUNT(*) FROM plus_feedback WHERE status = 'pending'`)
}

func (q *queries) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return q.count(ctx, `SELECT COUNT(*) FROM plus_feedback WHERE user_id = $1 AND user_unread`, userID)
}

func (q *queries) MarkRead(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `UPDATE plus_feedback SET user_unread = FALSE WHERE id = $1 AND user_unread`, id)
	return err
}

// ─── 防刷 ──────────────────────────────────────────────────────────────

func (q *queries) CountSince(ctx context.Context, userID int64, since time.Time) (int64, error) {
	return q.count(ctx, `SELECT COUNT(*) FROM plus_feedback WHERE user_id = $1 AND created_at >= $2`, userID, since)
}

func (q *queries) OpenCount(ctx context.Context, userID int64) (int64, error) {
	return q.count(ctx, `SELECT COUNT(*) FROM plus_feedback WHERE user_id = $1 AND status IN ('pending','awaiting')`, userID)
}

func (q *queries) LastSubmitAt(ctx context.Context, userID int64) (*time.Time, error) {
	var t sql.NullTime
	err := q.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM plus_feedback WHERE user_id = $1`, userID).Scan(&t)
	if err != nil || !t.Valid {
		return nil, err
	}
	return &t.Time, nil
}

// ─── 关联 ──────────────────────────────────────────────────────────────

// Related 关联反馈的简要信息：上一条、本条的后续、合并进来的、合并到的
type Related struct {
	ID       int64  `json:"id"`
	Kind     Kind   `json:"kind"`
	Status   Status `json:"status"`
	Summary  string `json:"summary"`
	Relation string `json:"relation"` // prev / next / merged_in / merged_into
}

func (q *queries) Related(ctx context.Context, f *Feedback, onlyOwner bool) ([]Related, error) {
	query := `SELECT f.id, f.kind, f.status, ` + summarySQL + `,
		CASE WHEN f.id = $2 THEN 'prev' WHEN f.id = $3 THEN 'merged_into' WHEN f.prev_id = $1 THEN 'next' ELSE 'merged_in' END
		FROM plus_feedback f
		WHERE (f.id = $2 OR f.id = $3 OR f.prev_id = $1 OR f.merged_into_id = $1)`
	if onlyOwner {
		query += ` AND f.user_id = $4`
	} else {
		query += ` AND $4::bigint IS NOT NULL`
	}
	rows, err := q.db.QueryContext(ctx, query+` ORDER BY f.id`, f.ID, f.PrevID, f.MergedIntoID, f.UserID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Related{}
	for rows.Next() {
		var r Related
		if err := rows.Scan(&r.ID, &r.Kind, &r.Status, &r.Summary, &r.Relation); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Tags 已用过的标签（按使用次数排序），给管理端做输入提示
func (q *queries) Tags(ctx context.Context) ([]string, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT t FROM plus_feedback, unnest(tags) AS t GROUP BY t ORDER BY COUNT(*) DESC, t LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ExpiredAwaiting 待补充超时的反馈 ID
func (q *queries) ExpiredAwaiting(ctx context.Context, before time.Time, limit int) ([]int64, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id FROM plus_feedback WHERE status = 'awaiting' AND asked_at < $1 ORDER BY asked_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ─── 附件 ──────────────────────────────────────────────────────────────

func (q *queries) InsertAttachment(ctx context.Context, a *Attachment, w, h int) error {
	return q.db.QueryRowContext(ctx,
		`INSERT INTO plus_feedback_attachments (uploader_id, path, mime, size, width, height) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		a.UploaderID, a.Path, a.Mime, a.Size, w, h).Scan(&a.ID)
}

func (q *queries) Attachment(ctx context.Context, id int64) (*Attachment, error) {
	var a Attachment
	err := q.db.QueryRowContext(ctx,
		`SELECT id, uploader_id, feedback_id, path, mime, size FROM plus_feedback_attachments WHERE id = $1`, id).
		Scan(&a.ID, &a.UploaderID, &a.FeedbackID, &a.Path, &a.Mime, &a.Size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

// BindAttachments 把上传者自己的、还没绑定的附件挂到反馈上；有任何一张不符合就失败
func (q *queries) BindAttachments(ctx context.Context, ids []int64, uploaderID, feedbackID int64) error {
	if len(ids) == 0 {
		return nil
	}
	res, err := q.db.ExecContext(ctx,
		`UPDATE plus_feedback_attachments SET feedback_id = $1
		 WHERE id = ANY($2) AND uploader_id = $3 AND feedback_id IS NULL`, feedbackID, pq.Array(ids), uploaderID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != int64(len(ids)) {
		return inputErr("图片已失效，请重新上传")
	}
	return nil
}

// OrphanAttachments 上传后一直没绑定的附件（用户放弃提交）
func (q *queries) OrphanAttachments(ctx context.Context, before time.Time, limit int) ([]Attachment, error) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, path FROM plus_feedback_attachments WHERE feedback_id IS NULL AND created_at < $1 LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.Path); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (q *queries) DeleteAttachment(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, `DELETE FROM plus_feedback_attachments WHERE id = $1`, id)
	return err
}

// likeEscape 转义 LIKE 通配符，让用户输入的 % 和 _ 按字面匹配
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
