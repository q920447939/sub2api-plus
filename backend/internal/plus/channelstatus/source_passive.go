// Package channelstatus — [plus] 被动信号来源：只读上游 V2 预聚合表（真实调用统计），不写任何上游表。
//
// 注意：V2 聚合器只在上游 channel_monitor_enabled=true 且 mode=v2 时运行，
// 因此上游开关必须保持开启，只隐藏上游 V1/V2 的页面入口。
package channelstatus

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

// v2TaxonomyVersion 与上游 service.ChannelMonitorV2TaxonomyVersion 保持一致
const v2TaxonomyVersion = 1

// passiveTier 一档数据表；按请求的桶大小选择最粗且能对齐的那一档
type passiveTier struct {
	maxBucket     time.Duration // 桶大小 < maxBucket 时使用此档
	metrics       string
	errors        string
	histograms    string
	rollupSeconds int // 0 表示 1 分钟表（无 bucket_seconds 列）
}

// tiers 由细到粗。周视图 84 天超过 1 小时档 30 天保留期，只能用 1 天档（保留 90 天）
var passiveTiers = []passiveTier{
	{maxBucket: time.Hour, metrics: "channel_monitor_v2_metrics_1m", errors: "channel_monitor_v2_error_metrics_1m", histograms: "channel_monitor_v2_latency_histograms_1m"},
	{maxBucket: 7 * 24 * time.Hour, metrics: "channel_monitor_v2_metrics_rollup", errors: "channel_monitor_v2_error_metrics_rollup", histograms: "channel_monitor_v2_latency_histograms_rollup", rollupSeconds: 3600},
	{maxBucket: 1<<63 - 1, metrics: "channel_monitor_v2_metrics_rollup", errors: "channel_monitor_v2_error_metrics_rollup", histograms: "channel_monitor_v2_latency_histograms_rollup", rollupSeconds: 86400},
}

func tierFor(bucket time.Duration) passiveTier {
	for _, t := range passiveTiers {
		if bucket < t.maxBucket {
			return t
		}
	}
	return passiveTiers[len(passiveTiers)-1]
}

// IgnoredCategoriesFunc 返回当前生效的忽略错误分类（来自配置，按次读取，避免数据源持有配置）
type IgnoredCategoriesFunc func(ctx context.Context) []string

// PassiveSource 读 V2 rollup 的 SignalSource 实现
type PassiveSource struct {
	db      *sql.DB
	ignored IgnoredCategoriesFunc
}

func NewPassiveSource(db *sql.DB, ignored IgnoredCategoriesFunc) *PassiveSource {
	return &PassiveSource{db: db, ignored: ignored}
}

var _ SignalSource = (*PassiveSource)(nil)

type sigKey struct {
	group  int64
	bucket int64
}

type sigAcc struct {
	success, errors, ignored      int64
	input, cacheCreate, cacheRead int64
	hist                          map[int64]int64
}

// hourlyRollupSpan 小时汇总保留 30 天，留 1 天余量
const hourlyRollupSpan = 29 * 24 * time.Hour

// segment 一段时间范围及其读取的数据档
type segment struct {
	tier       passiveTier
	start, end time.Time
}

// planSegments 选出读取哪些数据档。
// 上游平时刷新会跳过 1 天汇总（当天的数据要跨过 UTC 日界才写入），且 1 天汇总按 UTC 零点切分。
// 所以需要 1 天档时，最近 29 天改读小时汇总（持续刷新、能按本地时区对齐），更早的部分才读 1 天汇总；
// 分界点取 UTC 零点，保证两段不重叠、不重复计数。更早的周仍有 UTC 与本地零点的偏差，可接受。
func planSegments(start, end time.Time, bucket time.Duration) []segment {
	tier := tierFor(bucket)
	if tier.rollupSeconds != 86400 {
		return []segment{{tier, start, end}}
	}
	hourly := passiveTiers[1]
	split := end.Add(-hourlyRollupSpan).UTC().Truncate(24 * time.Hour)
	if !split.After(start) {
		return []segment{{hourly, start, end}}
	}
	return []segment{{tier, start, split}, {hourly, split, end}}
}

// Fetch 按 date_bin(bucket, bucket_start, origin=start) 分桶；调用方需保证 start 已对齐
func (s *PassiveSource) Fetch(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]Signal, error) {
	out := map[int64]map[int64]Signal{}
	if len(groupIDs) == 0 || !end.After(start) || bucket <= 0 {
		return out, nil
	}
	accs := map[sigKey]*sigAcc{}
	cats := s.ignoredCategories(ctx)
	for _, seg := range planSegments(start, end, bucket) {
		if err := s.accumulate(ctx, accs, seg, groupIDs, start, bucket, cats); err != nil {
			return nil, err
		}
	}
	for k, a := range accs {
		sig := a.signal()
		if sig.Requests <= 0 {
			continue
		}
		if out[k.group] == nil {
			out[k.group] = map[int64]Signal{}
		}
		out[k.group][k.bucket] = sig
	}
	return out, nil
}

