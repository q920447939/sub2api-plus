package dispatch

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"sort"
	"time"
)

// Agg 一段时间内（分组 × 账号 × 模型）的真实流量汇总
type Agg struct {
	Requests   int64   `json:"requests"` // 成功
	Errors     int64   `json:"errors"`   // 上游 5xx / 超时 / 连接中断（不含用户侧错误、429）
	Limited    int64   `json:"limited"`  // 上游 429
	TTFTP90    float64 `json:"ttft_p90"` // 毫秒
	TTFTN      int64   `json:"ttft_n"`
	OutTokens  int64   `json:"out_tokens"`
	GenMs      int64   `json:"gen_ms"`
	CacheRead  int64   `json:"cache_read"`
	CacheTotal int64   `json:"cache_total"`
}

// Total 所有请求（成功 + 出错 + 限流）
func (a Agg) Total() int64 { return a.Requests + a.Errors + a.Limited }

// Add 合并（p90 按样本数加权，近似值）
func (a Agg) Add(b Agg) Agg {
	out := Agg{
		Requests: a.Requests + b.Requests, Errors: a.Errors + b.Errors, Limited: a.Limited + b.Limited,
		TTFTN: a.TTFTN + b.TTFTN, OutTokens: a.OutTokens + b.OutTokens, GenMs: a.GenMs + b.GenMs,
		CacheRead: a.CacheRead + b.CacheRead, CacheTotal: a.CacheTotal + b.CacheTotal,
	}
	if out.TTFTN > 0 {
		out.TTFTP90 = (a.TTFTP90*float64(a.TTFTN) + b.TTFTP90*float64(b.TTFTN)) / float64(out.TTFTN)
	}
	return out
}

// TKey 流量的统计对象
type TKey struct {
	Group   int64
	Account int64
	Model   string
}

// TrafficSet 判定用的流量数据：最近 8 天的小时桶 + 最近 60 分钟的滚动值
type TrafficSet struct {
	Hourly map[TKey]map[int64]Agg // bucket(unix 秒) → 汇总
	Now    map[TKey]Agg
	NowAt  time.Time
}

func newTrafficSet(now time.Time) *TrafficSet {
	return &TrafficSet{Hourly: map[TKey]map[int64]Agg{}, Now: map[TKey]Agg{}, NowAt: now}
}

// Range 某对象在 [from, to) 的小时桶合计
func (t *TrafficSet) Range(k TKey, from, to time.Time) Agg {
	var out Agg
	for b, a := range t.Hourly[k] {
		if b >= from.Unix() && b < to.Unix() {
			out = out.Add(a)
		}
	}
	return out
}

