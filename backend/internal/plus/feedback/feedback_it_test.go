//go:build integration

package feedback

// 集成测试：需要一个已应用全部迁移（含 263_plus_feedback.sql）的独立库。
// 运行：PLUS_FEEDBACK_IT_DSN='postgres://...' go test -tags integration ./internal/plus/feedback -run IT -v
// 测试会清空 plus_feedback_* / plus_points_* 与 users 表，切勿指向业务库。

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func openIT(t *testing.T) (*sql.DB, *Service) {
	t.Helper()
	dsn := os.Getenv("PLUS_FEEDBACK_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_FEEDBACK_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, tbl := range []string{"plus_feedback_messages", "plus_feedback_attachments", "plus_feedback", "plus_feedback_config", "plus_points_ledger", "plus_points_accounts", "users"} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	svc := NewService(Deps{
		Repo: NewRepository(db), Accounts: sqlAccounts{db: db}, Errors: sqlErrorLogs{db: db},
		Blobs: localBlobs{root: t.TempDir()}, Rewarder: pointsRewarder{},
	})
	// 测试里连续提交，关掉提交间隔
	cfg := DefaultConfig()
	cfg.Limits.IntervalSeconds = 0
	if _, err := svc.SaveConfig(context.Background(), cfg, 0); err != nil {
		t.Fatal(err)
	}
	return db, svc
}