// accumulate 读一段数据累加进 accs；origin 是整体起点，各段共用以保证分桶一致
func (s *PassiveSource) accumulate(ctx context.Context, accs map[sigKey]*sigAcc, seg segment, groupIDs []int64, origin time.Time, bucket time.Duration, cats []string) error {
	tier := seg.tier
	get := func(g int64, b time.Time) *sigAcc {
		k := sigKey{g, b.Unix()}
		a := accs[k]
		if a == nil {
			a = &sigAcc{}
			accs[k] = a
		}
		return a
	}

	// $1 interval, $2 origin, $3 groups, $4 start, $5 end [, $6 bucket_seconds]
	args := []any{fmt.Sprintf("%d seconds", int64(bucket/time.Second)), origin.UTC(), pq.Array(groupIDs), seg.start.UTC(), seg.end.UTC()}
	where := `t.group_id = ANY($3) AND t.bucket_start >= $4 AND t.bucket_start < $5`
	if tier.rollupSeconds > 0 {
		args = append(args, tier.rollupSeconds)
		where += ` AND t.bucket_seconds = $6`
	}
	bin := `date_bin($1::interval, t.bucket_start, $2::timestamptz)`

	// 1) 请求量 / 错误量 / 缓存相关 token
	rows, err := s.db.QueryContext(ctx, `SELECT t.group_id, `+bin+`,
		SUM(t.success_requests), SUM(t.error_requests),
		SUM(t.input_tokens), SUM(t.cache_creation_tokens), SUM(t.cache_read_tokens)
		FROM `+tier.metrics+` t WHERE `+where+` GROUP BY 1, 2`, args...)
	if err != nil {
		return fmt.Errorf("channelstatus: load metrics: %w", err)
	}
	if err := scanRows(rows, func() error {
		var (
			g                                 int64
			b                                 time.Time
			succ, errs, input, cCreate, cRead int64
		)
		if err := rows.Scan(&g, &b, &succ, &errs, &input, &cCreate, &cRead); err != nil {
			return err
		}
		a := get(g, b)
		a.success += succ
		a.errors += errs
		a.input += input
		a.cacheCreate += cCreate
		a.cacheRead += cRead
		return nil
	}); err != nil {
		return err
	}

	// 2) 被忽略的错误分类（只从错误率分子扣除，与 V2 一致）
	if len(cats) > 0 {
		n := len(args)
		eArgs := append(append([]any{}, args...), pq.Array(cats), v2TaxonomyVersion)
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT t.group_id, %s, SUM(t.error_requests)
			FROM %s t WHERE %s AND t.error_category = ANY($%d) AND t.taxonomy_version = $%d GROUP BY 1, 2`,
			bin, tier.errors, where, n+1, n+2), eArgs...)
		if err != nil {
			return fmt.Errorf("channelstatus: load ignored errors: %w", err)
		}
		if err := scanRows(rows, func() error {
			var (
				g   int64
				b   time.Time
				cnt int64
			)
			if err := rows.Scan(&g, &b, &cnt); err != nil {
				return err
			}
			get(g, b).ignored += cnt
			return nil
		}); err != nil {
			return err
		}
	}

	// 3) TTFT 直方图（user_id=0 为全体受众）
	rows, err = s.db.QueryContext(ctx, `SELECT t.group_id, `+bin+`, t.upper_bound_ms, SUM(t.sample_count)
		FROM `+tier.histograms+` t WHERE `+where+` AND t.metric = 'ttft' AND t.user_id = 0 GROUP BY 1, 2, 3`, args...)
	if err != nil {
		return fmt.Errorf("channelstatus: load histograms: %w", err)
	}
	return scanRows(rows, func() error {
		var (
			g          int64
			b          time.Time
			bound, cnt int64
		)
		if err := rows.Scan(&g, &b, &bound, &cnt); err != nil {
			return err
		}
		a := get(g, b)
		if a.hist == nil {
			a.hist = map[int64]int64{}
		}
		a.hist[bound] += cnt
		return nil
	})
}

func (s *PassiveSource) ignoredCategories(ctx context.Context) []string {
	if s.ignored == nil {
		return nil
	}
	return s.ignored(ctx)
}

func (a *sigAcc) signal() Signal {
	req := a.success + a.errors
	errs := a.errors - a.ignored
	if errs < 0 {
		errs = 0
	}
	sig := Signal{Requests: req, Errors: errs}
	sig.TTFTP50Ms, sig.TTFTSamples = histPercentile(a.hist, 0.5)
	// 缓存命中率口径与 V2 一致：cache_read / (input + cache_creation + cache_read)
	if denom := a.input + a.cacheCreate + a.cacheRead; denom > 0 {
		sig.CacheDenom = denom
		sig.CacheRate = float64(a.cacheRead) / float64(denom)
	}
	return sig
}

// histPercentile 与 V2 一致：按累计计数取分桶上界；返回分位值与样本总数
func histPercentile(hist map[int64]int64, p float64) (*int64, int64) {
	var total int64
	keys := make([]int64, 0, len(hist))
	for bound, cnt := range hist {
		keys = append(keys, bound)
		total += cnt
	}
	if total == 0 {
		return nil, 0
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	target := int64(float64(total)*p + 0.999999)
	var cum int64
	for _, bound := range keys {
		cum += hist[bound]
		if cum >= target {
			v := bound
			return &v, total
		}
	}
	v := keys[len(keys)-1]
	return &v, total
}

func scanRows(rows *sql.Rows, fn func() error) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := fn(); err != nil {
			return err
		}
	}
	return rows.Err()
}
