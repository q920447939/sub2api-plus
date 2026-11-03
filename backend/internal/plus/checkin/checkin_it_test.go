//go:build integration

package checkin

// 集成测试：需要已应用 248_plus_points.sql 与 249_plus_checkin.sql 的独立库。
// 运行：PLUS_CHECKIN_IT_DSN='postgres://...' go test -tags integration ./internal/plus/checkin -run IT -v
// 测试会清空 plus_points_*/plus_checkin_*/users 表，切勿指向业务库。

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	_ "github.com/lib/pq"
)

func openIT(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PLUS_CHECKIN_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_CHECKIN_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tbl := range []string{
		"plus_slot_feed", "plus_slot_pot_ledger", "plus_slot_spins", "plus_points_redeem_grants",
		"plus_checkin_ip_logs", "plus_checkin_logs", "plus_checkin_config",
		"plus_points_ledger", "plus_points_accounts", "plus_points_config",
		"users",
	} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	return db
}

func seedUser(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
		 VALUES ($1, 'x', 0, 'user', NOW(), NOW()) RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// ITCheckinOnce 基本签到流程：首次签到得10分，连签=1
func TestITCheckinOnce(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db, "it_checkin1@example.com")

	pointsSvc := points.NewService(points.NewRepository(db))
	svc := NewService(NewRepository(db), pointsSvc)

	res, err := svc.Checkin(ctx, uid, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if res.Streak != 1 {
		t.Fatalf("streak: want 1, got %d", res.Streak)
	}
	if res.PointsGranted != 10 {
		t.Fatalf("points: want 10, got %d", res.PointsGranted)
	}
	if res.Balance != 10 {
		t.Fatalf("balance: want 10, got %d", res.Balance)
	}

	// 再签一次应该报错
	_, err = svc.Checkin(ctx, uid, "1.2.3.4")
	if err != ErrAlreadyCheckedIn {
		t.Fatalf("want ErrAlreadyCheckedIn, got %v", err)
	}
}

// ITCheckinStreak 连签第2天 streak=2 得20分
func TestITCheckinStreak(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db, "it_checkin2@example.com")

	pointsSvc := points.NewService(points.NewRepository(db))
	svc := NewService(NewRepository(db), pointsSvc)

	// Day 1
	res1, err := svc.Checkin(ctx, uid, "")
	if err != nil {
		t.Fatalf("day1: %v", err)
	}
	if res1.Streak != 1 || res1.PointsGranted != 10 {
		t.Fatalf("day1: streak=%d pts=%d", res1.Streak, res1.PointsGranted)
	}

	// 把今日签到记录的 checked_date 改成昨天，模拟昨日已签
	if _, err := db.Exec(
		`UPDATE plus_checkin_logs SET checked_date = CURRENT_DATE - 1 WHERE user_id = $1`, uid); err != nil {
		t.Fatal(err)
	}

	// Day 2
	res2, err := svc.Checkin(ctx, uid, "")
	if err != nil {
		t.Fatalf("day2: %v", err)
	}
	if res2.Streak != 2 {
		t.Fatalf("day2: want streak=2, got %d", res2.Streak)
	}
	if res2.PointsGranted != 20 {
		t.Fatalf("day2: want points=20, got %d", res2.PointsGranted)
	}
	if res2.Balance != 30 {
		t.Fatalf("day2: want balance=30, got %d", res2.Balance)
	}
}

// ITCheckinStreakBeyondWeek 连签第 8 天：streak 继续累加为 8，奖励回到第 1 天档位
func TestITCheckinStreakBeyondWeek(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db, "it_checkin8@example.com")
	svc := NewService(NewRepository(db), points.NewService(points.NewRepository(db)))

	res, err := svc.Checkin(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	// 伪造昨日已连签 7 天
	if _, err := db.Exec(`UPDATE plus_checkin_logs SET checked_date = CURRENT_DATE - 1, streak = 7 WHERE user_id = $1`, uid); err != nil {
		t.Fatal(err)
	}
	_ = res
	st, err := svc.Status(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Streak != 7 || st.TodayPoints != 10 {
		t.Fatalf("status before day 8: streak=%d today_points=%d", st.Streak, st.TodayPoints)
	}
	res8, err := svc.Checkin(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if res8.Streak != 8 || res8.PointsGranted != 10 {
		t.Fatalf("day 8: streak=%d pts=%d", res8.Streak, res8.PointsGranted)
	}
}
