//go:build unit

package outreach

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func testPurposes() *PurposeRegistry {
	r := NewPurposeRegistry()
	r.Register(NewVerifyEmailPurpose(verifyRewardVars{}))
	r.Register(NewRecallPurpose())
	r.Register(NewNewsPurpose())
	r.Register(NewPromoPurpose())
	return r
}

type fakeGranter struct{}

func (fakeGranter) GrantPoints(context.Context, DBTX, int64, int64, int64) error { return nil }

func testBenefits() *BenefitRegistry {
	b := NewBenefitRegistry()
	b.Register(NewPointsBenefit(fakeGranter{}))
	return b
}

// ─── 人群：参数绑定 ──────────────────────────────────────────────────────────

var placeholderNumRe = regexp.MustCompile(`\$(\d+)`)

func assertBound(t *testing.T, name, sql string, args []any) {
	t.Helper()
	seen := map[int]bool{}
	for _, m := range placeholderNumRe.FindAllStringSubmatch(sql, -1) {
		n, _ := strconv.Atoi(m[1])
		seen[n] = true
	}
	if len(seen) != len(args) {
		t.Fatalf("%s: %d distinct placeholders, %d args\n%s", name, len(seen), len(args), sql)
	}
	for i := 1; i <= len(args); i++ {
		if !seen[i] {
			t.Fatalf("%s: placeholder $%d unused\n%s", name, i, sql)
		}
	}
}

func TestAudienceFragmentsAlwaysBindConsistently(t *testing.T) {
	cfgs := []DeliveryConfig{DefaultDeliveryConfig(), {MarketingGapDays: 0, PurposeGapDays: map[string]int{"recall": 0, "verify_email": 0}}}
	segs := []Segment{{}, {Recharged: "yes", InactiveDays: 30, MinBalance: 1, RegisteredDays: 7}}
	for _, p := range testPurposes().All() {
		for ci, cfg := range cfgs {
			for _, cid := range []int64{0, 9} {
				for _, sg := range segs {
					a := buildAudience(p, sg, cfg, cid)
					cases := map[string][]frag{
						"target":  {a.target()},
						"fresh":   {a.pickable(ScopeFresh)},
						"again":   {a.pickable(ScopeAgain)},
						"send":    {a.unsub, a.recent},
						"preview": {not(a.valid), a.state, a.unsub, a.target(), and(a.target(), a.recent), a.reached, a.queued, a.base},
					}
					for name, fs := range cases {
						var q query
						sql := ""
						for _, f := range fs {
							sql += q.bind(f) + " ; "
						}
						sql += q.arg(1)
						assertBound(t, p.Key()+"/"+strconv.Itoa(ci)+"/"+name, sql, q.args)
					}
				}
			}
		}
	}
}

func TestMetricColumnsBind(t *testing.T) {
	var q query
	cols, keys := NewMetricRegistry(testPurposes()).selectCols(&q)
	sql := cols + q.arg(1) + q.arg(2)
	assertBound(t, "metrics", sql, q.args)
	// verified / clicked / returned_7d / paid_14d / claimed：各目的重复声明的指标只算一次
	if strings.Join(keys, ",") != "verified,clicked,returned_7d,paid_14d,claimed" {
		t.Fatalf("metrics: %v", keys)
	}
}

func TestPickableScope(t *testing.T) {
	p, _ := testPurposes().Get(PurposeNews)
	a := buildAudience(p, Segment{}, DefaultDeliveryConfig(), 3)
	var q1, q2 query
	fresh, again := q1.bind(a.pickable(ScopeFresh)), q2.bind(a.pickable(ScopeAgain))
	re := regexp.MustCompile(`plus_outreach_ledger l WHERE l.campaign_id`)
	if !re.MatchString(fresh) || re.MatchString(again) {
		t.Fatal("fresh must exclude reached users, again must not")
	}
}

func TestFrequencyRules(t *testing.T) {
	reg := testPurposes()
	news, _ := reg.Get(PurposeNews)
	verify, _ := reg.Get(PurposeVerifyEmail)
	cfg := DefaultDeliveryConfig()
	cfg.MarketingGapDays = 0
	if recentFrag(news, cfg).sql != "FALSE" {
		t.Fatal("news: no same-purpose gap and marketing gap 0 → no frequency control")
	}
	cfg.MarketingGapDays = 7
	if !strings.Contains(recentFrag(news, cfg).sql, "category = 'marketing'") {
		t.Fatal("marketing purposes share the cross-purpose gap")
	}
	if s := recentFrag(verify, cfg).sql; strings.Contains(s, "marketing") || !strings.Contains(s, "l.purpose") {
		t.Fatal("service purposes only use their own gap")
	}
	cfg.PurposeGapDays = map[string]int{PurposeVerifyEmail: 0}
	if recentFrag(verify, cfg).sql != "FALSE" {
		t.Fatal("settings override must win over purpose default")
	}
}

