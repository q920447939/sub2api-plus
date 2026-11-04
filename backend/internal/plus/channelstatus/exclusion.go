// Package channelstatus — [plus] 排除用户侧业务限制错误。
//
// 上游 V2 错误分类把「Insufficient account balance」等用户侧 403 归为 upstream_forbidden，
// 余额为 0 的用户反复调用就会把分组错误率拉到很高，而运维监控 SLA 按 is_business_limited
// 把它们排除了。这里与 SLA 口径对齐：用户侧（error_owner=client）的业务限制错误从请求数和
// 错误数中一并扣除；平台侧（无可用账号、分组不支持模型等）仍计为失败。
//
//   - ExcludedCounter（Strategy）：按分组 + 桶统计要扣除的错误数；
//   - ExclusionSource（Decorator）：包装任意 SignalSource 做扣除，开关来自配置；
//   - ExcludedProducer：后台按小时汇总 ops_error_logs，供历史条使用（原始表太大，不能每次全扫）。
package channelstatus

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lib/pq"
)

// userSideErrorFilter 与运维监控 SLA 的排除口径一致，并只取用户侧；
// 计数口径与 V2 错误统计一致（status>=400、排除 count_tokens、按 request_id 去重）
const userSideErrorFilter = `o.is_business_limited AND o.error_owner = 'client'
	AND COALESCE(o.status_code, 0) >= 400 AND NOT o.is_count_tokens`

const userSideErrorCount = `COUNT(DISTINCT COALESCE(NULLIF(o.request_id, ''), 'error:' || o.id::text))`

const utcOrigin = `TIMESTAMPTZ '1970-01-01 00:00:00+00'`

// ExcludedCounter 返回 map[groupID]map[桶起点 unix 秒]应扣除的错误数；分桶规则与 SignalSource 相同
type ExcludedCounter interface {
	Count(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]int64, error)
}

// DBExcludedCounter 桶小于 1 小时（当前状态判定窗口）直接查 ops_error_logs；
// 否则读小时汇总表 plus_channel_status_excluded_1h
type DBExcludedCounter struct{ db *sql.DB }

func NewDBExcludedCounter(db *sql.DB) *DBExcludedCounter { return &DBExcludedCounter{db: db} }

func (c *DBExcludedCounter) Count(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]int64, error) {
	out := map[int64]map[int64]int64{}
	if len(groupIDs) == 0 || !end.After(start) || bucket <= 0 {
		return out, nil
	}
	args := []any{fmt.Sprintf("%d seconds", int64(bucket/time.Second)), start.UTC(), pq.Array(groupIDs), start.UTC(), end.UTC()}
	var q string
	if bucket < time.Hour {
		q = `SELECT o.group_id, date_bin($1::interval, o.created_at, $2::timestamptz), ` + userSideErrorCount + `
			FROM ops_error_logs o
			WHERE o.created_at >= $4 AND o.created_at < $5 AND o.group_id = ANY($3) AND ` + userSideErrorFilter + `
			GROUP BY 1, 2`
	} else {
		q = `SELECT t.group_id, date_bin($1::interval, t.bucket_start, $2::timestamptz), SUM(t.error_requests)
			FROM plus_channel_status_excluded_1h t
			WHERE t.bucket_start >= $4 AND t.bucket_start < $5 AND t.group_id = ANY($3)
			GROUP BY 1, 2`
	}
	rows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("channelstatus: load excluded errors: %w", err)
	}
	err = scanRows(rows, func() error {
		var (
			g   int64
			b   time.Time
			cnt int64
		)
		if err := rows.Scan(&g, &b, &cnt); err != nil {
			return err
		}
		if out[g] == nil {
			out[g] = map[int64]int64{}
		}
		out[g][b.Unix()] += cnt
		return nil
	})
	return out, err
}

// ExclusionEnabledFunc 是否启用排除（按次读取配置）
type ExclusionEnabledFunc func(ctx context.Context) bool

// ExclusionSource 装饰器：从内层信号中扣除用户侧业务限制错误
type ExclusionSource struct {
	inner   SignalSource
	counter ExcludedCounter
	enabled ExclusionEnabledFunc
}

func NewExclusionSource(inner SignalSource, counter ExcludedCounter, enabled ExclusionEnabledFunc) *ExclusionSource {
	return &ExclusionSource{inner: inner, counter: counter, enabled: enabled}
}

var _ SignalSource = (*ExclusionSource)(nil)

func (s *ExclusionSource) Fetch(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]Signal, error) {
	out, err := s.inner.Fetch(ctx, groupIDs, start, end, bucket)
	if err != nil || s.enabled == nil || !s.enabled(ctx) {
		return out, err
	}
	ex, err := s.counter.Count(ctx, groupIDs, start, end, bucket)
	if err != nil {
		return nil, err
	}
	for g, buckets := range ex {
		for b, n := range buckets {
			sig, ok := out[g][b]
			if !ok {
				continue
			}
			if sig = subtractExcluded(sig, n); sig.Requests <= 0 {
				delete(out[g], b)
				continue
			}
			out[g][b] = sig
		}
	}
	return out, nil
}

