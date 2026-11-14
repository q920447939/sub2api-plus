package dispatch

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"
)

// Agg 一段时间内（分组 × 账号 × 模型）的真实流量汇总
type Agg struct {
	Requests   int64   `json:"requests"` // 成功
	Errors     int64   `json:"errors"`   // 上游 5xx / 超时 / 连接中断（不含用户侧错误、429）
	Limited    int64   `json:"limited"`  // 上游 429
	TTFTP90    float64 `json:"-"`        // 毫秒（近似：按样本数加权合并）
	TTFTN      int64   `json:"-"`
	TTFTP50    float64 `json:"-"` // 毫秒（健康分用的口径，和渠道状态一致）
	P50N       int64   `json:"-"`
	OutTokens  int64   `json:"-"` // 只统计流式、输出不少于 50 token 的请求（回答太短时「输出 ÷ 用时」没有意义）
	GenMs      int64   `json:"-"`
	CacheRead  int64   `json:"-"`
	CacheTotal int64   `json:"-"`
}

// Total 所有请求（成功 + 出错 + 限流）
func (a Agg) Total() int64 { return a.Requests + a.Errors + a.Limited }

// Failed 没接住的请求（出错 + 限流）
func (a Agg) Failed() int64 { return a.Errors + a.Limited }

// Add 合并（分位数按样本数加权，近似值）
func (a Agg) Add(b Agg) Agg {
	out := Agg{
		Requests: a.Requests + b.Requests, Errors: a.Errors + b.Errors, Limited: a.Limited + b.Limited,
		TTFTN: a.TTFTN + b.TTFTN, P50N: a.P50N + b.P50N, OutTokens: a.OutTokens + b.OutTokens, GenMs: a.GenMs + b.GenMs,
		CacheRead: a.CacheRead + b.CacheRead, CacheTotal: a.CacheTotal + b.CacheTotal,
	}
	if out.TTFTN > 0 {
		out.TTFTP90 = (a.TTFTP90*float64(a.TTFTN) + b.TTFTP90*float64(b.TTFTN)) / float64(out.TTFTN)
	}
	if out.P50N > 0 {
		out.TTFTP50 = (a.TTFTP50*float64(a.P50N) + b.TTFTP50*float64(b.P50N)) / float64(out.P50N)
	}
	return out
}

// TKey 流量的统计对象
type TKey struct {
	Group   int64
	Account int64
	Model   string
}

// TRow 一个小时桶
type TRow struct {
	Bucket int64 // unix 秒，整点
	TKey
	Agg
}

// TFilter 取数条件（零值 = 不限）
type TFilter struct {
	Account  int64
	Group    int64
	Model    string
	Accounts map[int64]bool
	Groups   map[int64]bool
	Models   map[string]bool
	From     int64 // [From, To)
	To       int64
}

func (f TFilter) match(r *TRow) bool {
	return (f.Account == 0 || r.Account == f.Account) && (f.Group == 0 || r.Group == f.Group) &&
		(f.Model == "" || r.Model == f.Model) && (f.Accounts == nil || f.Accounts[r.Account]) &&
		(f.Groups == nil || f.Groups[r.Group]) && (f.Models == nil || f.Models[r.Model]) &&
		(f.From == 0 || r.Bucket >= f.From) && (f.To == 0 || r.Bucket < f.To)
}

// TrafficSet 最近一个月的小时桶（含当前不满一小时的桶），按账号、分组建了索引
type TrafficSet struct {
	NowAt   time.Time
	Rows    []TRow
	byAcc   map[int64][]int
	byGroup map[int64][]int
}

// NewTrafficSet 由小时桶建索引
func NewTrafficSet(now time.Time, rows []TRow) *TrafficSet {
	t := &TrafficSet{NowAt: now, Rows: rows, byAcc: map[int64][]int{}, byGroup: map[int64][]int{}}
	for i := range rows {
		t.byAcc[rows[i].Account] = append(t.byAcc[rows[i].Account], i)
		t.byGroup[rows[i].Group] = append(t.byGroup[rows[i].Group], i)
	}
	return t
}

// NowBucket 当前（不满一小时的）桶
func (t *TrafficSet) NowBucket() int64 { return t.NowAt.Truncate(time.Hour).Unix() }