// Keys 所有对象
func (t *TrafficSet) Keys() []TKey {
	seen := map[TKey]bool{}
	var out []TKey
	for k := range t.Hourly {
		seen[k] = true
		out = append(out, k)
	}
	for k := range t.Now {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// GroupShares 分组里各账号近 24 小时的请求占比（%）
func (t *TrafficSet) GroupShares(groupID int64) (map[int64]float64, int64) {
	from := t.NowAt.Add(-24 * time.Hour)
	per := map[int64]int64{}
	var total int64
	for _, k := range t.Keys() {
		if k.Group != groupID {
			continue
		}
		n := t.Range(k, from.Truncate(time.Hour), t.NowAt.Add(time.Hour)).Total()
		per[k.Account] += n
		total += n
	}
	out := map[int64]float64{}
	for a, n := range per {
		if total > 0 {
			out[a] = float64(n) * 100 / float64(total)
		}
	}
	return out, total
}

// GroupModelShares 分组里各模型近 24 小时的请求占比（%）
func (t *TrafficSet) GroupModelShares(groupID int64) map[string]float64 {
	from := t.NowAt.Add(-24 * time.Hour).Truncate(time.Hour)
	per := map[string]int64{}
	var total int64
	for _, k := range t.Keys() {
		if k.Group != groupID {
			continue
		}
		n := t.Range(k, from, t.NowAt.Add(time.Hour)).Total()
		per[k.Model] += n
		total += n
	}
	out := map[string]float64{}
	for m, n := range per {
		if total > 0 && n > 0 {
			out[m] = float64(n) * 100 / float64(total)
		}
	}
	return out
}

// ---------- 指标定义（流量统计类型的「指标库」） ----------

// Metric 一个流量指标
type Metric struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Unit   string `json:"unit"`   // % | s | tok/s
	Better string `json:"better"` // low | high
	Dec    int    `json:"dec"`
	Desc   string `json:"desc"`
	// Counted 指标是「出问题的请求占比」：判定时还要求出问题的请求够一定次数（避免 1 次偶发就报警）
	Counted bool `json:"counted"`
	Rule    TrafficRule
	value   func(a Agg) (float64, bool)
	count   func(a Agg) int64
}

// ProblemCount 出问题的请求次数（不是计数类指标时 ok=false）
func (m Metric) ProblemCount(a Agg) (int64, bool) {
	if m.count == nil {
		return 0, false
	}
	return m.count(a), true
}

// Value 指标值；样本不够算不出时 ok=false
func (m Metric) Value(a Agg) (float64, bool) { return m.value(a) }

// Format 带单位的展示
func (m Metric) Format(v float64) string {
	switch m.Unit {
	case "%":
		return fmt.Sprintf("%.*f%%", m.Dec, v)
	case "s":
		return fmt.Sprintf("%.1f 秒", v)
	}
	return fmt.Sprintf("%.0f %s", v, m.Unit)
}

// relBases 只能和别人 / 自己的历史比的指标
func relBases() []string { return []string{"peer", "site", "self"} }

// absBases 出错类指标：都比不了时按绝对值判（固定值 0，即「比 0 高几个百分点」）
func absBases() []string { return []string{"peer", "site", "self", "fixed"} }

var metrics = []Metric{
	{Key: "err", Name: "报错率", Unit: "%", Better: "low", Dec: 1,
		Desc: "上游返回 5xx、超时、连接中断的请求占比。用户自己造成的错误（余额不足、参数错）和 429 不算。",
		Rule: TrafficRule{Bases: absBases(), Fixed: fptr(0), MinPeers: 1, MinSamples: 50, Duration: 60, Mode: "diff", Warn: 2, Bad: 5, Floor: fptr(1), MinEvents: 3}, Counted: true,
		count: func(a Agg) int64 { return a.Errors },
		value: func(a Agg) (float64, bool) {
			if a.Total() == 0 {
				return 0, false
			}
			return float64(a.Errors) * 100 / float64(a.Total()), true
		}},
	{Key: "r429", Name: "限流率", Unit: "%", Better: "low", Dec: 1,
		Desc: "上游返回 429 的请求占比，说明号池容量紧张。",
		Rule: TrafficRule{Bases: absBases(), Fixed: fptr(0), MinPeers: 1, MinSamples: 50, Duration: 60, Mode: "diff", Warn: 5, Bad: 12, Floor: fptr(2), MinEvents: 3}, Counted: true,
		count: func(a Agg) int64 { return a.Limited },
		value: func(a Agg) (float64, bool) {
			if a.Total() == 0 {
				return 0, false
			}
			return float64(a.Limited) * 100 / float64(a.Total()), true
		}},
	{Key: "ttft", Name: "首字耗时 p90", Unit: "s", Better: "low", Dec: 1,
		Desc: "流式请求从发出到收到第一个字的时间，取最慢的 10%（p90），比平均值更能反映卡顿。",
		Rule: TrafficRule{Bases: relBases(), MinPeers: 1, MinSamples: 50, Duration: 60, Mode: "ratio", Warn: 1.5, Bad: 2, Floor: fptr(3)},
		value: func(a Agg) (float64, bool) {
			if a.TTFTN == 0 {
				return 0, false
			}
			return a.TTFTP90 / 1000, true
		}},
	{Key: "tps", Name: "输出速度", Unit: "tok/s", Better: "high", Dec: 0,
		Desc: "开始输出以后每秒输出多少 token（输出 token ÷ 输出用时）。",
		Rule: TrafficRule{Bases: relBases(), MinPeers: 1, MinSamples: 50, Duration: 60, Mode: "pct", Warn: 70, Bad: 50},
		value: func(a Agg) (float64, bool) {
			if a.GenMs <= 0 || a.OutTokens <= 0 {
				return 0, false
			}
			return float64(a.OutTokens) / (float64(a.GenMs) / 1000), true
		}},
	{Key: "cache", Name: "缓存命中率", Unit: "%", Better: "high", Dec: 0,
		Desc: "输入里命中缓存的比例（缓存读取 ÷ 全部输入）。反映上游号池能不能把同一会话留在同一个底层账号上。",
		Rule: TrafficRule{Bases: relBases(), MinPeers: 1, MinSamples: 50, Duration: 60, Mode: "diff", Warn: 15, Bad: 30},
		value: func(a Agg) (float64, bool) {
			if a.CacheTotal <= 0 {
				return 0, false
			}
			return float64(a.CacheRead) * 100 / float64(a.CacheTotal), true
		}},
}

func metricByKey(k string) (Metric, bool) {
	for _, m := range metrics {
		if m.Key == k {
			return m, true
		}
	}
	return Metric{}, false
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
         COALESCE(SUM(output_tokens) FILTER (WHERE output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0)), 0) AS ot,
         COALESCE(SUM(duration_ms - COALESCE(first_token_ms, 0)) FILTER (WHERE output_tokens > 0 AND duration_ms > COALESCE(first_token_ms, 0)), 0) AS gm,
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
       COALESCE(s.ot, 0), COALESCE(s.gm, 0), COALESCE(s.cr, 0), COALESCE(s.ct, 0)
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
			&a.OutTokens, &a.GenMs, &a.CacheRead, &a.CacheTotal); err != nil {
			return nil, err
		}
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
		(bucket, group_id, account_id, model, requests, errors, limited, ttft_p90, ttft_n, out_tokens, gen_ms, cache_read, cache_total)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (bucket, group_id, account_id, model) DO UPDATE SET requests = EXCLUDED.requests, errors = EXCLUDED.errors,
		  limited = EXCLUDED.limited, ttft_p90 = EXCLUDED.ttft_p90, ttft_n = EXCLUDED.ttft_n, out_tokens = EXCLUDED.out_tokens,
		  gen_ms = EXCLUDED.gen_ms, cache_read = EXCLUDED.cache_read, cache_total = EXCLUDED.cache_total`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, m := range data {
		for b, a := range m {
			var p90 any
			if a.TTFTN > 0 {
				p90 = a.TTFTP90
			}
			if _, err := stmt.ExecContext(ctx, time.Unix(b, 0), k.Group, k.Account, k.Model, a.Requests, a.Errors, a.Limited,
				p90, a.TTFTN, a.OutTokens, a.GenMs, a.CacheRead, a.CacheTotal); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (t SQLTraffic) LoadHourly(ctx context.Context, from time.Time, groupID int64) (map[TKey]map[int64]Agg, error) {
	q := `SELECT bucket, group_id, account_id, model, requests, errors, limited, COALESCE(ttft_p90, 0), ttft_n,
	             out_tokens, gen_ms, cache_read, cache_total
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
			&a.OutTokens, &a.GenMs, &a.CacheRead, &a.CacheTotal); err != nil {
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
	trafficHistory   = 8 * 24 * time.Hour
	trafficRetention = 35 * 24 * time.Hour
)

