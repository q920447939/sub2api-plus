//go:build integration

package lottery

// 集成测试：需要一个已应用 246_plus_lottery.sql 且包含 users / payment_orders / redeem_codes 表的独立库。
// 运行：PLUS_LOTTERY_IT_DSN='postgres://...' go test -tags integration ./internal/plus/lottery -run IT -v
// 测试会清空上述表，切勿指向业务库。

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"

	_ "github.com/lib/pq"
)

type fixedRNG float64

func (f fixedRNG) Float64() float64 { return float64(f) }

func openIT(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PLUS_LOTTERY_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_LOTTERY_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tbl := range []string{"plus_lottery_feed", "plus_lottery_draws", "plus_lottery_chances",
		"plus_lottery_redeems", "plus_lottery_daily_budget", "plus_lottery_config", "redeem_codes", "payment_orders", "users"} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	return db
}

func itExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
}

func itInt(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return v
}

func itFloat(t *testing.T, db *sql.DB, q string, args ...any) float64 {
	t.Helper()
	var v float64
	if err := db.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return v
}

func setupIT(t *testing.T) (*sql.DB, *Service) {
	db := openIT(t)
	svc := NewService(NewRepository(db), nil)
	svc.rng = fixedRNG(0.99) // 总落在最高奖档，便于触发风控

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ChanceTiers = []ChanceTier{{Threshold: 10, Chances: 1}, {Threshold: 50, Chances: 3}}
	cfg.PrizePoolReal = PrizePool{Tiers: []PrizeTier{{Min: 5, Max: 10, Weight: 1}}}
	cfg.RiskControl = RiskControl{FloorAmount: 0.1, SingleOrderPct: 1}
	cfg.VirtualFeed.Enabled = false
	if _, err := svc.SaveConfig(context.Background(), cfg, 0); err != nil {
		t.Fatalf("save config: %v", err)
	}
	itExec(t, db, `INSERT INTO users (id, email, password_hash, balance) VALUES (1, 'alice@example.com', 'x', 0)`)
	// 100: enabled_at 之前兑换的卡密不应发放机会
	itRedeem(t, db, 100, "CARD-OLD", "balance", 100, "", "NOW() - interval '1 day'")
	// 101: 面值 50 → 3 张；102: 面值 9 不满 10 → 登记但 0 张
	itRedeem(t, db, 101, "CARD-50", "balance", 50, "", "NOW() + interval '1 second'")
	itRedeem(t, db, 102, "CARD-9", "balance", 9, "", "NOW() + interval '1 second'")
	return db, svc
}

// itRedeem 插入一条已被用户 1 兑换的卡密，usedAt 为 SQL 表达式
func itRedeem(t *testing.T, db *sql.DB, id int64, code, typ string, value float64, notes, usedAt string) {
	t.Helper()
	itExec(t, db, `INSERT INTO redeem_codes (id, code, type, value, status, used_by, used_at, notes)
		VALUES ($1, $2, $3, $4, 'used', 1, `+usedAt+`, $5)`, id, code, typ, value, notes)
}