// Each 遍历符合条件的桶
func (t *TrafficSet) Each(f TFilter, fn func(r *TRow)) {
	visit := func(i int) {
		if r := &t.Rows[i]; f.match(r) {
			fn(r)
		}
	}
	switch {
	case f.Account != 0:
		for _, i := range t.byAcc[f.Account] {
			visit(i)
		}
	case f.Group != 0:
		for _, i := range t.byGroup[f.Group] {
			visit(i)
		}
	default:
		for i := range t.Rows {
			visit(i)
		}
	}
}

// Sum 合计
func (t *TrafficSet) Sum(f TFilter) Agg {
	var out Agg
	t.Each(f, func(r *TRow) { out = out.Add(r.Agg) })
	return out
}

// ---------- 指标 ----------

// Metric 一个流量指标（账号详情、调整前后对比逐项列出）
type Metric struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Unit   string  `json:"unit"`   // % | s | tok/s
	Better string  `json:"better"` // low | high
	Abs    float64 `json:"-"`      // 前后对比：绝对变化多少才算数（百分点 / 秒）
	Rel    float64 `json:"-"`      // 或相对变化多少才算数
	value  func(a Agg) (float64, bool)
}

// Value 指标值；样本不够算不出时 ok=false
func (m Metric) Value(a Agg) (float64, bool) { return m.value(a) }

func pctOf(n, d int64) (float64, bool) {
	if d <= 0 {
		return 0, false
	}
	return float64(n) * 100 / float64(d), true
}

var metrics = []Metric{
	{Key: "err", Name: "报错率", Unit: "%", Better: "low", Abs: 1, value: func(a Agg) (float64, bool) { return pctOf(a.Errors, a.Total()) }},
	{Key: "r429", Name: "限流率", Unit: "%", Better: "low", Abs: 1, value: func(a Agg) (float64, bool) { return pctOf(a.Limited, a.Total()) }},
	{Key: "ttft", Name: "首字耗时", Unit: "s", Better: "low", Rel: 0.2, value: func(a Agg) (float64, bool) {
		if a.P50N == 0 {
			return 0, false
		}
		return a.TTFTP50 / 1000, true
	}},
	{Key: "tps", Name: "输出速度", Unit: "tok/s", Better: "high", Rel: 0.15, value: func(a Agg) (float64, bool) {
		if a.GenMs <= 0 || a.OutTokens <= 0 {
			return 0, false
		}
		return float64(a.OutTokens) / (float64(a.GenMs) / 1000), true
	}},
	{Key: "cache", Name: "缓存命中率", Unit: "%", Better: "high", Abs: 10, value: func(a Agg) (float64, bool) { return pctOf(a.CacheRead, a.CacheTotal) }},
}

// ---------- 汇总任务：从 usage_logs / ops_error_logs 汇总，写 plus_dispatch_traffic_hourly ----------

// TrafficStore 流量汇总的存取
type TrafficStore interface {
	Aggregate(ctx context.Context, from, to time.Time, hourly bool) (map[TKey]map[int64]Agg, error)
	SaveHourly(ctx context.Context, from, to time.Time, rows map[TKey]map[int64]Agg) error
	LoadHourly(ctx context.Context, from time.Time, groupID int64) (map[TKey]map[int64]Agg, error)
	PruneHourly(ctx context.Context, before time.Time) error
}

// SQLTraffic 原生 SQL 实现
type SQLTraffic struct{ DB *sql.DB }

var _ TrafficStore = SQLTraffic{}

