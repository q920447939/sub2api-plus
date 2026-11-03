//go:build integration

package points

// 集成测试：需要一个已应用 248_plus_points.sql 的独立库。
// 运行：PLUS_POINTS_IT_DSN='postgres://...' go test -tags integration ./internal/plus/points -run IT -v
// 测试会清空 plus_points_* 与 users 表，切勿指向业务库。

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func openIT(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PLUS_POINTS_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_POINTS_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tbl := range []string{"plus_slot_feed", "plus_slot_pot_ledger", "plus_slot_spins", "plus_checkin_ip_logs", "plus_checkin_logs", "plus_points_redeem_grants", "redeem_codes", "plus_points_ledger", "plus_points_accounts", "plus_points_config", "users"} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	return db
}

func itExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec: %v\n%s", err, q)
	}
}

func itInt(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("query int: %v\n%s", err, q)
	}
	return v
}

func seedUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
		 VALUES ('it_points@example.com', 'x', 0, 'user', NOW(), NOW())
		 RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// ITCreditDebit 基本增减积分
func TestITCreditDebit(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db)

	entry, err := Credit(ctx, db, uid, 100, ReasonCheckin, 0, "签到第1天")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Balance != 100 {
		t.Fatalf("want balance=100, got %d", entry.Balance)
	}

	entry2, err := Credit(ctx, db, uid, 50, ReasonCheckin, 0, "签到第2天")
	if err != nil {
		t.Fatal(err)
	}
	if entry2.Balance != 150 {
		t.Fatalf("want balance=150, got %d", entry2.Balance)
	}

	de, err := Debit(ctx, db, uid, 30, ReasonSlotConsume, 0, "消耗30分")
	if err != nil {
		t.Fatal(err)
	}
	if de.Balance != 120 {
		t.Fatalf("want balance=120, got %d", de.Balance)
	}

	// 验证数据库里的余额快照
	got := itInt(t, db, `SELECT balance FROM plus_points_accounts WHERE user_id = $1`, uid)
	if got != 120 {
		t.Fatalf("db balance: want 120, got %d", got)
	}
	ledgerCount := itInt(t, db, `SELECT COUNT(*) FROM plus_points_ledger WHERE user_id = $1`, uid)
	if ledgerCount != 3 {
		t.Fatalf("ledger count: want 3, got %d", ledgerCount)
	}
}

// ITDebitInsufficient 余额不足时返回错误
func TestITDebitInsufficient(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db)

	if _, err := Credit(ctx, db, uid, 10, ReasonCheckin, 0, ""); err != nil {
		t.Fatal(err)
	}
	_, err := Debit(ctx, db, uid, 100, ReasonSlotConsume, 0, "超额扣减")
	if err != ErrInsufficientPoints {
		t.Fatalf("want ErrInsufficientPoints, got %v", err)
	}
	// 余额不变
	got := itInt(t, db, `SELECT balance FROM plus_points_accounts WHERE user_id = $1`, uid)
	if got != 10 {
		t.Fatalf("db balance unchanged: want 10, got %d", got)
	}
}

// ITExpire 过期清零
func TestITExpire(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db)

	if _, err := Credit(ctx, db, uid, 500, ReasonCheckin, 0, ""); err != nil {
		t.Fatal(err)
	}
	// 手动把 last_active_at 拨到 400 天前
	itExec(t, db,
		`UPDATE plus_points_accounts SET last_active_at = NOW() - INTERVAL '400 days' WHERE user_id = $1`, uid)

	repo := NewRepository(db)
	threshold := now().AddDate(0, 0, -365)
	res, more, err := repo.ExpireInactive(ctx, threshold, 100)
	if err != nil {
		t.Fatal(err)
	}
	if more {
		t.Fatal("unexpected more=true")
	}
	if res.ExpiredAccounts != 1 {
		t.Fatalf("want 1 expired account, got %d", res.ExpiredAccounts)
	}
	if res.ExpiredPoints != 500 {
		t.Fatalf("want 500 expired points, got %d", res.ExpiredPoints)
	}
	got := itInt(t, db, `SELECT balance FROM plus_points_accounts WHERE user_id = $1`, uid)
	if got != 0 {
		t.Fatalf("balance after expire: want 0, got %d", got)
	}
	ledgerRows := itInt(t, db,
		`SELECT COUNT(*) FROM plus_points_ledger WHERE user_id = $1 AND reason = $2`, uid, ReasonExpire)
	if ledgerRows != 1 {
		t.Fatalf("expire ledger rows: want 1, got %d", ledgerRows)
	}
}