func TestDescribeAudienceShowsImplicitRules(t *testing.T) {
	verify, _ := testPurposes().Get(PurposeVerifyEmail)
	rules := strings.Join(describeAudience(verify, Segment{}), "|")
	if !strings.Contains(rules, "邮箱未验证") || !strings.Contains(rules, "不限") || strings.Contains(rules, "退订") {
		t.Fatalf("unexpected rules: %s", rules)
	}
	recall, _ := testPurposes().Get(PurposeRecall)
	rules = strings.Join(describeAudience(recall, Segment{Recharged: "yes", InactiveDays: 30}), "|")
	for _, want := range []string{"邮箱已验证", "充值过", "30 天", "退订「召回」"} {
		if !strings.Contains(rules, want) {
			t.Fatalf("missing %q in %s", want, rules)
		}
	}
}

func TestBindPanicsOnMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	var q query
	q.bind(sqlf("a = $? AND b = $?", 1))
}

// ─── 目的 / 指标 ─────────────────────────────────────────────────────────────

func TestPrimaryMetricFollowsPurpose(t *testing.T) {
	reg := testPurposes()
	m := NewMetricRegistry(reg)
	cases := []struct {
		purpose      string
		cta, benefit bool
		want         string
	}{
		{PurposeVerifyEmail, true, false, MetricVerified},
		{PurposeRecall, true, false, "returned_7d"},
		{PurposeNews, true, false, MetricClicked},
		{PurposeNews, false, false, "returned_7d"}, // 没有按钮时不看点击
		{PurposePromo, true, true, "paid_14d"},
	}
	for _, c := range cases {
		p, _ := reg.Get(c.purpose)
		got := m.Applicable(p, c.cta, c.benefit)
		if got[0] != c.want {
			t.Fatalf("%s cta=%v: primary %s, want %s", c.purpose, c.cta, got[0], c.want)
		}
		hasClaim := false
		for _, k := range got {
			hasClaim = hasClaim || k == MetricClaimed
			if k == MetricClicked && !c.cta {
				t.Fatal("clicked must be hidden without CTA")
			}
		}
		if hasClaim != c.benefit {
			t.Fatalf("%s: claimed metric presence %v, want %v", c.purpose, hasClaim, c.benefit)
		}
	}
}

// ─── 行动 ────────────────────────────────────────────────────────────────────

func TestActionChain(t *testing.T) {
	reg := testPurposes()
	recall, _ := reg.Get(PurposeRecall)
	verify, _ := reg.Get(PurposeVerifyEmail)
	tgt := ActionTarget{Base: "https://a.com", Landing: "/pricing", Token: "tk", InviteToken: "inv"}

	if u, _ := actionFor(recall, false).Resolve(tgt); u != "https://a.com/pricing" {
		t.Fatalf("landing: %s", u)
	}
	if u, _ := actionFor(recall, true).Resolve(tgt); u != "https://a.com/outreach/claim?t=tk" {
		t.Fatalf("claim decorator: %s", u)
	}
	// 领完之后的去向 = 不带权益时的基础去向（ClaimInfo 用 actionFor(p, false) 计算）
	if next, _ := actionFor(recall, false).Resolve(tgt); next != "https://a.com/pricing" {
		t.Fatalf("claim next: %s", next)
	}
	if u, _ := actionFor(verify, false).Resolve(tgt); u != "https://a.com/verify-email?token=inv" {
		t.Fatalf("verify: %s", u)
	}
	if _, err := actionFor(verify, false).Resolve(ActionTarget{Base: "https://a.com"}); err == nil {
		t.Fatal("verify without invite token must fail")
	}
	if u, _ := (landingAction{}).Resolve(ActionTarget{Base: "https://a.com"}); u != "https://a.com/" {
		t.Fatalf("empty landing → home, got %s", u)
	}
}

func TestNormalizeLanding(t *testing.T) {
	bases := []string{"https://news.a.com", "https://a.com"}
	ok := []string{"", "/pricing", "https://a.com/x?y=1", "https://NEWS.a.com/p"}
	bad := []string{"//evil.com", "https://evil.com/", "javascript:alert(1)", "ftp://a.com/x", "pricing"}
	for _, s := range ok {
		if _, err := normalizeLanding(s, bases); err != nil {
			t.Fatalf("%q should pass: %v", s, err)
		}
	}
	for _, s := range bad {
		if _, err := normalizeLanding(s, bases); err == nil {
			t.Fatalf("%q should be rejected", s)
		}
	}
}

// ─── 变量 / 渲染 ─────────────────────────────────────────────────────────────