// 模型口径：用户请求的模型名（requested_model，旧数据回退 model），和计划里选的模型一致。
// 错误口径：只算上游（provider / platform 侧）的 5xx、超时、网络错误和 429；用户侧业务限制（余额不足等）不算
const trafficSQL = `
WITH s AS (
  SELECT %[1]s AS b, group_id, account_id, COALESCE(NULLIF(requested_model, ''), model) AS model,
         COUNT(*) AS n,
         percentile_cont(0.9) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE stream AND first_token_ms IS NOT NULL) AS p90,
         COUNT(*) FILTER (WHERE stream AND first_token_ms IS NOT NULL) AS tn,
         percentile_cont(0.5) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE stream AND first_token_ms IS NOT NULL) AS p50,
         COALESCE(SUM(output_tokens) FILTER (WHERE stream AND output_tokens >= 50 AND duration_ms > COALESCE(first_token_ms, 0)), 0) AS ot,
         COALESCE(SUM(duration_ms - COALESCE(first_token_ms, 0)) FILTER (WHERE stream AND output_tokens >= 50 AND duration_ms > COALESCE(first_token_ms, 0)), 0) AS gm,
         COALESCE(SUM(cache_read_tokens), 0) AS cr,
         COALESCE(SUM(input_tokens + cache_read_tokens + cache_creation_tokens), 0) AS ct
  FROM usage_logs
  WHERE created_at >= $1 AND created_at < $2 AND group_id IS NOT NULL AND account_id > 0
  GROUP BY 1, 2, 3, 4
), e AS (
  SELECT %[1]s AS b, group_id, account_id, COALESCE(NULLIF(requested_model, ''), NULLIF(model, ''), '') AS model,
         COUNT(*) FILTER (WHERE COALESCE(upstream_status_code, status_code) = 429) AS lim,
         COUNT(*) FILTER (WHERE COALESCE(upstream_status_code, status_code) IS DISTINCT FROM 429) AS err
  FROM ops_error_logs
  WHERE created_at >= $1 AND created_at < $2 AND group_id IS NOT NULL AND account_id IS NOT NULL AND account_id > 0
    AND NOT is_business_limited AND COALESCE(error_owner, '') <> 'client' AND NOT COALESCE(is_count_tokens, FALSE)
    AND (error_phase IN ('upstream', 'network') OR COALESCE(upstream_status_code, status_code) >= 500
         OR COALESCE(upstream_status_code, status_code) = 429)
  GROUP BY 1, 2, 3, 4
)
SELECT COALESCE(s.b, e.b), COALESCE(s.group_id, e.group_id), COALESCE(s.account_id, e.account_id), COALESCE(s.model, e.model),
       COALESCE(s.n, 0), COALESCE(e.err, 0), COALESCE(e.lim, 0), COALESCE(s.p90, 0), COALESCE(s.tn, 0),
       COALESCE(s.p50, 0), COALESCE(s.ot, 0), COALESCE(s.gm, 0), COALESCE(s.cr, 0), COALESCE(s.ct, 0)
FROM s FULL JOIN e ON s.b = e.b AND s.group_id = e.group_id AND s.account_id = e.account_id AND s.model = e.model`

