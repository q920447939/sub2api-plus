//go:build integration

package channelstatus

// 集成测试：PLUS_CHANNEL_STATUS_IT_DSN 指向任意 PostgreSQL（14+）。
// 全部使用会话级 TEMP 表（同名遮蔽真实表），连接关闭即消失，不写任何真实数据。

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func openTempDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("PLUS_CHANNEL_STATUS_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_CHANNEL_STATUS_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // 临时表只在本连接可见
	t.Cleanup(func() { _ = db.Close() })
	ddl := []string{
		`CREATE TEMP TABLE channel_monitor_v2_metrics_1m (bucket_start TIMESTAMPTZ, platform TEXT, group_id BIGINT, model TEXT,
			success_requests BIGINT DEFAULT 0, error_requests BIGINT DEFAULT 0, input_tokens BIGINT DEFAULT 0,
			cache_creation_tokens BIGINT DEFAULT 0, cache_read_tokens BIGINT DEFAULT 0)`,
		`CREATE TEMP TABLE channel_monitor_v2_error_metrics_1m (bucket_start TIMESTAMPTZ, platform TEXT, group_id BIGINT, model TEXT,
			error_category TEXT, taxonomy_version SMALLINT, error_requests BIGINT)`,
		`CREATE TEMP TABLE channel_monitor_v2_latency_histograms_1m (bucket_start TIMESTAMPTZ, platform TEXT, group_id BIGINT, model TEXT,
			user_id BIGINT, metric TEXT, upper_bound_ms INTEGER, sample_count BIGINT)`,
		`CREATE TEMP TABLE channel_monitor_v2_metrics_rollup (bucket_start TIMESTAMPTZ, bucket_seconds INTEGER, platform TEXT, group_id BIGINT, model TEXT,
			success_requests BIGINT DEFAULT 0, error_requests BIGINT DEFAULT 0, input_tokens BIGINT DEFAULT 0,
			cache_creation_tokens BIGINT DEFAULT 0, cache_read_tokens BIGINT DEFAULT 0)`,
		`CREATE TEMP TABLE channel_monitor_v2_error_metrics_rollup (bucket_start TIMESTAMPTZ, bucket_seconds INTEGER, platform TEXT, group_id BIGINT, model TEXT,
			error_category TEXT, taxonomy_version SMALLINT, error_requests BIGINT)`,
		`CREATE TEMP TABLE channel_monitor_v2_latency_histograms_rollup (bucket_start TIMESTAMPTZ, bucket_seconds INTEGER, platform TEXT, group_id BIGINT, model TEXT,
			user_id BIGINT, metric TEXT, upper_bound_ms INTEGER, sample_count BIGINT)`,
	}
	for _, q := range ddl {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
}

func TestPassiveSource_CurrentWindow1m(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 10, 30, 0, 0, time.UTC)
	start := now.Add(-15 * time.Minute)

	// 分组 7 两个模型、两个平台（混合分组）都要合并；窗口外数据不计
	mustExec(t, db, `INSERT INTO channel_monitor_v2_metrics_1m VALUES
		($1, 'anthropic', 7, 'm1', 40, 10, 600, 100, 300),
		($2, 'openai',    7, 'm2', 50,  0, 0,   0,   0),
		($3, 'anthropic', 7, 'm1', 999, 999, 0, 0, 0),
		($1, 'anthropic', 8, 'm1', 5, 0, 0, 0, 0)`,
		start.Add(time.Minute), now.Add(-time.Minute), start.Add(-time.Minute))
	mustExec(t, db, `INSERT INTO channel_monitor_v2_error_metrics_1m VALUES
		($1, 'anthropic', 7, 'm1', 'client_cancelled', 1, 4),
		($1, 'anthropic', 7, 'm1', 'timeout', 1, 6),
		($1, 'anthropic', 7, 'm1', 'client_cancelled', 2, 99)`, start.Add(time.Minute))
	mustExec(t, db, `INSERT INTO channel_monitor_v2_latency_histograms_1m VALUES
		($1, 'anthropic', 7, 'm1', 0, 'ttft', 1000, 30),
		($1, 'anthropic', 7, 'm1', 0, 'ttft', 5000, 60),
		($1, 'anthropic', 7, 'm1', 42, 'ttft', 100, 500),
		($1, 'anthropic', 7, 'm1', 0, 'duration', 100, 500)`, start.Add(time.Minute))

	src := NewPassiveSource(db, func(context.Context) []string { return []string{"client_cancelled"} })
	got, err := src.Fetch(ctx, []int64{7, 8}, start, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s := got[7][start.Unix()]
	if s.Requests != 100 || s.Errors != 6 {
		t.Fatalf("请求 100 / 有效错误 10-4=6, got %+v", s)
	}
	if s.TTFTP50Ms == nil || *s.TTFTP50Ms != 5000 || s.TTFTSamples != 90 {
		t.Fatalf("只用 user_id=0 的 ttft 直方图，p50=5000/90, got %v/%d", s.TTFTP50Ms, s.TTFTSamples)
	}
	if s.CacheDenom != 1000 || s.CacheRate != 0.3 {
		t.Fatalf("缓存 300/1000, got %d %v", s.CacheDenom, s.CacheRate)
	}
	if got[8][start.Unix()].Requests != 5 {
		t.Fatalf("分组 8 应有 5 次请求: %+v", got[8])
	}
}

func TestPassiveSource_DayBinsFromHourlyRollup(t *testing.T) {
	db := openTempDB(t)
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 11, 4, 10, 30, 0, 0, loc)
	tl := BuildTimeline(GranDay, now, loc)

	today := tl.BucketStart(tl.Count - 1)
	// 今天 01:00 与 09:00（东八区）落在同一天；昨天 23:00 落在前一天；1 天档数据不应被小时/天视图读取
	mustExec(t, db, `INSERT INTO channel_monitor_v2_metrics_rollup VALUES
		($1, 3600, 'anthropic', 7, 'm', 10, 0, 0, 0, 0),
		($2, 3600, 'anthropic', 7, 'm', 20, 0, 0, 0, 0),
		($3, 3600, 'anthropic', 7, 'm', 5, 0, 0, 0, 0),
		($1, 86400, 'anthropic', 7, 'm', 1000, 0, 0, 0, 0)`,
		today.Add(time.Hour), today.Add(9*time.Hour), today.Add(-time.Hour))

	got, err := NewPassiveSource(db, nil).Fetch(context.Background(), []int64{7}, tl.Start, tl.End, tl.Bucket)
	if err != nil {
		t.Fatal(err)
	}
	if got[7][today.Unix()].Requests != 30 {
		t.Fatalf("今天应为 30, got %+v", got[7])
	}
	if got[7][today.Add(-24*time.Hour).Unix()].Requests != 5 {
		t.Fatalf("昨天应为 5, got %+v", got[7])
	}
}