func TestUnknownVariables(t *testing.T) {
	reg := testPurposes()
	vars := NewVariables(testBenefits())
	news, _ := reg.Get(PurposeNews)
	verify, _ := reg.Get(PurposeVerifyEmail)
	pts := []BenefitSpec{{Kind: KindPointsGrant, Config: json.RawMessage(`{"points":5}`)}}

	if unk := vars.Unknown(news, nil, "{{username}} {{cta_url}} {{reward_points}}"); len(unk) != 1 || unk[0] != "reward_points" {
		t.Fatalf("reward_points needs a points benefit: %v", unk)
	}
	if unk := vars.Unknown(news, pts, "{{reward_points}} {{unsubscribe_url}}"); len(unk) != 0 {
		t.Fatalf("unexpected unknown: %v", unk)
	}
	if unk := vars.Unknown(verify, nil, "{{unsubscribe_url}} {{verify_reward_points}} {{link_url}}"); strings.Join(unk, ",") != "link_url,unsubscribe_url" {
		t.Fatalf("service mail has no unsubscribe; old link vars are gone: %v", unk)
	}
}

func TestComplianceBlockAlwaysPresent(t *testing.T) {
	reg := testPurposes()
	recall, _ := reg.Get(PurposeRecall)
	verify, _ := reg.Get(PurposeVerifyEmail)
	for _, lay := range []LayoutConfig{{}, {Style: StylePlain, Header: HeaderNone, FooterMD: ""}} {
		_, h := renderMail(MailInput{Policy: recall.Policy(), Subject: "s", BodyMD: "b", Layout: lay, SiteName: "S", UnsubURL: "https://a.com/unsubscribe?t=x"})
		if !strings.Contains(h, `href="https://a.com/unsubscribe?t=x"`) || !strings.Contains(h, "点此退订") {
			t.Fatal("marketing mail must always carry the unsubscribe link")
		}
		_, h = renderMail(MailInput{Policy: verify.Policy(), Subject: "s", BodyMD: "b", Layout: lay, SiteName: "S", UnsubURL: "https://a.com/u"})
		if strings.Contains(h, "退订") {
			t.Fatal("service mail must not offer unsubscribe")
		}
	}
}

func TestRenderPipeline(t *testing.T) {
	vars := map[string]string{"username": "<b>小明</b>", "cta_url": "https://a.com/r/t", "site_name": "站点"}
	subj, h := renderMail(MailInput{
		Policy: Policy{Category: CategoryMarketing}, Subject: "你好 {{username}}",
		BodyMD: "# 标题\n\n[[去看看]]({{cta_url}})\n\n[坏链接](javascript:alert(1))",
		Layout: LayoutConfig{ThemeColor: "#ff0000", FooterMD: "© {{site_name}}"}, Vars: vars, SiteName: "站点", UnsubURL: "https://a.com/u",
	})
	if subj != "你好 <b>小明</b>" {
		t.Fatalf("subject: %s", subj)
	}
	if strings.Contains(h, "<b>小明</b>") {
		t.Fatal("variables must be escaped in HTML")
	}
	if !strings.Contains(h, `href="https://a.com/r/t"`) || !strings.Contains(h, "background:#ff0000") {
		t.Fatal("button must link to cta and use theme color")
	}
	if strings.Contains(h, "javascript:") {
		t.Fatal("unsafe link must be dropped")
	}
	if !strings.Contains(h, "© 站点") {
		t.Fatal("footer brand block missing")
	}
}

func TestHeaderNameOverride(t *testing.T) {
	vars := map[string]string{"cta_url": "https://a.com/r/t"}
	mk := func(lay LayoutConfig) string {
		_, h := renderMail(MailInput{Policy: Policy{Category: CategoryMarketing}, Subject: "s", BodyMD: "b",
			Layout: lay, Vars: vars, SiteName: "站点", UnsubURL: "https://a.com/u"})
		return h
	}
	// header=text + 空 HeaderName → 用站点名
	if h := mk(LayoutConfig{Header: HeaderText}); !strings.Contains(h, ">站点<") {
		t.Fatal("text header falls back to site name when header_name empty")
	}
	// header=text + 自定义名称 → 用自定义名称
	if h := mk(LayoutConfig{Header: HeaderText, HeaderName: "我的品牌"}); !strings.Contains(h, ">我的品牌<") || strings.Contains(h, ">站点<") {
		t.Fatalf("text header must use custom header_name; got: %s", h)
	}
	// header=logo + 空 HeaderName → logo alt 用站点名
	if h := mk(LayoutConfig{Header: HeaderLogo, LogoURL: "https://a.com/l.png"}); !strings.Contains(h, `alt="站点"`) {
		t.Fatal("logo alt falls back to site name")
	}
	// header=logo + 自定义名称 → logo alt 用自定义名称
	if h := mk(LayoutConfig{Header: HeaderLogo, LogoURL: "https://a.com/l.png", HeaderName: "品牌X"}); !strings.Contains(h, `alt="品牌X"`) {
		t.Fatal("logo alt must use header_name when set")
	}
}