func (t SQLTraffic) Aggregate(ctx context.Context, from, to time.Time, hourly bool) (map[TKey]map[int64]Agg, error) {
	bucket := "date_trunc('hour', created_at)"
	if !hourly {
		bucket = "$1::timestamptz"
	}
	rows, err := t.DB.QueryContext(ctx, fmt.Sprintf(trafficSQL, bucket), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[TKey]map[int64]Agg{}
	for rows.Next() {
		var (
			b time.Time
			k TKey
			a Agg
		)
		if err := rows.Scan(&b, &k.Group, &k.Account, &k.Model, &a.Requests, &a.Errors, &a.Limited, &a.TTFTP90, &a.TTFTN,
			&a.TTFTP50, &a.OutTokens, &a.GenMs, &a.CacheRead, &a.CacheTotal); err != nil {
			return nil, err
		}
		a.P50N = a.TTFTN
		m := out[k]
		if m == nil {
			m = map[int64]Agg{}
			out[k] = m
		}
		m[b.Unix()] = m[b.Unix()].Add(a)
	}
	return out, rows.Err()
}

func (t SQLTraffic) SaveHourly(ctx context.Context, from, to time.Time, data map[TKey]map[int64]Agg) error {
	tx, err := t.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM plus_dispatch_traffic_hourly WHERE bucket >= $1 AND bucket < $2`, from, to); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO plus_dispatch_traffic_hourly
		(bucket, group_id, account_id, model, requests, errors, limited, ttft_p90, ttft_n, out_tokens, gen_ms, cache_read, cache_total,
		 ttft_p50, p50_n)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (bucket, group_id, account_id, model) DO UPDATE SET requests = EXCLUDED.requests, errors = EXCLUDED.errors,
		  limited = EXCLUDED.limited, ttft_p90 = EXCLUDED.ttft_p90, ttft_n = EXCLUDED.ttft_n, out_tokens = EXCLUDED.out_tokens,
		  gen_ms = EXCLUDED.gen_ms, cache_read = EXCLUDED.cache_read, cache_total = EXCLUDED.cache_total,
		  ttft_p50 = EXCLUDED.ttft_p50, p50_n = EXCLUDED.p50_n`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, m := range data {
		for b, a := range m {
			var p90, p50 any
			if a.TTFTN > 0 {
				p90 = a.TTFTP90
			}
			if a.P50N > 0 {
				p50 = a.TTFTP50
			}
			if _, err := stmt.ExecContext(ctx, time.Unix(b, 0), k.Group, k.Account, k.Model, a.Requests, a.Errors, a.Limited,
				p90, a.TTFTN, a.OutTokens, a.GenMs, a.CacheRead, a.CacheTotal, p50, a.P50N); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (t SQLTraffic) LoadHourly(ctx context.Context, from time.Time, groupID int64) (map[TKey]map[int64]Agg, error) {
	q := `SELECT bucket, group_id, account_id, model, requests, errors, limited, COALESCE(ttft_p90, 0), ttft_n,
	             out_tokens, gen_ms, cache_read, cache_total, COALESCE(ttft_p50, 0), p50_n
	      FROM plus_dispatch_traffic_hourly WHERE bucket >= $1`
	args := []any{from}
	if groupID > 0 {
		q += ` AND group_id = $2`
		args = append(args, groupID)
	}
	rows, err := t.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[TKey]map[int64]Agg{}
	for rows.Next() {
		var (
			b time.Time
			k TKey
			a Agg
		)
		if err := rows.Scan(&b, &k.Group, &k.Account, &k.Model, &a.Requests, &a.Errors, &a.Limited, &a.TTFTP90, &a.TTFTN,
			&a.OutTokens, &a.GenMs, &a.CacheRead, &a.CacheTotal, &a.TTFTP50, &a.P50N); err != nil {
			return nil, err
		}
		m := out[k]
		if m == nil {
			m = map[int64]Agg{}
			out[k] = m
		}
		m[b.Unix()] = a
	}
	return out, rows.Err()
}

func (t SQLTraffic) PruneHourly(ctx context.Context, before time.Time) error {
	_, err := t.DB.ExecContext(ctx, `DELETE FROM plus_dispatch_traffic_hourly WHERE bucket < $1`, before)
	return err
}

// TrafficLoader 汇总 + 读取（带首次回填）
type TrafficLoader struct {
	Store    TrafficStore
	backfill bool
}

const (
	trafficBackfill  = 8 * 24 * time.Hour  // 启动时按当前口径重算多久
	trafficWindow    = 31 * 24 * time.Hour // 页面最多看多久（按天的状态条 30 天）
	trafficRetention = 35 * 24 * time.Hour
)

// Refresh 重算最近两个小时的小时桶（含当前小时）；第一次运行时回填 8 天
func (l *TrafficLoader) Refresh(ctx context.Context, now time.Time) error {
	from := now.Truncate(time.Hour).Add(-time.Hour)
	if !l.backfill {
		from = now.Truncate(time.Hour).Add(-trafficBackfill)
	}
	to := now.Truncate(time.Hour).Add(time.Hour)
	// 回填按天分段，避免一次扫太多
	for start := from; start.Before(to); start = start.Add(24 * time.Hour) {
		end := start.Add(24 * time.Hour)
		if end.After(to) {
			end = to
		}
		data, err := l.Store.Aggregate(ctx, start, end, true)
		if err != nil {
			return err
		}
		if err := l.Store.SaveHourly(ctx, start, end, data); err != nil {
			return err
		}
	}
	if !l.backfill {
		l.backfill = true
		if err := l.Store.PruneHourly(ctx, now.Add(-trafficRetention)); err != nil {
			log.Printf("[plus.dispatch] prune traffic: %v", err)
		}
	}
	return nil
}

// Load 读最近一个月的小时桶
func (l *TrafficLoader) Load(ctx context.Context, now time.Time) (*TrafficSet, error) {
	h, err := l.Store.LoadHourly(ctx, now.Truncate(time.Hour).Add(-trafficWindow), 0)
	if err != nil {
		return nil, err
	}
	rows := make([]TRow, 0, 4096)
	for k, m := range h {
		for b, a := range m {
			rows = append(rows, TRow{Bucket: b, TKey: k, Agg: a})
		}
	}
	return NewTrafficSet(now, rows), nil
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
