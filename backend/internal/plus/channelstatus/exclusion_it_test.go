//go:build integration

package channelstatus

import (
	"context"
	"testing"
	"time"
)

func TestExcluded_RollupAndCounter(t *testing.T) {
	db := openTempDB(t)
	for _, q := range []string{
		`CREATE TEMP TABLE ops_error_logs (id BIGSERIAL, created_at TIMESTAMPTZ, group_id BIGINT, request_id TEXT,
			status_code INT, is_business_limited BOOL, error_owner TEXT, is_count_tokens BOOL DEFAULT FALSE)`,
		`CREATE TEMP TABLE plus_channel_status_excluded_1h (bucket_start TIMESTAMPTZ, group_id BIGINT, error_requests BIGINT,
			computed_at TIMESTAMPTZ, PRIMARY KEY (bucket_start, group_id))`,
		`CREATE TEMP TABLE plus_channel_status_excluded_cursor (id SMALLINT PRIMARY KEY, backfill_before TIMESTAMPTZ, updated_at TIMESTAMPTZ DEFAULT NOW())`,
	} {
		mustExec(t, db, q)
	}
	now := time.Date(2026, 11, 4, 10, 30, 0, 0, time.UTC)
	old := now.Add(-30 * time.Hour)
	mustExec(t, db, `INSERT INTO ops_error_logs (created_at, group_id, request_id, status_code, is_business_limited, error_owner) VALUES
		($1, 11, 'a', 403, true, 'client'),
		($1, 11, 'a', 403, true, 'client'),      -- 同一请求只算一次
		($1, 11, 'b', 403, true, 'client'),
		($1, 11, 'c', 503, true, 'platform'),    -- 平台侧（无可用账号）不排除
		($1, 11, 'd', 502, false, 'provider'),   -- 真实上游错误不排除
		($2, 11, 'e', 403, true, 'client')`, now.Add(-10*time.Minute), old)

	p := NewExcludedProducer(db, nil)
	p.now = func() time.Time { return now }
	for i := 0; i < 3; i++ { // 首轮初始化游标，之后补算
		if err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	c := NewDBExcludedCounter(db)
	// 当前窗口：直接查原始表
	start := now.Add(-15 * time.Minute)
	got, err := c.Count(context.Background(), []int64{11}, start, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got[11][start.Unix()] != 2 {
		t.Fatalf("当前窗口应排除 2, got %v", got)
	}
	// 历史：读小时汇总，含补算出来的 30 小时前那一条
	dayStart := now.Truncate(24 * time.Hour).Add(-48 * time.Hour)
	got, err = c.Count(context.Background(), []int64{11}, dayStart, now.Add(time.Hour), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, n := range got[11] {
		total += n
	}
	if total != 3 {
		t.Fatalf("历史应合计 3（今天 2 + 补算 1）, got %v", got)
	}
}