// Refresh 重算最近两个小时的小时桶；第一次运行时回填 8 天
func (l *TrafficLoader) Refresh(ctx context.Context, now time.Time) error {
	from := now.Truncate(time.Hour).Add(-time.Hour)
	if !l.backfill {
		from = now.Truncate(time.Hour).Add(-trafficHistory)
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

// Load 读最近 8 天的小时桶 + 最近 60 分钟滚动值
func (l *TrafficLoader) Load(ctx context.Context, now time.Time) (*TrafficSet, error) {
	ts := newTrafficSet(now)
	h, err := l.Store.LoadHourly(ctx, now.Truncate(time.Hour).Add(-trafficHistory), 0)
	if err != nil {
		return nil, err
	}
	ts.Hourly = h
	cur, err := l.Store.Aggregate(ctx, now.Add(-time.Hour), now, false)
	if err != nil {
		return nil, err
	}
	for k, m := range cur {
		var a Agg
		for _, v := range m {
			a = a.Add(v)
		}
		ts.Now[k] = a
	}
	return ts, nil
}

func median(vs []float64) (float64, bool) {
	if len(vs) == 0 {
		return 0, false
	}
	s := append([]float64(nil), vs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m], true
	}
	return (s[m-1] + s[m]) / 2, true
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

const timeHour = time.Hour
