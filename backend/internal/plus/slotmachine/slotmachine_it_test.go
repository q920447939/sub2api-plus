//go:build integration

package slotmachine_test

// HTTP 端到端集成测试：gin Engine + 真实 PostgreSQL（需已执行全部 migration 的独立库）。
// 运行：PLUS_SLOT_IT_DSN='postgres://...' go test -tags integration ./internal/plus/slotmachine -v
// 测试会清空 plus_* / redeem_codes / payment_orders / users 表，切勿指向业务库。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/checkin"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	"github.com/Wei-Shaw/sub2api/internal/plus/slotmachine"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

type fixedRNG float64

func (f fixedRNG) Float64() float64 { return float64(f) }

type env struct {
	t       *testing.T
	db      *sql.DB
	svc     *slotmachine.Service
	points  *points.Service
	checkin *checkin.Service
	r       *gin.Engine
	uid     int64
}

func setup(t *testing.T, rng float64, mutate func(*slotmachine.Config)) *env {
	t.Helper()
	dsn := os.Getenv("PLUS_SLOT_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_SLOT_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tbl := range []string{
		"plus_slot_feed", "plus_slot_pot_ledger", "plus_slot_spins", "plus_slot_config",
		"plus_checkin_ip_logs", "plus_checkin_logs", "plus_checkin_config", "plus_points_redeem_grants",
		"plus_points_ledger", "plus_points_accounts", "plus_points_config",
		"redeem_codes", "payment_orders", "users",
	} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	if _, err := db.Exec(`UPDATE plus_slot_pots SET balance = 0`); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, db: db}
	if err := db.QueryRow(
		`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
		 VALUES ('player@example.com', 'x', 0, 'user', NOW(), NOW()) RETURNING id`).Scan(&e.uid); err != nil {
		t.Fatal(err)
	}
	e.points = points.NewService(points.NewRepository(db))
	e.checkin = checkin.NewService(checkin.NewRepository(db), e.points)
	e.svc = slotmachine.NewService(slotmachine.NewRepository(db), e.points, nil, e.checkin)
	e.svc.SetRNG(fixedRNG(rng))

	cfg := slotmachine.DefaultConfig()
	cfg.Enabled = true
	cfg.Rules = nil
	if mutate != nil {
		mutate(&cfg)
	}
	if _, err := e.svc.AdminSaveConfig(context.Background(), cfg, 0); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	e.r = gin.New()
	g := e.r.Group("/plus")
	g.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: e.uid})
		c.Next()
	})
	slotmachine.NewHandler(e.svc).RegisterUserRoutes(g)
	return e
}