func TestLayoutValidation(t *testing.T) {
	good := LayoutConfig{Header: HeaderText, ThemeColor: "#123456", Style: StyleCard, Width: 600, FooterMD: "© {{site_name}}"}
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	bads := []LayoutConfig{
		{Header: HeaderLogo, ThemeColor: "#123456", Style: StyleCard, Width: 600},
		{Header: HeaderText, ThemeColor: "red;x", Style: StyleCard, Width: 600},
		{Header: HeaderText, ThemeColor: "#123456", Style: StyleCard, Width: 600, FooterMD: "{{username}}"},
		{Header: HeaderText, ThemeColor: "#123456", Style: StyleCard, Width: 600, HeaderLink: "javascript:x"},
	}
	for i, b := range bads {
		if err := b.validate(); err == nil {
			t.Fatalf("bad layout %d accepted", i)
		}
	}
}

// ─── 权益 / 批次请求 ─────────────────────────────────────────────────────────

func TestBenefitRegistry(t *testing.T) {
	r := testBenefits()
	specs := []BenefitSpec{{Kind: KindPointsGrant, Config: json.RawMessage(`{"points":200}`)}}
	if err := r.Validate(specs); err != nil {
		t.Fatal(err)
	}
	if got := r.Describe(specs); len(got) != 1 || got[0] != "200 积分" {
		t.Fatalf("describe: %v", got)
	}
	for _, bad := range [][]BenefitSpec{
		{{Kind: "nope"}},
		{{Kind: KindPointsGrant, Config: json.RawMessage(`{"points":0}`)}},
		{specs[0], specs[0]},
	} {
		if err := r.Validate(bad); err == nil {
			t.Fatalf("should reject %v", bad)
		}
	}
}

func TestBatchRequestNormalize(t *testing.T) {
	r := BatchRequest{Mode: PickUsers, Emails: []string{" A@x.com", "a@x.com", ""}}
	if err := r.normalize(); err != nil || len(r.Emails) != 1 || r.Scope != ScopeFresh {
		t.Fatalf("normalize: %+v %v", r, err)
	}
	for _, bad := range []BatchRequest{{Mode: PickRandom}, {Mode: "x"}, {Scope: "x"}, {Mode: PickUsers}} {
		if err := bad.normalize(); err == nil {
			t.Fatalf("should reject %+v", bad)
		}
	}
}

func TestMetricWindowsAreFixedInDefinition(t *testing.T) {
	m := NewMetricRegistry(testPurposes())
	v := m.Views([]string{"returned_7d", "paid_14d"}, map[string]int{"returned_7d": 3})
	if v[0].Label != "7 日内回访" || v[0].Value != 3 || !v[0].Primary || v[1].Label != "14 日内付费" {
		t.Fatalf("views: %+v", v)
	}
	if !strings.Contains(ReturnedWithin(7).Cond.sql, "INTERVAL '7 days'") {
		t.Fatal("window must be part of the metric definition")
	}
}

func TestMetricKeyConflictPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("same key with different definitions must panic")
		}
	}()
	r := NewPurposeRegistry()
	r.Register(&purposeDef{key: "a", metrics: []MetricSpec{ClickedMetric()}})
	r.Register(&purposeDef{key: "b", metrics: []MetricSpec{{Key: MetricClicked, Cond: sqlf("TRUE")}}})
	NewMetricRegistry(r)
}

func TestCampaignClaimDays(t *testing.T) {
	base := CampaignInput{Name: "n", Subject: "s", BodyMD: "b"}
	in := base
	if err := in.normalize(); err != nil || in.ClaimDays != DefaultClaimDays {
		t.Fatalf("no benefits → default claim days: %+v %v", in, err)
	}
	in = base
	in.Benefits, in.ClaimDays = []BenefitSpec{{Kind: KindPointsGrant}}, 400
	if err := in.normalize(); err == nil {
		t.Fatal("claim days > 365 must be rejected")
	}
}

func TestDeliveryConfigValidate(t *testing.T) {
	c := DefaultDeliveryConfig()
	c.LinkBaseURL = " https://a.com/ "
	c.PurposeGapDays = nil
	if err := c.Validate(); err != nil || c.LinkBaseURL != "https://a.com" || c.PurposeGapDays == nil {
		t.Fatalf("normalize: %+v %v", c, err)
	}
	c.PurposeGapDays = map[string]int{"x": -1}
	if c.Validate() == nil {
		t.Fatal("negative gap must be rejected")
	}
}