// ITAdminAdjust 管理员加减积分：原因只在管理端可见；扣减不能超过余额；统计计入 admin_adjust
func TestITAdminAdjust(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db)
	var opID int64
	if err := db.QueryRow(`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
		VALUES ('it_admin@example.com', 'x', 0, 'admin', NOW(), NOW()) RETURNING id`).Scan(&opID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepository(db))

	if _, err := svc.AdminAdjust(ctx, uid, 100, opID, ""); err == nil {
		t.Fatal("empty remark must be rejected")
	}
	bal, err := svc.AdminAdjust(ctx, uid, 300, opID, "活动补偿 #42")
	if err != nil || bal != 300 {
		t.Fatalf("credit: %v %d", err, bal)
	}
	if _, err := svc.AdminAdjust(ctx, uid, -500, opID, "误发回收"); err != ErrInsufficientPoints {
		t.Fatalf("over-debit want ErrInsufficientPoints, got %v", err)
	}
	if bal, err = svc.AdminAdjust(ctx, uid, -100, opID, "误发回收"); err != nil || bal != 200 {
		t.Fatalf("debit: %v %d", err, bal)
	}

	// 用户端流水只看到统一说明
	items, n, err := svc.Ledger(ctx, uid, 1, 20, "")
	if err != nil || n != 2 || items[0].Note != AdminAdjustNote || items[0].Reason != ReasonAdminAdjust {
		t.Fatalf("user ledger %v %d %+v", err, n, items)
	}
	// 管理端看到原因与操作人
	adm, total, err := svc.AdminLedger(ctx, LedgerQuery{UserID: uid})
	if err != nil || total != 2 || adm[1].Remark != "活动补偿 #42" || adm[1].OperatorEmail != "it_admin@example.com" {
		t.Fatalf("admin ledger %v %d %+v", err, total, adm)
	}

	accs, total, err := svc.AdminListAccounts(ctx, AccountQuery{Keyword: "it_points"})
	if err != nil || total != 1 || accs[0].Balance != 200 || accs[0].TotalEarned != 300 || accs[0].TotalSpent != 100 {
		t.Fatalf("accounts %v %d %+v", err, total, accs)
	}

	st, err := svc.AdminStats(ctx, 7)
	if err != nil || len(st.Trend) != 7 || st.Outstanding != 200 {
		t.Fatalf("stats %v %+v", err, st)
	}
	today := st.Trend[6]
	if today.Issued != 300 || today.Consumed != 100 {
		t.Fatalf("today trend %+v", today)
	}
}

// ITTopHolders 积分排行：按余额降序取前 10，排除 0 余额与已删除用户
func TestITTopHolders(t *testing.T) {
	db := openIT(t)
	ctx := context.Background()
	svc := NewService(NewRepository(db))
	newUser := func(email string, pts int64, deleted bool) {
		var id int64
		if err := db.QueryRow(`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
			VALUES ($1, 'x', 0, 'user', NOW(), NOW()) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if pts > 0 {
			if _, err := Credit(ctx, db, id, pts, ReasonCheckin, 0, "seed"); err != nil {
				t.Fatal(err)
			}
		} else {
			itExec(t, db, `INSERT INTO plus_points_accounts (user_id, balance) VALUES ($1, 0)`, id)
		}
		if deleted {
			itExec(t, db, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, id)
		}
	}
	for i := 1; i <= 12; i++ {
		newUser(fmt.Sprintf("top%02d@example.com", i), int64(i*100), false)
	}
	newUser("zero@example.com", 0, false)
	newUser("gone@example.com", 99999, true)

	st, err := svc.AdminStats(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.TopHolders) != TopHolderLimit {
		t.Fatalf("want %d holders, got %d", TopHolderLimit, len(st.TopHolders))
	}
	first, last := st.TopHolders[0], st.TopHolders[TopHolderLimit-1]
	if first.Rank != 1 || first.Email != "top12@example.com" || first.Balance != 1200 {
		t.Fatalf("first %+v", first)
	}
	if last.Rank != 10 || last.Email != "top03@example.com" || last.Balance != 300 {
		t.Fatalf("last %+v", last)
	}
	for _, h := range st.TopHolders {
		if h.Email == "zero@example.com" || h.Email == "gone@example.com" {
			t.Fatalf("should be excluded: %+v", h)
		}
	}
}
