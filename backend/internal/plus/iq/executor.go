// Package iq — [plus] 执行一次检测：解析目标 → 生成 → 提取作品 → 落库 → 清理旧样本 → 异步反查账号
package iq

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// attributionDelays 用量日志可能延迟几秒写入，按这个节奏重试反查
var attributionDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 40 * time.Second}

// ExecutorDeps 执行器依赖
type ExecutorDeps struct {
	Config     ConfigStore
	Tasks      TaskStore
	Runs       RunQueue
	Catalog    GroupCatalog
	Channel    Channel
	Challenge  Challenge
	Client     LLMClient
	Attributor Attributor
	Now        func() time.Time
	Sleep      func(ctx context.Context, d time.Duration) bool // 单测可注入
}

// Executor 执行检测
type Executor struct{ d ExecutorDeps }

func NewExecutor(d ExecutorDeps) *Executor {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = func(ctx context.Context, dur time.Duration) bool {
			t := time.NewTimer(dur)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return false
			case <-t.C:
				return true
			}
		}
	}
	return &Executor{d: d}
}

// Execute 执行一条已进入 running 状态的检测记录（同步，耗时可能几分钟）
func (e *Executor) Execute(ctx context.Context, run Run) {
	started := e.d.Now()
	if run.StartedAt != nil {
		started = *run.StartedAt
	}
	out, keyID := e.generate(ctx, run, started)
	out.DurationMs = int(e.d.Now().Sub(started) / time.Millisecond)

	// 落库用独立 ctx：生成超时后 ctx 已取消，结果仍要写回
	wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := e.d.Runs.Finish(wctx, run.ID, keyID, out); err != nil {
		log.Printf("[plus.iq] finish run %d: %v", run.ID, err)
		return
	}
	if task, err := e.d.Tasks.GetTask(wctx, run.TaskID); err == nil {
		if err := e.d.Runs.Prune(wctx, task.ID, task.Retention); err != nil {
			log.Printf("[plus.iq] prune task %d: %v", task.ID, err)
		}
	}
	if out.ClientRequestID != "" && e.d.Attributor != nil {
		go e.attribute(run.ID, out.ClientRequestID, started)
	} else {
		_ = e.d.Runs.SaveAttribution(wctx, run.ID, nil)
	}
}

func failed(msg string) RunOutcome { return RunOutcome{Status: RunFailed, Error: truncate(msg, 500)} }

func (e *Executor) generate(ctx context.Context, run Run, started time.Time) (RunOutcome, *int64) {
	cfg, _, err := e.d.Config.LoadConfig(ctx)
	if err != nil {
		return failed("读取配置失败：" + err.Error()), nil
	}
	task, err := e.d.Tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return failed("任务不存在或已删除"), nil
	}
	group, ok := e.findGroup(ctx, task.GroupID)
	if !ok {
		return failed("分组不存在或已停用"), &task.APIKeyID
	}
	target, err := e.d.Channel.Resolve(ctx, *task, group, cfg)
	if err != nil {
		return failed(err.Error()), &task.APIKeyID
	}

	gctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	comp, err := e.d.Client.Complete(gctx, CompletionRequest{
		Target: target, Prompt: e.d.Challenge.Prompt(cfg), MaxTokens: cfg.MaxOutputTokens, Thinking: task.Thinking,
	})
	out := RunOutcome{ClientRequestID: comp.ClientRequestID, InputTokens: comp.InputTokens, OutputTokens: comp.OutputTokens}
	if err != nil {
		if errors.Is(gctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("生成超时（%d 秒）", cfg.TimeoutSeconds)
		}
		out.Status, out.Error, out.RawExcerpt = RunFailed, truncate(err.Error(), 500), rawExcerpt(comp)
		return out, &task.APIKeyID
	}
	art, err := e.d.Challenge.Extract(comp.Text)
	if err != nil {
		out.Status, out.Error, out.RawExcerpt = RunFailed, err.Error(), rawExcerpt(comp)
		return out, &task.APIKeyID
	}
	out.Status, out.HTML, out.ExternalRefs = RunSuccess, art.HTML, art.ExternalRefs
	return out, &task.APIKeyID
}

func (e *Executor) findGroup(ctx context.Context, id int64) (GroupInfo, bool) {
	groups, err := e.d.Catalog.ListGroups(ctx)
	if err != nil {
		return GroupInfo{}, false
	}
	for _, g := range groups {
		if g.ID == id {
			return g, true
		}
	}
	return GroupInfo{}, false
}

// rawExcerpt 失败时保留一段原始输出，方便管理员排查
func rawExcerpt(c Completion) string {
	var b strings.Builder
	if c.Thinking != "" {
		_, _ = b.WriteString("【思考】\n" + truncate(c.Thinking, 1500) + "\n\n")
	}
	if c.Text != "" {
		_, _ = b.WriteString("【正文】\n" + truncate(c.Text, 3000))
	}
	return b.String()
}

// attribute 反查实际账号；查不到也要写一个空链路，表示"未知"
func (e *Executor) attribute(runID int64, clientRequestID string, since time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for i, d := range attributionDelays {
		if !e.d.Sleep(ctx, d) {
			return
		}
		hops, found, err := e.d.Attributor.Attribute(ctx, clientRequestID, since)
		if err != nil {
			log.Printf("[plus.iq] attribute run %d: %v", runID, err)
		}
		if found || i == len(attributionDelays)-1 {
			if err := e.d.Runs.SaveAttribution(ctx, runID, hops); err != nil {
				log.Printf("[plus.iq] save attribution run %d: %v", runID, err)
			}
			return
		}
	}
}
