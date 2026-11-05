//go:build integration

package iq

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// 用独立 schema 隔离：上游表只建本模块读到的列，plus_iq_* 直接执行迁移文件
func openITDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PLUS_IQ_IT_DSN")
	if dsn == "" {
		t.Skip("PLUS_IQ_IT_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // search_path 只在本连接生效
	schema := fmt.Sprintf("iqit_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		_ = db.Close()
	})
	ddl, err := os.ReadFile("../../../migrations/258_plus_iq.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE SCHEMA ` + schema,
		`SET search_path TO ` + schema,
		`CREATE TABLE groups (id BIGINT PRIMARY KEY, name TEXT, platform TEXT, status TEXT DEFAULT 'active', deleted_at TIMESTAMPTZ)`,
		`CREATE TABLE accounts (id BIGINT PRIMARY KEY, name TEXT, deleted_at TIMESTAMPTZ)`,
		`CREATE TABLE account_groups (account_id BIGINT, group_id BIGINT)`,
		`CREATE TABLE users (id BIGINT PRIMARY KEY, email TEXT)`,
		`CREATE TABLE api_keys (id BIGINT PRIMARY KEY, user_id BIGINT, name TEXT, key TEXT, group_id BIGINT, status TEXT DEFAULT 'active', expires_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ)`,
		`CREATE TABLE usage_logs (id BIGSERIAL PRIMARY KEY, request_id TEXT, account_id BIGINT, created_at TIMESTAMPTZ DEFAULT NOW())`,
		`CREATE TABLE ops_error_logs (id BIGSERIAL PRIMARY KEY, client_request_id TEXT, account_id BIGINT, status_code INT, error_message TEXT, upstream_errors JSONB, created_at TIMESTAMPTZ DEFAULT NOW())`,
		string(ddl),
		`INSERT INTO groups (id, name, platform) VALUES (1, 'Claude 官转', 'anthropic'), (2, 'GPT', 'openai'), (3, 'old', 'openai')`,
		`UPDATE groups SET deleted_at = NOW() WHERE id = 3`,
		`INSERT INTO accounts (id, name) VALUES (11, '中转A'), (12, '中转B')`,
		`INSERT INTO account_groups VALUES (11, 1), (12, 1)`,
		`INSERT INTO users VALUES (1, 'admin@x'), (2, 'ops@x')`,
		`INSERT INTO api_keys (id, user_id, name, key, group_id) VALUES (7, 1, 'IQ-Claude 官转', 'sk-aaaaaaaaaaaa1234', 1), (8, 2, 'ops', 'sk-bbbbbbbbbbbb5678', 1)`,
		`INSERT INTO api_keys (id, user_id, name, key, group_id, status) VALUES (9, 1, 'off', 'sk-cccccccccccc0000', 1, 'disabled')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

func TestRepositoryIT(t *testing.T) {
	db := openITDB(t)
	r := NewRepository(db)
	ctx := context.Background()

	// 配置：迁移插入了默认行（version 0）和预览密钥
	cfg, ver, err := r.LoadConfig(ctx)
	if err != nil || ver != 0 || cfg.Prompt != PelicanPrompt || cfg.Concurrency != 2 {
		t.Fatalf("load default: %+v %d %v", cfg, ver, err)
	}
	if sec, err := r.PreviewSecret(ctx); err != nil || len(sec) < 32 {
		t.Fatalf("secret: %v", err)
	}
	cfg.UserEnabled = true
	if v, err := r.SaveConfig(ctx, cfg, 0); err != nil || v != 1 {
		t.Fatalf("save: %d %v", v, err)
	}
	if _, err := r.SaveConfig(ctx, cfg, 0); !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("stale version must conflict: %v", err)
	}

	// 只读上游
	groups, _ := r.ListGroups(ctx)
	if len(groups) != 2 {
		t.Fatalf("deleted group must be hidden: %+v", groups)
	}
	keys, _ := r.ListGroupKeys(ctx, 1)
	if len(keys) != 2 || keys[0].Masked != "sk-aaa****1234" || keys[1].UserEmail != "ops@x" {
		t.Fatalf("keys: %+v", keys)
	}
	if _, _, err := r.ResolveKey(ctx, 9); !errors.Is(err, ErrKeyUnavailable) {
		t.Fatal("disabled key must be unavailable")
	}
	infos, _ := r.KeyInfos(ctx, []int64{7, 9})
	if !infos[7].Active || infos[9].Active {
		t.Fatalf("key infos: %+v", infos)
	}
	if counts, _ := r.AccountCounts(ctx); counts[1] != 2 {
		t.Fatalf("counts: %v", counts)
	}

	// 任务
	id, err := r.CreateTask(ctx, Task{GroupID: 1, Model: "claude-x", KeyMode: KeyAuto, APIKeyID: 7, Thinking: ThinkingOff, IntervalMinutes: 30, Retention: 2, Enabled: true, CreatedBy: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateTask(ctx, Task{GroupID: 1, Model: "claude-x", KeyMode: KeyAuto, APIKeyID: 7, Thinking: ThinkingOff, IntervalMinutes: 30, Retention: 2, CreatedBy: 1}); !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("duplicate: %v", err)
	}

	// 到期抢占：新任务 next_run_at = NOW，抢到后推进一个间隔，再抢不到
	now := time.Now().Add(time.Second)
	due, err := r.ClaimDue(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].ID != id {
		t.Fatalf("claim: %+v %v", due, err)
	}
	if again, _ := r.ClaimDue(ctx, now, 10); len(again) != 0 {
		t.Fatal("claimed twice")
	}
	task, _ := r.GetTask(ctx, id)
	if d := task.NextRunAt.Sub(now); d < 29*time.Minute || d > 31*time.Minute {
		t.Fatalf("next run not advanced: %v", d)
	}

	// 队列：同一任务只能有一条排队/运行中
	run1, _ := r.Enqueue(ctx, *task, TriggerCron, nil, 1)
	if dup, _ := r.Enqueue(ctx, *task, TriggerManual, nil, 1); run1 == 0 || dup != 0 {
		t.Fatalf("enqueue dedupe: %d %d", run1, dup)
	}
	started, _ := r.StartQueued(ctx, 5)
	if len(started) != 1 || started[0].Status != RunRunning || started[0].StartedAt == nil {
		t.Fatalf("start: %+v", started)
	}
	if n, _ := r.CountRunning(ctx); n != 1 {
		t.Fatal("running count")
	}
	if err := r.Finish(ctx, run1, nil, RunOutcome{Status: RunSuccess, ClientRequestID: "cid-1", DurationMs: 1200, OutputTokens: intp(900), HTML: "<html></html>", ExternalRefs: []string{"https://x"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetRun(ctx, run1)
	if !got.HasHTML || got.APIKeyID == nil || *got.APIKeyID != 7 || *got.OutputTokens != 900 || len(got.ExternalRefs) != 1 || len(got.AccountChain) != 0 {
		t.Fatalf("run: %+v", got)
	}

	// 归因：成功走 usage_logs，失败走 ops_error_logs（含故障切换事件）
	attr := UsageLogAttributor{DB: db}
	if _, found, _ := attr.Attribute(ctx, "cid-1", time.Now().Add(-time.Minute)); found {
		t.Fatal("nothing written yet")
	}
	mustExecIT(t, db, `INSERT INTO usage_logs (request_id, account_id) VALUES ('client:cid-1', 12)`)
	mustExecIT(t, db, `INSERT INTO ops_error_logs (client_request_id, account_id, status_code, error_message, upstream_errors)
		VALUES ('cid-2', 12, 502, 'all failed', '[{"account_id":11,"upstream_status_code":529,"message":"overloaded"},{"account_id":12,"upstream_status_code":502}]')`)
	hops, found, err := attr.Attribute(ctx, "cid-1", time.Now().Add(-time.Minute))
	if err != nil || !found || len(hops) != 1 || hops[0].AccountID != 12 || !hops[0].OK {
		t.Fatalf("success hops: %+v %v", hops, err)
	}
	hops, _, _ = attr.Attribute(ctx, "cid-2", time.Now().Add(-time.Minute))
	if len(hops) != 2 || hops[0].AccountID != 11 || hops[0].StatusCode != 529 || hops[1].AccountID != 12 || hops[1].OK {
		t.Fatalf("failover hops: %+v", hops)
	}
	_ = r.SaveAttribution(ctx, run1, []AccountHop{{AccountID: 12, OK: true}})
	if runs, total, _ := r.ListRuns(ctx, RunFilter{AccountID: 12}); total != 1 || runs[0].ID != run1 {
		t.Fatal("filter by account")
	}
	if _, total, _ := r.ListRuns(ctx, RunFilter{AccountID: 11}); total != 0 {
		t.Fatal("filter by other account")
	}

	// 再造 3 成功 + 1 失败，保留 2 条：成功剩 2 条，失败 1 条不受影响
	for i := 0; i < 4; i++ {
		rid, _ := r.Enqueue(ctx, *task, TriggerManual, intp64(1), 1)
		_, _ = r.StartQueued(ctx, 1)
		st := RunSuccess
		if i == 3 {
			st = RunFailed
		}
		_ = r.Finish(ctx, rid, nil, RunOutcome{Status: st, HTML: map[bool]string{true: "<html></html>"}[st == RunSuccess], Error: "x"})
	}
	if err := r.Prune(ctx, id, 2); err != nil {
		t.Fatal(err)
	}
	recent, _ := r.RecentRuns(ctx, []int64{id}, 10, true)
	all, _ := r.RecentRuns(ctx, []int64{id}, 10, false)
	if len(recent[id]) != 2 || len(all[id]) != 3 || recent[id][0].ID < recent[id][1].ID {
		t.Fatalf("prune/recent: success=%d all=%d", len(recent[id]), len(all[id]))
	}
	if _, _, _, err := r.RunHTML(ctx, all[id][0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed run has no html")
	}
	if st, _ := r.Stats(ctx, time.Now().Add(-time.Hour)); st.Finished != 3 || st.Success != 2 {
		t.Fatalf("stats: %+v", st)
	}

	// 中断的运行记录
	rid, _ := r.Enqueue(ctx, *task, TriggerManual, nil, 1)
	_, _ = r.StartQueued(ctx, 1)
	if active, _ := r.ActiveByTask(ctx); active[id] != RunRunning {
		t.Fatal("active by task")
	}
	if n, _ := r.FailStale(ctx, time.Now().Add(time.Second), "中断"); n != 1 {
		t.Fatal("fail stale")
	}
	if got, _ := r.GetRun(ctx, rid); got.Status != RunFailed || got.Error != "中断" {
		t.Fatalf("stale: %+v", got)
	}

	// 更新与删除（级联删除记录）
	task.Model, task.IntervalMinutes = "claude-y", 5
	if err := r.UpdateTask(ctx, *task); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := r.ListRuns(ctx, RunFilter{}); total != 0 {
		t.Fatal("runs must cascade")
	}
}

func intp64(v int64) *int64 { return &v }

func mustExecIT(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}