func seedUser(t *testing.T, db *sql.DB, email, role string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`INSERT INTO users (email, password_hash, balance, role, created_at, updated_at)
		VALUES ($1, 'x', 12.5, $2, NOW(), NOW()) RETURNING id`, email, role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func pngBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	must(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestITIssueLifecycle(t *testing.T) {
	db, svc := openIT(t)
	ctx := context.Background()
	uid := seedUser(t, db, "fb_user@example.com", "user")
	other := seedUser(t, db, "fb_other@example.com", "user")
	admin := seedUser(t, db, "fb_admin@example.com", "admin")

	attID, err := svc.Upload(ctx, uid, pngBytes(t))
	must(t, err)

	// 场景不属于该类型
	if _, err := svc.Submit(ctx, uid, SubmitInput{Kind: KindIssue, Scenario: "new_model", Content: "这是一段足够长的描述内容"}, ClientInfo{}); err == nil {
		t.Fatal("建议场景不能用于问题")
	}
	f, err := svc.Submit(ctx, uid, SubmitInput{
		Kind: KindIssue, Scenario: "call_error", Content: "调用 claude 时一直返回 502 错误",
		Extra: map[string]string{"model": "claude-sonnet", "junk": "x"}, Attachments: []int64{attID}, Page: "/keys",
	}, ClientInfo{IP: "1.2.3.4", UserAgent: "UA"})
	must(t, err)

	// 别人上传的图不能挂到自己的反馈上；已绑定的图不能再绑
	if _, err := svc.Submit(ctx, other, SubmitInput{Kind: KindIssue, Scenario: "other_issue", Content: "偷别人的图片来提交反馈", Attachments: []int64{attID}}, ClientInfo{}); err == nil {
		t.Fatal("不能绑定别人的附件")
	}

	// 越权
	if _, err := svc.MineDetail(ctx, other, f.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("别人的反馈应 404，got %v", err)
	}
	if _, _, err := svc.OpenAttachment(ctx, other, false, attID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("别人的附件应 404，got %v", err)
	}
	rc, mime, err := svc.OpenAttachment(ctx, uid, false, attID)
	must(t, err)
	_ = rc.Close()
	if mime != "image/png" {
		t.Fatalf("mime=%s", mime)
	}

	// 追问 → 待补充，用户端红点
	_, err = svc.AdminMessage(ctx, admin, f.ID, MessageInput{Kind: MsgReply, Body: "请提供报错时间"})
	must(t, err)
	_, err = svc.AdminMessage(ctx, admin, f.ID, MessageInput{Kind: MsgNote, Body: "疑似上游问题"})
	must(t, err)
	if n, _ := svc.Unread(ctx, uid); n != 1 {
		t.Fatalf("unread=%d", n)
	}
	d, err := svc.MineDetail(ctx, uid, f.ID)
	must(t, err)
	if d.Feedback.Status != StatusAwaiting || d.Feedback.Context != nil {
		t.Fatalf("status=%s ctx=%s", d.Feedback.Status, d.Feedback.Context)
	}
	for _, m := range d.Messages {
		if m.Internal {
			t.Fatal("用户端不能看到内部备注")
		}
	}
	if n, _ := svc.Unread(ctx, uid); n != 0 {
		t.Fatal("打开详情后应清除未读")
	}

	// 用户补充 → 待处理
	_, err = svc.Supplement(ctx, uid, f.ID, MessageInput{Body: "大约下午三点"})
	must(t, err)
	if pc, _ := svc.PendingCount(ctx); pc != 1 {
		t.Fatalf("pending=%d", pc)
	}

	// 定级 + 标签 + 改场景
	sev := SeverityMajor
	tags := []string{"上游", " 上游 ", "502"}
	sc := "speed"
	must(t, svc.AdminUpdate(ctx, admin, f.ID, UpdateInput{Severity: &sev, Tags: &tags, Scenario: &sc}))

	// 问题不能发积分
	if err := svc.Close(ctx, admin, f.ID, CloseRequest{Verdict: StatusResolved, RewardPoints: 500}); err == nil {
		t.Fatal("问题不能发积分")
	}
	must(t, svc.Close(ctx, admin, f.ID, CloseRequest{Verdict: StatusResolved, Message: "已修复"}))
	if _, err := svc.Supplement(ctx, uid, f.ID, MessageInput{Body: "还有问题"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("结案后不能补充，got %v", err)
	}

	ad, err := svc.AdminDetail(ctx, f.ID)
	must(t, err)
	if ad.Feedback.Scenario != "speed" || ad.Feedback.ScenarioUser != "call_error" || len(ad.Feedback.Tags) != 2 {
		t.Fatalf("detail=%+v", ad.Feedback)
	}
	if ad.User == nil || ad.User.Email != "fb_user@example.com" || ad.User.Accepted != 1 {
		t.Fatalf("user card=%+v", ad.User)
	}

	// 关联上一条
	prev := f.ID
	f2, err := svc.Submit(ctx, uid, SubmitInput{Kind: KindIssue, Scenario: "speed", Content: "修复后又开始变慢了，麻烦再看看", PrevID: &prev}, ClientInfo{})
	must(t, err)
	d2, err := svc.MineDetail(ctx, uid, f2.ID)
	must(t, err)
	if len(d2.Related) != 1 || d2.Related[0].Relation != "prev" {
		t.Fatalf("related=%+v", d2.Related)
	}

	list, total, _, _, err := svc.AdminList(ctx, AdminQuery{Status: "open", Content: "变慢"})
	must(t, err)
	if total != 1 || list[0].ID != f2.ID {
		t.Fatalf("admin list total=%d", total)
	}
	// 分字段模糊查询：用户（邮箱片段）、ID（带 #）、通配符按字面匹配
	for _, c := range []struct {
		q    AdminQuery
		want int64
	}{
		{AdminQuery{User: "fb_user"}, 2},
		{AdminQuery{User: "nobody"}, 0},
		{AdminQuery{ID: fmt.Sprintf("#%d", f2.ID)}, 1},
		{AdminQuery{Content: "%"}, 0},
	} {
		_, n, _, _, err := svc.AdminList(ctx, c.q)
		must(t, err)
		if n < c.want || (c.want == 0 && n != 0) {
			t.Fatalf("query %+v total=%d want %d", c.q, n, c.want)
		}
	}
	mine, total, _, _, err := svc.ListMine(ctx, UserQuery{UserID: uid})
	must(t, err)
	if total != 2 || len(mine) != 2 {
		t.Fatalf("mine total=%d", total)
	}
	// 按类型筛选：这两条都是问题
	if _, n, _, _, err := svc.ListMine(ctx, UserQuery{UserID: uid, Kind: KindSuggestion}); err != nil || n != 0 {
		t.Fatalf("kind=suggestion total=%d err=%v", n, err)
	}
	if _, n, _, _, err := svc.ListMine(ctx, UserQuery{UserID: uid, Kind: KindIssue}); err != nil || n != 2 {
		t.Fatalf("kind=issue total=%d err=%v", n, err)
	}
}

func TestITSuggestionReward(t *testing.T) {
	db, svc := openIT(t)
	ctx := context.Background()
	a := seedUser(t, db, "fb_a@example.com", "user")
	admin := seedUser(t, db, "fb_admin2@example.com", "admin")

	main, err := svc.Submit(ctx, a, SubmitInput{Kind: KindSuggestion, Scenario: "new_model", Content: "希望接入 gemini 3 pro 模型"}, ClientInfo{})
	must(t, err)
	if err := svc.Close(ctx, admin, main.ID, CloseRequest{Verdict: StatusDuplicate}); err == nil {
		t.Fatal("重复已下线")
	}
	if err := svc.Close(ctx, admin, main.ID, CloseRequest{Verdict: StatusResolved}); err == nil {
		t.Fatal("建议不能标已解决")
	}

	st, err := svc.Stats(ctx, 30)
	must(t, err)
	if len(st.Pool) != 1 || st.Pool[0].ID != main.ID || st.Pool[0].Impact != 1 {
		t.Fatalf("pool=%+v", st.Pool)
	}

	must(t, svc.Close(ctx, admin, main.ID, CloseRequest{Verdict: StatusAdopted, RewardPoints: 600}))
	var bal int64
	must(t, db.QueryRow(`SELECT balance FROM plus_points_accounts WHERE user_id = $1`, a).Scan(&bal))
	if bal != 600 {
		t.Fatalf("balance=%d", bal)
	}
	var reason string
	var ref int64
	must(t, db.QueryRow(`SELECT reason, ref_id FROM plus_points_ledger WHERE user_id = $1`, a).Scan(&reason, &ref))
	if reason != ReasonFeedbackReward || ref != main.ID {
		t.Fatalf("ledger reason=%s ref=%d", reason, ref)
	}
	// 结案只能一次，积分不会重复发
	if err := svc.Close(ctx, admin, main.ID, CloseRequest{Verdict: StatusAdopted, RewardPoints: 600}); !errors.Is(err, ErrClosed) {
		t.Fatalf("重复结案应失败，got %v", err)
	}

	st, err = svc.Stats(ctx, 7)
	must(t, err)
	if st.Overview.Suggestions != 1 || st.Overview.Adopted != 1 || st.Overview.RewardPoints != 600 || len(st.Trend) != 7 || len(st.Users) != 1 {
		t.Fatalf("overview=%+v users=%d trend=%d", st.Overview, len(st.Users), len(st.Trend))
	}
	if st.Users[0].Scenarios["new_model"] != 1 {
		t.Fatalf("user mix=%+v", st.Users[0].Scenarios)
	}
}

func TestITAutoCloseAndRate(t *testing.T) {
	db, svc := openIT(t)
	ctx := context.Background()
	u := seedUser(t, db, "fb_auto@example.com", "user")
	admin := seedUser(t, db, "fb_admin3@example.com", "admin")

	f, err := svc.Submit(ctx, u, SubmitInput{Kind: KindIssue, Scenario: "account", Content: "登录后页面一直空白怎么办"}, ClientInfo{})
	must(t, err)
	_, err = svc.AdminMessage(ctx, admin, f.ID, MessageInput{Kind: MsgReply, Body: "用的什么浏览器？"})
	must(t, err)

	n, err := svc.AutoCloseExpired(ctx)
	must(t, err)
	if n != 0 {
		t.Fatal("没到期不应关闭")
	}
	svc.Now = func() time.Time { return time.Now().AddDate(0, 0, 8) }
	n, err = svc.AutoCloseExpired(ctx)
	must(t, err)
	if n != 1 {
		t.Fatalf("closed=%d", n)
	}
	got, err := svc.Repo.Get(ctx, f.ID)
	must(t, err)
	if got.Status != StatusDeclined || got.CloseReason == "" || !got.UserUnread {
		t.Fatalf("got %+v", got)
	}

	// 同时未结案上限
	svc.Now = time.Now
	cfg, ver, _ := svc.AdminConfig(ctx)
	cfg.Limits.OpenMax = 1
	_, err = svc.SaveConfig(ctx, cfg, ver)
	must(t, err)
	_, err = svc.Submit(ctx, u, SubmitInput{Kind: KindSuggestion, Scenario: "ux", Content: "希望列表页支持按时间排序"}, ClientInfo{})
	must(t, err)
	_, err = svc.Submit(ctx, u, SubmitInput{Kind: KindSuggestion, Scenario: "ux", Content: "希望列表页支持按时间排序"}, ClientInfo{})
	var re *RateError
	if !errors.As(err, &re) {
		t.Fatalf("应触发未结案上限，got %v", err)
	}

	res, err := svc.Batch(ctx, admin, BatchInput{IDs: []int64{f.ID}, AddTag: "浏览器"})
	must(t, err)
	if res.Done != 1 {
		t.Fatalf("batch=%+v", res)
	}
	tags, err := svc.Tags(ctx)
	must(t, err)
	if len(tags) != 1 || tags[0] != "浏览器" {
		t.Fatalf("tags=%v", tags)
	}
}