func TestIT_GrantDrawIdempotency(t *testing.T) {
	db, svc := setupIT(t)
	ctx := context.Background()

	// 重复同步不重复发放；历史卡密 100 不发放
	for i := 0; i < 3; i++ {
		if err := svc.SyncChances(ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_chances WHERE user_id = 1`); n != 3 {
		t.Fatalf("chances = %d, want 3", n)
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_redeems WHERE redeem_code_id = 100`); n != 0 {
		t.Fatal("pre-activity redeem must not be granted")
	}
	if n := itInt(t, db, `SELECT chances FROM plus_lottery_redeems WHERE redeem_code_id = 102`); n != 0 {
		t.Fatalf("redeem 102 chances = %d, want 0 (value below threshold)", n)
	}
	if v := itFloat(t, db, `SELECT MIN(code_value)::float8 FROM plus_lottery_chances WHERE redeem_code_id = 101`); v != 50 {
		t.Fatalf("chance code_value = %.2f, want 50", v)
	}
	// 最近一次兑换是 102（id 更大、同一时刻），面值 9 不满门槛 → 0；把 101 改成更晚后应得 3
	if n, err := svc.LatestRedeemGrant(ctx, 1); err != nil || n != 0 {
		t.Fatalf("LatestRedeemGrant = %d,%v want 0", n, err)
	}
	itExec(t, db, `UPDATE redeem_codes SET used_at = NOW() + interval '2 seconds' WHERE id = 101`)
	if n, err := svc.LatestRedeemGrant(ctx, 1); err != nil || n != 3 {
		t.Fatalf("LatestRedeemGrant = %d,%v want 3", n, err)
	}
	if n, _ := svc.LatestRedeemGrant(ctx, 2); n != 0 {
		t.Fatal("LatestRedeemGrant must not leak other user's redeem")
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_chances
		WHERE redeem_code_id = 101 AND expires_at BETWEEN NOW() + interval '23 hours' AND NOW() + interval '25 hours'`); n != 3 {
		t.Fatalf("chances with ~24h validity = %d, want 3", n)
	}

	// 同一 key 并发 10 次：只开一次奖、只加一次余额
	var wg sync.WaitGroup
	results := make([]*DrawResult, 10)
	errs := make([]error, 10)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.Draw(ctx, 1, 0, "key-aaaaaaaa")
		}(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("draw %d: %v", i, errs[i])
		}
		if r.DrawID != results[0].DrawID || r.Amount != results[0].Amount {
			t.Fatalf("replay mismatch: %+v vs %+v", r, results[0])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("fresh draws = %d, want 1", fresh)
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_draws`); n != 1 {
		t.Fatalf("draws = %d, want 1", n)
	}
	bal := itFloat(t, db, `SELECT balance::float8 FROM users WHERE id = 1`)
	if math.Abs(bal-results[0].Amount) > 1e-9 {
		t.Fatalf("balance %.2f != award %.2f", bal, results[0].Amount)
	}
	code := RedeemCodePrefix + itoa(results[0].DrawID)
	if n := itInt(t, db, `SELECT COUNT(*) FROM redeem_codes WHERE code = $1 AND used_by = 1`, code); n != 1 {
		t.Fatalf("redeem code %s missing", code)
	}

	// 指定已用卡 + 新 key：拒绝
	_, err := svc.Draw(ctx, 1, results[0].ChanceID, "key-bbbbbbbb")
	if !errors.Is(err, ErrChanceAlreadyUsed) {
		t.Fatalf("want ErrChanceAlreadyUsed, got %v", err)
	}

	// 不同 key 并发抽完剩余 2 张，第 3 次无卡
	keys := []string{"key-c0000000", "key-c1111111", "key-c2222222"}
	errs = make([]error, len(keys))
	for i, k := range keys {
		wg.Add(1)
		go func(i int, k string) {
			defer wg.Done()
			_, errs[i] = svc.Draw(ctx, 1, 0, k)
		}(i, k)
	}
	wg.Wait()
	ok, none := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, ErrNoChancesLeft):
			none++
		default:
			t.Fatalf("unexpected: %v", e)
		}
	}
	if ok != 2 || none != 1 {
		t.Fatalf("ok=%d none=%d, want 2/1", ok, none)
	}
}