func TestPassiveSource_WeekRecentHourlyOlderDaily(t *testing.T) {
	db := openTempDB(t)
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 11, 4, 10, 30, 0, 0, loc) // 周三
	tl := BuildTimeline(GranWeek, now, loc)
	week := tl.BucketStart(tl.Count - 1) // 本周一 00:00 CST
	old := tl.BucketStart(tl.Count - 9)  // 8 周前，超出 29 天，走 1 天档

	// 本周：小时档 周一 01:00 CST（UTC 前一天 17:00）+ 今天；1 天档的行应被忽略（最近 29 天只读小时档）
	mustExec(t, db, `INSERT INTO channel_monitor_v2_metrics_rollup VALUES
		($1, 3600,  'openai', 7, 'm', 4, 0, 0, 0, 0),
		($2, 3600,  'openai', 7, 'm', 6, 0, 0, 0, 0),
		($3, 86400, 'openai', 7, 'm', 999, 0, 0, 0, 0)`,
		week.Add(time.Hour), now.Truncate(time.Hour), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	// 8 周前：1 天档（按 UTC 0 点存储，即 08:00 CST）；同周的小时档行应被忽略
	mustExec(t, db, `INSERT INTO channel_monitor_v2_metrics_rollup VALUES
		($1, 86400, 'openai', 7, 'm', 7, 0, 0, 0, 0),
		($1, 3600,  'openai', 7, 'm', 500, 0, 0, 0, 0)`, old.Add(8*time.Hour))

	got, err := NewPassiveSource(db, nil).Fetch(context.Background(), []int64{7}, tl.Start, tl.End, tl.Bucket)
	if err != nil {
		t.Fatal(err)
	}
	if r := got[7][week.Unix()].Requests; r != 10 {
		t.Fatalf("本周应只算小时档 = 10, got %d", r)
	}
	if r := got[7][old.Unix()].Requests; r != 7 {
		t.Fatalf("8 周前应只算 1 天档 = 7, got %d", r)
	}
}