func (e *env) grant(n int64) {
	if _, err := points.Credit(context.Background(), e.db, e.uid, n, points.ReasonCheckin, 0, "seed"); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) do(method, path, key string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func data(body map[string]any) map[string]any {
	d, _ := body["data"].(map[string]any)
	return d
}

func (e *env) scalar(q string, args ...any) float64 {
	var v float64
	if err := e.db.QueryRow(q, args...).Scan(&v); err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (e *env) fillPots(small, jackpot int64) {
	if _, err := e.db.Exec(`UPDATE plus_slot_pots SET balance = CASE pot WHEN 'small' THEN $1::bigint ELSE $2::bigint END`, small, jackpot); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) pot(name string) float64 {
	return e.scalar(`SELECT balance FROM plus_slot_pots WHERE pot = $1`, name)
}

// 默认奖池（万分比）下标 0..7 = 25,2500,1200,2255,120,3000,400,500。
// 大奖罐为空时 $20（下标 0）被排除、其 25 转给谢谢惠顾，累计区间为：
// 1:[0,2500) 2:[2500,3700) 3:[3700,5980) 4:[5980,6100) 5:[6100,9100) 6:[9100,9500) 7:[9500,10000)

func TestSpinPointsPrizeAndPots(t *testing.T) {
	e := setup(t, 0.70, nil) // 7000 → 下标 5（50 积分）
	e.grant(1000)
	e.fillPots(10000, 0)
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-points-0001")
	if code != 200 {
		t.Fatalf("spin %d %v", code, body)
	}
	d := data(body)
	if d["tier_index"].(float64) != 5 || d["prize_type"] != "points" || d["balance"].(float64) != 950 {
		t.Fatalf("unexpected result %v", d)
	}
	// 小奖罐：10000 + 60 注入 - 50 支出；大奖罐：0 + 5 注入
	if s, j := e.pot("small"), e.pot("jackpot"); s != 10010 || j != 5 {
		t.Fatalf("pots small=%v jackpot=%v", s, j)
	}
	if excl := e.scalar(`SELECT cardinality(excluded_tiers) FROM plus_slot_spins`); excl != 1 {
		t.Fatalf("jackpot tier should be excluded when jackpot pot is empty, got %v", excl)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM plus_points_ledger WHERE user_id=$1 AND ref_id IS NOT NULL AND reason LIKE 'slot_%'`, e.uid); n != 2 {
		t.Fatalf("ledger rows linked to spin = %v", n)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM plus_slot_feed WHERE NOT is_virtual`); n != 1 {
		t.Fatalf("real win should be written to feed, got %v", n)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM plus_slot_pot_ledger`); n != 3 {
		t.Fatalf("pot ledger rows (2 inflow + 1 payout) = %v", n)
	}
}

func TestSpinBalancePrizeAndIdempotency(t *testing.T) {
	e := setup(t, 0.93, nil) // 9300 → 下标 6（$1 余额，100 积分估值）
	e.grant(300)
	e.fillPots(1000, 0)
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-balance-001")
	if code != 200 {
		t.Fatalf("spin %d %v", code, body)
	}
	d := data(body)
	codeStr, _ := d["redeem_code"].(string)
	if d["prize_type"] != "balance" || !strings.HasPrefix(codeStr, "SLOT-") {
		t.Fatalf("unexpected %v", d)
	}
	// 凭证码必须是随机码：不含摇奖 ID，且与 redeem_codes、摇奖记录里存的一致
	spinID := int64(d["spin_id"].(float64))
	if len(codeStr) != len("SLOT-")+16 || codeStr == fmt.Sprintf("SLOT-%d", spinID) {
		t.Fatalf("redeem code should be random, got %q (spin %d)", codeStr, spinID)
	}
	var stored, rcStatus string
	if err := e.db.QueryRow(`SELECT s.redeem_code, rc.status FROM plus_slot_spins s
		JOIN redeem_codes rc ON rc.code = s.redeem_code WHERE s.id = $1`, spinID).Scan(&stored, &rcStatus); err != nil {
		t.Fatalf("stored code: %v", err)
	}
	if stored != codeStr || rcStatus != "used" {
		t.Fatalf("stored %q status %q, want %q used", stored, rcStatus, codeStr)
	}
	_, hb := e.do(http.MethodGet, "/plus/slot/history?page=1&page_size=10", "")
	if items := data(hb)["items"].([]any); items[0].(map[string]any)["redeem_code"] != codeStr {
		t.Fatalf("history code mismatch: %v", items[0])
	}
	if bal := e.scalar(`SELECT balance FROM users WHERE id=$1`, e.uid); bal != 1 {
		t.Fatalf("user balance %v", bal)
	}
	if s := e.pot("small"); s != 1000+60-100 {
		t.Fatalf("balance prize should charge 100 points to small pot, got %v", s)
	}
	code, body = e.do(http.MethodPost, "/plus/slot/spin", "key-balance-001")
	if code != 200 || data(body)["replayed"] != true || data(body)["redeem_code"] != codeStr {
		t.Fatalf("replay %d %v", code, body)
	}
	if pts := e.scalar(`SELECT balance FROM plus_points_accounts WHERE user_id=$1`, e.uid); pts != 200 {
		t.Fatalf("points after replay %v", pts)
	}
	if bal := e.scalar(`SELECT balance FROM users WHERE id=$1`, e.uid); bal != 1 {
		t.Fatalf("balance credited twice: %v", bal)
	}
}

// 罐空时只剩价值 ≤ 本次注入（60）的奖档可中，其余概率全部给谢谢惠顾
func TestEmptyPotsExcludeTiers(t *testing.T) {
	e := setup(t, 0.99, nil)
	e.grant(1000)
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-emptypot-01")
	if code != 200 {
		t.Fatalf("spin %d %v", code, body)
	}
	// 可中：20 积分 [0,2500)、谢谢惠顾 [2500,7000)、50 积分 [7000,10000) → 9900 落在 50 积分
	if idx := data(body)["tier_index"].(float64); idx != 5 {
		t.Fatalf("want tier 5, got %v", idx)
	}
	var excl string
	if err := e.db.QueryRow(`SELECT excluded_tiers::text FROM plus_slot_spins`).Scan(&excl); err != nil {
		t.Fatal(err)
	}
	if excl != "{0,2,4,6,7}" {
		t.Fatalf("excluded tiers %s", excl)
	}
	if s := e.pot("small"); s != 10 {
		t.Fatalf("small pot 0+60-50 = 10, got %v", s)
	}
}

func TestSpinInsufficientPoints(t *testing.T) {
	e := setup(t, 0.1, nil)
	e.grant(50)
	_, ov := e.do(http.MethodGet, "/plus/slot/overview", "")
	if d := data(ov); d["can_spin"] != false || d["blocker"] != "insufficient_points" {
		t.Fatalf("overview %v", d)
	}
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-insufficient")
	if code != http.StatusConflict || body["reason"] != "insufficient_points" {
		t.Fatalf("spin %d %v", code, body)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM plus_slot_pot_ledger`); n != 0 {
		t.Fatalf("failed spin must not touch pots: %v", n)
	}
}

func TestRulesDailyLimitAndCooldown(t *testing.T) {
	e := setup(t, 0.5, func(c *slotmachine.Config) {
		c.Rules = []slotmachine.RuleConfig{
			{Type: slotmachine.RuleDailyLimit, Enabled: true, Params: map[string]any{"max": 2}},
			{Type: slotmachine.RuleCooldown, Enabled: true, Params: map[string]any{"seconds": 30}},
		}
	})
	e.grant(1000)
	if code, _ := e.do(http.MethodPost, "/plus/slot/spin", "key-rule-00001"); code != 200 {
		t.Fatal("first spin should pass")
	}
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-rule-00002")
	if code != http.StatusTooManyRequests || body["reason"] != "cooldown" {
		t.Fatalf("want cooldown, got %d %v", code, body)
	}
	_, _ = e.db.Exec(`UPDATE plus_slot_spins SET created_at = created_at - INTERVAL '1 minute'`)
	if code, _ := e.do(http.MethodPost, "/plus/slot/spin", "key-rule-00003"); code != 200 {
		t.Fatal("second spin after cooldown should pass")
	}
	_, _ = e.db.Exec(`UPDATE plus_slot_spins SET created_at = created_at - INTERVAL '1 minute'`)
	code, body = e.do(http.MethodPost, "/plus/slot/spin", "key-rule-00004")
	if code != http.StatusForbidden || body["reason"] != "daily_limit" {
		t.Fatalf("want daily_limit, got %d %v", code, body)
	}
}

// 充值门槛：系统奖励码与在线订单生成的卡密不重复计入，在线订单按订单金额计入
func TestRechargeRuleSources(t *testing.T) {
	e := setup(t, 0.5, func(c *slotmachine.Config) {
		c.Rules = []slotmachine.RuleConfig{{Type: slotmachine.RuleRecharge, Enabled: true,
			Params: map[string]any{"window_days": 30, "min_amount": 10}}}
	})
	e.grant(1000)
	ins := func(code string, v float64, usedAt string) {
		if _, err := e.db.Exec(`INSERT INTO redeem_codes (code, type, value, status, used_by, used_at)
			VALUES ($1, 'balance', $2, 'used', $3, `+usedAt+`)`, code, v, e.uid); err != nil {
			t.Fatal(err)
		}
	}
	ins("LOTTERY-1", 50, "NOW()")
	ins("SLOT-1", 50, "NOW()")
	ins("OLD-CARD", 50, "NOW() - INTERVAL '40 days'")
	ins("PAY-9-1", 6, "NOW()")
	if _, err := e.db.Exec(`INSERT INTO payment_orders (user_id, user_email, user_name, amount, pay_amount, recharge_code,
		payment_type, order_type, status, expires_at, completed_at, created_at, updated_at)
		VALUES ($1, 'player@example.com', 'p', 6, 6, 'PAY-9-1', 'alipay', 'balance', 'COMPLETED', NOW(), NOW(), NOW(), NOW())`, e.uid); err != nil {
		t.Fatal(err)
	}
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-recharge-01")
	if code != http.StatusForbidden || body["reason"] != "recharge_required" {
		t.Fatalf("6 of 10 recharged should be blocked, got %d %v", code, body)
	}
	ins("CARD-REAL", 4, "NOW()")
	if code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-recharge-02"); code != 200 {
		t.Fatalf("10 of 10 recharged should pass, got %d %v", code, body)
	}
}

// 连签加成：500 积分档（下标 4）+30 个百分点，从谢谢惠顾里扣
func TestStreakBoostFromCheckinConfig(t *testing.T) {
	e := setup(t, 0.40, nil)
	ctx := context.Background()
	ccfg := checkin.DefaultConfig()
	ccfg.StreakBoost = checkin.StreakBoost{Enabled: true, MinStreak: 3, Tiers: []checkin.TierBoost{{Tier: 4, AddBP: 3000}}}
	if _, err := e.checkin.SaveConfig(ctx, ccfg, 0); err != nil {
		t.Fatal(err)
	}
	e.grant(1000)
	e.fillPots(100000, 0)

	// 无连签：4000 落在谢谢惠顾 [3700,5980)
	code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-boost-00001")
	if code != 200 || data(body)["tier_index"].(float64) != 3 || data(body)["boosted"] != false {
		t.Fatalf("no streak: %d %v", code, body)
	}

	ledger := e.scalar(`SELECT id FROM plus_points_ledger WHERE user_id=$1 ORDER BY id LIMIT 1`, e.uid)
	today := time.Now().In(time.FixedZone("CST", 8*3600))
	if _, err := e.db.Exec(`INSERT INTO plus_checkin_logs (user_id, checked_date, streak, points_granted, ledger_id)
		VALUES ($1, $2, 3, 30, $3)`, e.uid, today.AddDate(0, 0, -1).Format("2006-01-02"), int64(ledger)); err != nil {
		t.Fatal(err)
	}
	_, ov := e.do(http.MethodGet, "/plus/slot/overview", "")
	d := data(ov)
	if boosts, _ := d["boosts"].([]any); len(boosts) != 1 || d["streak"].(float64) != 3 {
		t.Fatalf("overview boosts %v", d)
	}
	// 用户端只知道哪格有加成，不下发概率数字
	if tier4 := d["tiers"].([]any)[4].(map[string]any); tier4["boosted"] != true {
		t.Fatalf("tier 4 should be boosted: %v", tier4)
	}
	// 加成后：谢谢惠顾缩到 [3700,3700+剩余)，4000 落入 500 积分档
	code, body = e.do(http.MethodPost, "/plus/slot/spin", "key-boost-00002")
	if code != 200 || data(body)["tier_index"].(float64) != 4 || data(body)["boosted"] != true {
		t.Fatalf("with streak: %d %v", code, body)
	}
}

func TestFeedTodayAndVirtual(t *testing.T) {
	e := setup(t, 0, func(c *slotmachine.Config) {
		c.Atmosphere.VirtualDailyTarget = 5000
		c.Atmosphere.MarqueeSize = 5
	})
	now := time.Now().Add(-2 * time.Minute)
	if err := e.svc.GenerateVirtual(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.GenerateVirtual(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM plus_slot_feed WHERE is_virtual`); n != 1 {
		t.Fatalf("virtual rows %v", n)
	}
	// rng=0 → 虚拟分布第一个非零档：下标 0（$20）
	if idx := e.scalar(`SELECT tier_index FROM plus_slot_feed`); idx != 0 {
		t.Fatalf("virtual tier %v", idx)
	}
	// 昨天的记录不进今日跑马灯
	if _, err := e.db.Exec(`INSERT INTO plus_slot_feed (display_name, tier_index, label, emoji, created_at)
		VALUES ('a***@q***.com', 1, '20 积分', '🪙', NOW() - INTERVAL '2 days')`); err != nil {
		t.Fatal(err)
	}
	_, body := e.do(http.MethodGet, "/plus/slot/feed", "")
	items, _ := body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("feed %v", body)
	}
	if name := items[0].(map[string]any)["display_name"].(string); !strings.Contains(name, "***@") {
		t.Fatalf("display name not masked: %s", name)
	}
	if _, ok := items[0].(map[string]any)["is_virtual"]; ok {
		t.Fatal("feed must not expose is_virtual")
	}
}

func TestAdminPotAdjustAndState(t *testing.T) {
	e := setup(t, 0, nil)
	ctx := context.Background()
	if _, err := e.svc.AdminAdjustPot(ctx, "small", 5000, e.uid, ""); err == nil {
		t.Fatal("empty note should be rejected")
	}
	if bal, err := e.svc.AdminAdjustPot(ctx, "jackpot", 3000, e.uid, "上线初始注水"); err != nil || bal != 3000 {
		t.Fatalf("add %v %v", bal, err)
	}
	if _, err := e.svc.AdminAdjustPot(ctx, "jackpot", -5000, e.uid, "抽水"); err != slotmachine.ErrPotInsufficient {
		t.Fatalf("over-withdraw: %v", err)
	}
	led, err := e.svc.AdminPotLedger(ctx, true, 10)
	if err != nil || len(led) != 1 || led[0].OperatorID != e.uid {
		t.Fatalf("manual ledger %+v %v", led, err)
	}
	st, err := e.svc.AdminState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Base.SmallInflow != 60 || st.Base.JackpotInflow != 5 || st.Base.SmallExpected <= 0 {
		t.Fatalf("pot preview %+v", st.Base)
	}
	if st.Boosted.SmallExpected <= st.Base.SmallExpected {
		t.Fatalf("default streak boost should raise small pot expectation: %+v vs %+v", st.Boosted, st.Base)
	}
	cfg := st.Config
	cfg.Tiers[1].ProbBP++
	if _, err := e.svc.AdminSaveConfig(ctx, cfg, st.Version); err == nil {
		t.Fatal("probabilities not summing to 100% should be rejected")
	}
}

// 兑换送积分：钩子即时发放、重复调用不重复发、代发只登记、补偿任务补漏
func TestRedeemBonus(t *testing.T) {
	e := setup(t, 0, nil)
	ctx := context.Background()
	pcfg := points.DefaultConfig()
	pcfg.RedeemBonus.Enabled = true
	if _, err := e.points.SaveConfig(ctx, pcfg, 0); err != nil {
		t.Fatal(err)
	}
	code := func(c string, v float64, usedAt string) int64 {
		var id int64
		if err := e.db.QueryRow(`INSERT INTO redeem_codes (code, type, value, status, used_by, used_at)
			VALUES ($1, 'balance', $2, 'used', $3, `+usedAt+`) RETURNING id`, c, v, e.uid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ev := func(id int64, c string, v float64, src string) points.RedeemEvent {
		return points.RedeemEvent{UserID: e.uid, CodeID: id, Code: c, Type: "balance", Value: v, Source: src}
	}
	c1 := code("CARD-50", 50, "NOW()")
	for i := 0; i < 2; i++ {
		if err := e.points.HandleRedeem(ctx, ev(c1, "CARD-50", 50, points.RedeemSourceUser)); err != nil {
			t.Fatal(err)
		}
	}
	if b := e.scalar(`SELECT balance FROM plus_points_accounts WHERE user_id=$1`, e.uid); b != 600 {
		t.Fatalf("$50 should give 600 once, got %v", b)
	}
	c2 := code("ADMIN-100", 100, "NOW()")
	if err := e.points.HandleRedeem(ctx, ev(c2, "ADMIN-100", 100, points.RedeemSourceAdmin)); err != nil {
		t.Fatal(err)
	}
	if err := e.points.HandleRedeem(ctx, ev(0, "SLOT-9", 100, points.RedeemSourceUser)); err != nil {
		t.Fatal(err)
	}
	if b := e.scalar(`SELECT balance FROM plus_points_accounts WHERE user_id=$1`, e.uid); b != 600 {
		t.Fatalf("admin and system codes must not give points, got %v", b)
	}
	g, err := e.points.LatestRedeemGrant(ctx, e.uid, 5*time.Minute)
	if err != nil || g == nil || g.Points != 600 {
		t.Fatalf("latest grant %+v %v", g, err)
	}

	// 钩子漏发的卡密（3 分钟前兑换）由补偿任务补上；开关打开之前的卡密不补
	code("MISSED-10", 10, "NOW() - INTERVAL '3 minutes'")
	if _, err := e.db.Exec(`UPDATE plus_points_config SET data = jsonb_set(data, '{redeem_bonus,enabled_at}', to_jsonb((NOW() - INTERVAL '10 minutes')::timestamptz))`); err != nil {
		t.Fatal(err)
	}
	code("BEFORE-ON", 100, "NOW() - INTERVAL '1 hour'")
	time.Sleep(11 * time.Second) // 等积分配置缓存过期
	n, err := e.points.BackfillRedeemBonus(ctx)
	if err != nil || n != 1 {
		t.Fatalf("backfill n=%d err=%v", n, err)
	}
	if b := e.scalar(`SELECT balance FROM plus_points_accounts WHERE user_id=$1`, e.uid); b != 700 {
		t.Fatalf("after backfill want 700, got %v", b)
	}
}

// 用户端接口不得泄露概率与加成数值
func TestOverviewHidesOdds(t *testing.T) {
	e := setup(t, 0.1, nil)
	e.grant(50)
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/plus/slot/overview", nil))
	raw := w.Body.String()
	for _, banned := range []string{"prob_bp", "boost_bp", "add_bp", `"tiers":{`} {
		if strings.Contains(raw, banned) {
			t.Fatalf("overview leaks %s: %s", banned, raw)
		}
	}
	d := data(mustJSON(t, raw))
	if d["reason"] != "积分不足，还差 50 积分" {
		t.Fatalf("reason %v", d["reason"])
	}
	// 未注册签到来源、兑换送积分默认关闭 → 没有可用的获取方式
	if ways, _ := d["earn_ways"].([]any); len(ways) != 0 {
		t.Fatalf("earn_ways %v", d["earn_ways"])
	}
}

// 积分获取方式按已注册来源动态返回：签到开启且今天未签 → 去签到；兑换送积分开启 → 兑换卡密
func TestOverviewEarnWays(t *testing.T) {
	e := setup(t, 0.1, nil)
	ctx := context.Background()
	e.points.RegisterEarnWay(e.checkin.EarnWay)
	ccfg := checkin.DefaultConfig()
	ccfg.Enabled = true
	if _, err := e.checkin.SaveConfig(ctx, ccfg, 0); err != nil {
		t.Fatal(err)
	}
	pcfg := points.DefaultConfig()
	pcfg.RedeemBonus.Enabled = true
	if _, err := e.points.SaveConfig(ctx, pcfg, 0); err != nil {
		t.Fatal(err)
	}
	_, ov := e.do(http.MethodGet, "/plus/slot/overview", "")
	ways, _ := data(ov)["earn_ways"].([]any)
	if len(ways) != 2 || ways[0].(map[string]any)["key"] != "checkin" || ways[1].(map[string]any)["key"] != "redeem_bonus" {
		t.Fatalf("earn_ways %v", ways)
	}
	if _, err := e.checkin.Checkin(ctx, e.uid, ""); err != nil {
		t.Fatal(err)
	}
	_, ov = e.do(http.MethodGet, "/plus/slot/overview", "")
	ways, _ = data(ov)["earn_ways"].([]any)
	if len(ways) != 1 || ways[0].(map[string]any)["key"] != "redeem_bonus" {
		t.Fatalf("after checkin earn_ways %v", ways)
	}
}

// 冷却文案不再是"手速太快"，并且返回 retry_after 供前端倒数
func TestCooldownMessage(t *testing.T) {
	e := setup(t, 0.1, func(c *slotmachine.Config) {
		c.Rules = []slotmachine.RuleConfig{{Type: slotmachine.RuleCooldown, Enabled: true, Params: map[string]any{"seconds": 30}}}
	})
	e.grant(1000)
	if code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-cool-00001"); code != 200 {
		t.Fatalf("first spin %d %v", code, body)
	}
	_, ov := e.do(http.MethodGet, "/plus/slot/overview", "")
	d := data(ov)
	if d["blocker"] != "cooldown" || !strings.HasPrefix(d["reason"].(string), "休息一下") || d["retry_after"].(float64) <= 0 {
		t.Fatalf("overview %v", d)
	}
}

// 抽奖记录分页 + 只看中奖
func TestHistoryPagination(t *testing.T) {
	e := setup(t, 0.1, nil) // 0.1 → 谢谢惠顾以外的档位，见上方区间说明
	e.fillPots(100000, 100000)
	e.grant(1000)
	for i := 0; i < 3; i++ {
		if code, body := e.do(http.MethodPost, "/plus/slot/spin", "key-hist-0000"+string(rune('1'+i))); code != 200 {
			t.Fatalf("spin %d %v", code, body)
		}
	}
	_, body := e.do(http.MethodGet, "/plus/slot/history?page=2&page_size=2", "")
	d := data(body)
	if d["total"].(float64) != 3 || len(d["items"].([]any)) != 1 || d["page"].(float64) != 2 {
		t.Fatalf("history page 2: %v", d)
	}
	_, body = e.do(http.MethodGet, "/plus/slot/history?win=1", "")
	wins := e.scalar(`SELECT COUNT(*) FROM plus_slot_spins WHERE prize_type <> 'none'`)
	if data(body)["total"].(float64) != wins {
		t.Fatalf("win-only total %v want %v", data(body)["total"], wins)
	}
}

func mustJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}