func TestIT_RiskControl(t *testing.T) {
	db, svc := setupIT(t)
	ctx := context.Background()
	cfg, ver, _ := svc.Config(ctx)
	cfg.RiskControl.UserDailyMax = 12
	if _, err := svc.SaveConfig(ctx, cfg, ver); err != nil {
		t.Fatal(err)
	}

	var total float64
	for _, k := range []string{"risk-0000000", "risk-1111111", "risk-2222222"} {
		r, err := svc.Draw(ctx, 1, 0, k)
		if err != nil {
			t.Fatal(err)
		}
		total += r.Amount
	}
	// 10 + 2(截断到上限) + 0.1(保底)
	if total > 12+0.1+1e-9 {
		t.Fatalf("user daily max breached: %.2f", total)
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_draws WHERE degraded = $1`, DegradedUserDaily); n == 0 {
		t.Fatal("expected user_daily degradation")
	}
}

// 排除规则：LOTTERY-* 奖励码、admin_balance、在线支付生成的码、开奖得到的奖金卡密都不再发卡；备注关键词过滤生效
func TestIT_RedeemExclusions(t *testing.T) {
	db, svc := setupIT(t)
	ctx := context.Background()
	soon := "NOW() + interval '1 second'"
	itRedeem(t, db, 200, RedeemCodePrefix+"999", "balance", 100, "", soon)
	itRedeem(t, db, 201, "ADMIN-ADJ", "admin_balance", 100, "", soon)
	itRedeem(t, db, 202, "PAY-CODE", "balance", 100, "", soon)
	itExec(t, db, `INSERT INTO payment_orders (id, user_id, order_type, status, amount, pay_amount, recharge_code, expires_at)
		VALUES (1, 1, 'balance', 'COMPLETED', 100, 100, 'PAY-CODE', NOW())`)
	itRedeem(t, db, 203, "SUB-CODE", "subscription", 100, "", soon)

	if err := svc.SyncChances(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_redeems WHERE redeem_code_id >= 200`); n != 0 {
		t.Fatalf("excluded codes registered: %d", n)
	}
	// 开奖产生的 LOTTERY-<id> 码不能再发卡（防止奖上加奖的循环）
	r, err := svc.Draw(ctx, 1, 0, "excl-0000000")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncChances(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_redeems lr JOIN redeem_codes rc ON rc.id = lr.redeem_code_id
		WHERE rc.code = $1`, RedeemCodePrefix+itoa(r.DrawID)); n != 0 {
		t.Fatal("prize code must not grant chances")
	}

	// 备注关键词：只计入备注包含"售卖"的卡密
	cfg, ver, _ := svc.Config(ctx)
	cfg.NotesKeyword = "售卖"
	if _, err := svc.SaveConfig(ctx, cfg, ver); err != nil {
		t.Fatal(err)
	}
	itRedeem(t, db, 300, "GIFT-1", "balance", 50, "赠送", soon)
	itRedeem(t, db, 301, "SOLD-1", "balance", 50, "淘宝售卖批次A", soon)
	if err := svc.SyncChances(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if n := itInt(t, db, `SELECT COUNT(*) FROM plus_lottery_redeems WHERE redeem_code_id = 300`); n != 0 {
		t.Fatal("code without keyword must not be granted")
	}
	if n := itInt(t, db, `SELECT chances FROM plus_lottery_redeems WHERE redeem_code_id = 301`); n != 3 {
		t.Fatalf("keyword code chances = %d, want 3", n)
	}

	st, err := svc.AdminStats(ctx, svc.Today())
	if err != nil {
		t.Fatal(err)
	}
	if st.RedeemCount != 2 || st.RedeemAmount != 100 {
		t.Fatalf("stats redeem = %d/%.2f, want 2/100", st.RedeemCount, st.RedeemAmount)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestIT_ExpiredChance(t *testing.T) {
	db, svc := setupIT(t)
	ctx := context.Background()
	if err := svc.SyncChances(ctx, 1); err != nil {
		t.Fatal(err)
	}
	itExec(t, db, `UPDATE plus_lottery_chances SET expires_at = NOW() - interval '1 minute' WHERE redeem_code_id = 101 AND seq = 1`)
	var expiredID int64
	if err := db.QueryRow(`SELECT id FROM plus_lottery_chances WHERE redeem_code_id = 101 AND seq = 1`).Scan(&expiredID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Draw(ctx, 1, expiredID, "expired-0000"); !errors.Is(err, ErrChanceExpired) {
		t.Fatalf("want ErrChanceExpired, got %v", err)
	}
	ov, err := svc.Overview(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Remaining != 2 || ov.NextExpiresAt == nil {
		t.Fatalf("overview remaining=%d next=%v, want 2 and non-nil", ov.Remaining, ov.NextExpiresAt)
	}
	// 自动选卡跳过过期卡
	r, err := svc.Draw(ctx, 1, 0, "expired-1111")
	if err != nil {
		t.Fatal(err)
	}
	if r.ChanceID == expiredID {
		t.Fatal("auto pick must skip expired chance")
	}
	st, err := svc.AdminStats(ctx, svc.Today())
	if err != nil {
		t.Fatal(err)
	}
	if st.ExpiredChances != 1 || st.UnusedChances != 1 {
		t.Fatalf("stats expired=%d unused=%d, want 1/1", st.ExpiredChances, st.UnusedChances)
	}
}