// subtractExcluded 被排除的错误既不算请求也不算失败（与 SLA 分母一致）
func subtractExcluded(sig Signal, n int64) Signal {
	if n <= 0 {
		return sig
	}
	if n > sig.Requests {
		n = sig.Requests
	}
	sig.Requests -= n
	sig.Errors -= n
	if sig.Errors < 0 {
		sig.Errors = 0
	}
	if sig.Errors > sig.Requests {
		sig.Errors = sig.Requests
	}
	return sig
}

// ─── 小时汇总生产者 ──────────────────────────────────────────────────────

const (
	excludedTickInterval  = time.Minute
	excludedTrailing      = 2 * time.Hour       // 每轮重算最近 2 小时（覆盖迟到写入）
	excludedBackfillChunk = 6 * time.Hour       // 每段补算 6 小时
	excludedBackfillPerTk = 4                   // 每轮最多补 4 段
	excludedRetention     = 91 * 24 * time.Hour // 与 V2 最长保留（90 天）对齐
)

const excludedUpsertSQL = `
INSERT INTO plus_channel_status_excluded_1h (bucket_start, group_id, error_requests, computed_at)
SELECT date_bin('1 hour'::interval, o.created_at, ` + utcOrigin + `), o.group_id, ` + userSideErrorCount + `, NOW()
FROM ops_error_logs o
WHERE o.created_at >= $1 AND o.created_at < $2 AND o.group_id IS NOT NULL AND ` + userSideErrorFilter + `
GROUP BY 1, 2
ON CONFLICT (bucket_start, group_id) DO UPDATE
SET error_requests = EXCLUDED.error_requests, computed_at = EXCLUDED.computed_at`

// ExcludedProducer 维护小时汇总表；只在新渠道状态开启且启用排除时运行。
// upsert 是幂等的，多实例同时跑结果一致，因此不需要 leader 锁。
type ExcludedProducer struct {
	db    *sql.DB
	store ConfigStore
	now   func() time.Time

	stopOnce sync.Once
	stop     chan struct{}
}

func NewExcludedProducer(db *sql.DB, store ConfigStore) *ExcludedProducer {
	return &ExcludedProducer{db: db, store: store, now: time.Now, stop: make(chan struct{})}
}

func (p *ExcludedProducer) Start() {
	go func() {
		t := time.NewTicker(excludedTickInterval)
		defer t.Stop()
		for {
			p.tick()
			select {
			case <-t.C:
			case <-p.stop:
				return
			}
		}
	}()
}

func (p *ExcludedProducer) Stop() { p.stopOnce.Do(func() { close(p.stop) }) }

func (p *ExcludedProducer) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cfg, _, err := p.store.Load(ctx)
	if err != nil || !cfg.Enabled || !cfg.Rules.ExcludeUserSideErrors {
		return
	}
	if err := p.RunOnce(ctx); err != nil {
		log.Printf("[plus.channelstatus] excluded rollup: %v", err)
	}
}

// RunOnce 重算最近一段 + 向前补算若干段 + 清理过期数据
func (p *ExcludedProducer) RunOnce(ctx context.Context) error {
	now := p.now().UTC()
	trailStart := now.Truncate(time.Hour).Add(-excludedTrailing + time.Hour)
	if _, err := p.db.ExecContext(ctx, excludedUpsertSQL, trailStart, now.Add(time.Minute)); err != nil {
		return fmt.Errorf("trailing: %w", err)
	}

	floor := now.Truncate(time.Hour).Add(-excludedRetention + 24*time.Hour)
	for i := 0; i < excludedBackfillPerTk; i++ {
		var cur sql.NullTime
		err := p.db.QueryRowContext(ctx, `SELECT backfill_before FROM plus_channel_status_excluded_cursor WHERE id = 1`).Scan(&cur)
		if err == sql.ErrNoRows {
			if _, err = p.db.ExecContext(ctx, `INSERT INTO plus_channel_status_excluded_cursor (id, backfill_before) VALUES (1, $1) ON CONFLICT (id) DO NOTHING`, trailStart); err != nil {
				return fmt.Errorf("init cursor: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("load cursor: %w", err)
		}
		end := trailStart
		if cur.Valid {
			end = cur.Time.UTC()
		}
		if !end.After(floor) {
			break
		}
		start := end.Add(-excludedBackfillChunk)
		if start.Before(floor) {
			start = floor
		}
		if _, err := p.db.ExecContext(ctx, excludedUpsertSQL, start, end); err != nil {
			return fmt.Errorf("backfill %s..%s: %w", start, end, err)
		}
		if _, err := p.db.ExecContext(ctx, `UPDATE plus_channel_status_excluded_cursor SET backfill_before = $1, updated_at = NOW() WHERE id = 1`, start); err != nil {
			return fmt.Errorf("save cursor: %w", err)
		}
	}

	_, err := p.db.ExecContext(ctx, `DELETE FROM plus_channel_status_excluded_1h WHERE bucket_start < $1`, now.Add(-excludedRetention))
	return err
}
