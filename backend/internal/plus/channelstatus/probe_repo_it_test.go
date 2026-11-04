//go:build integration

package channelstatus

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// 探测状态表的 CAS 抢占、每日上限、跨天重置、Finish 回写；表结构直接取自迁移文件，用 TEMP 表遮蔽
func TestProbeRepo_ClaimCASAndDailyLimit(t *testing.T) {
	db := openTempDB(t)
	ddl, err := os.ReadFile("../../../migrations/256_plus_channel_status_probe.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, strings.Replace(string(ddl), "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1))
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC)

	claim := func(prev *time.Time, at time.Time, day string, limit int, force bool) bool {
		ok, err := repo.Claim(ctx, 1, prev, at, day, limit, force)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !claim(nil, now, "2026-11-04", 2, false) {
		t.Fatal("首次应抢到")
	}
	if claim(nil, now.Add(time.Second), "2026-11-04", 2, false) {
		t.Fatal("prevStarted 不一致（另一实例已抢）应失败")
	}
	if err := repo.Finish(ctx, 1, now.Add(2*time.Second), true, 1234, "", 3); err != nil {
		t.Fatal(err)
	}
	st, err := repo.LoadStates(ctx, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	s := st[1]
	if s == nil || s.LastOK == nil || !*s.LastOK || *s.LatencyMs != 1234 || s.IdleStreak != 3 || s.Day != "2026-11-04" || s.DayCount != 1 {
		t.Fatalf("状态回写不对: %+v", s)
	}
	prev := *s.LastStartedAt
	t2 := now.Add(time.Hour)
	if !claim(&prev, t2, "2026-11-04", 2, false) {
		t.Fatal("prevStarted 一致应抢到第二次")
	}
	if claim(&t2, now.Add(2*time.Hour), "2026-11-04", 2, false) {
		t.Fatal("已达每日上限 2 次应失败")
	}
	if !claim(&t2, now.Add(24*time.Hour), "2026-11-05", 2, false) {
		t.Fatal("跨天后应重置计数")
	}
	if !claim(nil, now.Add(25*time.Hour), "2026-11-05", 1, true) {
		t.Fatal("force 应跳过 CAS 与上限")
	}
}
