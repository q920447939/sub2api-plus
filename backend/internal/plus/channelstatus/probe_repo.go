// Package channelstatus — [plus] 兜底探测的数据访问：探测状态（自有表）+ API Key（只读上游 api_keys）
package channelstatus

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// ProbeStore 探测状态存储
type ProbeStore interface {
	LoadStates(ctx context.Context, groupIDs []int64) (map[int64]*ProbeState, error)
	// Claim 抢占一次探测：prevStarted 必须与库中一致（多实例 CAS），且未超过每日上限；force 跳过两项检查
	Claim(ctx context.Context, groupID int64, prevStarted *time.Time, now time.Time, day string, dailyLimit int, force bool) (bool, error)
	Finish(ctx context.Context, groupID int64, now time.Time, ok bool, latencyMs int, message string, idleStreak int) error
}

// ProbeKey 可用于探测的 API Key（只给管理端看脱敏值）
type ProbeKey struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Masked  string `json:"masked"`
	GroupID int64  `json:"group_id"`
}

// KeyStore 读取 API Key
type KeyStore interface {
	// ResolveKey 返回可用 Key 的明文与所属分组；Key 不存在、停用或过期时返回 ErrKeyUnavailable
	ResolveKey(ctx context.Context, keyID int64) (key string, groupID int64, err error)
	ListUserKeys(ctx context.Context, userID int64) ([]ProbeKey, error)
}

var ErrKeyUnavailable = errors.New("channelstatus: api key unavailable")

var (
	_ ProbeStore = (*Repository)(nil)
	_ KeyStore   = (*Repository)(nil)
)

const activeKeyCond = `deleted_at IS NULL AND status = 'active' AND group_id IS NOT NULL AND (expires_at IS NULL OR expires_at > NOW())`

func (r *Repository) LoadStates(ctx context.Context, groupIDs []int64) (map[int64]*ProbeState, error) {
	out := make(map[int64]*ProbeState, len(groupIDs))
	if len(groupIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_id, last_started_at, last_checked_at, last_ok, latency_ms, message, idle_streak,
		        COALESCE(to_char(day, 'YYYY-MM-DD'), ''), day_count
		 FROM plus_channel_status_probe_state WHERE group_id = ANY($1)`, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			st               ProbeState
			started, checked sql.NullTime
			ok               sql.NullBool
			lat              sql.NullInt64
		)
		if err := rows.Scan(&st.GroupID, &started, &checked, &ok, &lat, &st.Message, &st.IdleStreak, &st.Day, &st.DayCount); err != nil {
			return nil, err
		}
		if started.Valid {
			st.LastStartedAt = &started.Time
		}
		if checked.Valid {
			st.LastCheckedAt = &checked.Time
		}
		if ok.Valid {
			st.LastOK = &ok.Bool
		}
		if lat.Valid {
			v := int(lat.Int64)
			st.LatencyMs = &v
		}
		out[st.GroupID] = &st
	}
	return out, rows.Err()
}

func (r *Repository) Claim(ctx context.Context, groupID int64, prevStarted *time.Time, now time.Time, day string, dailyLimit int, force bool) (bool, error) {
	var prev any
	if prevStarted != nil {
		prev = *prevStarted
	}
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO plus_channel_status_probe_state AS t (group_id, last_started_at, day, day_count, updated_at)
		 VALUES ($1, $2, $3::date, 1, NOW())
		 ON CONFLICT (group_id) DO UPDATE SET
		   last_started_at = EXCLUDED.last_started_at,
		   day_count = CASE WHEN t.day = EXCLUDED.day THEN t.day_count + 1 ELSE 1 END,
		   day = EXCLUDED.day,
		   updated_at = NOW()
		 WHERE $6 OR (t.last_started_at IS NOT DISTINCT FROM $4::timestamptz
		              AND (t.day IS DISTINCT FROM EXCLUDED.day OR t.day_count < $5))
		 RETURNING group_id`,
		groupID, now, day, prev, dailyLimit, force).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) Finish(ctx context.Context, groupID int64, now time.Time, ok bool, latencyMs int, message string, idleStreak int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE plus_channel_status_probe_state
		 SET last_checked_at = $2, last_ok = $3, latency_ms = $4, message = $5, idle_streak = $6, updated_at = NOW()
		 WHERE group_id = $1`, groupID, now, ok, latencyMs, message, idleStreak)
	return err
}

func (r *Repository) ResolveKey(ctx context.Context, keyID int64) (string, int64, error) {
	var (
		key     string
		groupID int64
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT key, group_id FROM api_keys WHERE id = $1 AND `+activeKeyCond, keyID).Scan(&key, &groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrKeyUnavailable
	}
	return key, groupID, err
}

func (r *Repository) ListUserKeys(ctx context.Context, userID int64) ([]ProbeKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, key, group_id FROM api_keys WHERE user_id = $1 AND `+activeKeyCond+` ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ProbeKey{}
	for rows.Next() {
		var (
			k   ProbeKey
			raw string
		)
		if err := rows.Scan(&k.ID, &k.Name, &raw, &k.GroupID); err != nil {
			return nil, err
		}
		k.Masked = maskKey(raw)
		out = append(out, k)
	}
	return out, rows.Err()
}

func maskKey(k string) string {
	if len(k) <= 10 {
		return "****"
	}
	return k[:6] + "****" + k[len(k)-4:]
}
