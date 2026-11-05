//go:build unit

package iq

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// memStore 内存版 ConfigStore + TaskStore + RunQueue + GroupCatalog + KeyStore
type memStore struct {
	mu       sync.Mutex
	cfg      Config
	tasks    map[int64]Task
	groups   []GroupInfo
	keys     map[int64][2]any // id -> {key, groupID}
	finished map[int64]RunOutcome
	chains   map[int64][]AccountHop
	pruned   map[int64]int
	queued   []Run
	running  int
}

func newMem() *memStore {
	cfg := DefaultConfig()
	cfg.Endpoint = "http://gw"
	return &memStore{
		cfg:      cfg,
		tasks:    map[int64]Task{1: {ID: 1, GroupID: 10, Model: "claude-x", APIKeyID: 7, Retention: 3, IntervalMinutes: 30, Enabled: true, Thinking: ThinkingOff}},
		groups:   []GroupInfo{{ID: 10, Name: "G", Platform: "anthropic"}},
		keys:     map[int64][2]any{7: {"sk-7", int64(10)}},
		finished: map[int64]RunOutcome{}, chains: map[int64][]AccountHop{}, pruned: map[int64]int{},
	}
}

func (m *memStore) LoadConfig(context.Context) (Config, int64, error)        { return m.cfg, 1, nil }
func (m *memStore) SaveConfig(context.Context, Config, int64) (int64, error) { return 2, nil }
func (m *memStore) PreviewSecret(context.Context) ([]byte, error) {
	return []byte(strings.Repeat("s", 40)), nil
}
func (m *memStore) ListTasks(context.Context) ([]Task, error) {
	var out []Task
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}
func (m *memStore) GetTask(_ context.Context, id int64) (*Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &t, nil
}
func (m *memStore) CreateTask(context.Context, Task) (int64, error) { return 0, nil }
func (m *memStore) UpdateTask(context.Context, Task) error          { return nil }
func (m *memStore) DeleteTask(context.Context, int64) error         { return nil }
func (m *memStore) Enqueue(_ context.Context, t Task, tr Trigger, _ *int64, _ int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.queued {
		if r.TaskID == t.ID {
			return 0, nil
		}
	}
	id := int64(100 + len(m.queued))
	m.queued = append(m.queued, Run{ID: id, TaskID: t.ID, GroupID: t.GroupID, Trigger: tr})
	return id, nil
}
func (m *memStore) ClaimDue(_ context.Context, now time.Time, _ int) ([]Task, error) {
	var out []Task
	for id, t := range m.tasks {
		if t.Enabled && !t.NextRunAt.After(now) {
			t.NextRunAt = now.Add(time.Duration(t.IntervalMinutes) * time.Minute)
			m.tasks[id] = t
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memStore) StartQueued(_ context.Context, limit int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit > len(m.queued) {
		limit = len(m.queued)
	}
	if limit <= 0 {
		return nil, nil
	}
	out := m.queued[:limit]
	m.queued = m.queued[limit:]
	m.running += len(out)
	return out, nil
}
func (m *memStore) CountRunning(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running, nil
}
func (m *memStore) Finish(_ context.Context, id int64, _ *int64, out RunOutcome) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finished[id] = out
	m.running--
	return nil
}
func (m *memStore) SaveAttribution(_ context.Context, id int64, hops []AccountHop) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chains[id] = hops
	return nil
}
func (m *memStore) Prune(_ context.Context, taskID int64, keep int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruned[taskID] = keep
	return nil
}
func (m *memStore) FailStale(context.Context, time.Time, string) (int64, error) { return 0, nil }
func (m *memStore) ListGroups(context.Context) ([]GroupInfo, error)             { return m.groups, nil }
func (m *memStore) AccountNames(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{}, nil
}
func (m *memStore) AccountCounts(context.Context) (map[int64]int, error) { return map[int64]int{}, nil }
func (m *memStore) ResolveKey(_ context.Context, id int64) (string, int64, error) {
	k, ok := m.keys[id]
	if !ok {
		return "", 0, ErrKeyUnavailable
	}
	return k[0].(string), k[1].(int64), nil
}
func (m *memStore) ListGroupKeys(context.Context, int64) ([]KeyOption, error) { return nil, nil }
func (m *memStore) KeyInfos(context.Context, []int64) (map[int64]KeyOption, error) {
	return map[int64]KeyOption{}, nil
}

type fakeLLM struct {
	text string
	err  error
	got  CompletionRequest
}

func (f *fakeLLM) Complete(_ context.Context, req CompletionRequest) (Completion, error) {
	f.got = req
	return Completion{Text: f.text, ClientRequestID: "cid-1", OutputTokens: intp(42)}, f.err
}

type fakeAttr struct{ calls int }

func (f *fakeAttr) Attribute(context.Context, string, time.Time) ([]AccountHop, bool, error) {
	f.calls++
	if f.calls < 2 { // 第一次还没写入用量日志
		return nil, false, nil
	}
	return []AccountHop{{AccountID: 5, OK: true}}, true, nil
}

func newTestExecutor(m *memStore, llm LLMClient, attr Attributor) *Executor {
	return NewExecutor(ExecutorDeps{
		Config: m, Tasks: m, Runs: m, Catalog: m, Channel: GatewayChannel{Keys: m},
		Challenge: PelicanChallenge{}, Client: llm, Attributor: attr,
		Sleep: func(context.Context, time.Duration) bool { return true },
	})
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout")
}

func TestExecutorSuccess(t *testing.T) {
	m := newMem()
	llm := &fakeLLM{text: "```html\n<!DOCTYPE html><html><body><svg></svg></body></html>\n```"}
	attr := &fakeAttr{}
	newTestExecutor(m, llm, attr).Execute(context.Background(), Run{ID: 1, TaskID: 1})

	out := m.finished[1]
	if out.Status != RunSuccess || !strings.HasPrefix(out.HTML, "<!DOCTYPE html>") || out.ClientRequestID != "cid-1" {
		t.Fatalf("outcome = %+v", out)
	}
	if llm.got.Target.Endpoint != "http://gw" || llm.got.Target.APIKey != "sk-7" || llm.got.Target.Format != FormatAnthropic {
		t.Fatalf("target = %+v", llm.got.Target)
	}
	if llm.got.Prompt != PelicanPrompt {
		t.Fatal("prompt should be the pelican prompt")
	}
	if m.pruned[1] != 3 {
		t.Fatal("prune with task retention expected")
	}
	waitFor(t, func() bool { m.mu.Lock(); defer m.mu.Unlock(); return len(m.chains[1]) == 1 })
	if attr.calls != 2 {
		t.Fatalf("attribution should retry until found, calls=%d", attr.calls)
	}
}

func TestExecutorFailures(t *testing.T) {
	t.Run("no html keeps excerpt", func(t *testing.T) {
		m := newMem()
		newTestExecutor(m, &fakeLLM{text: "我只描述一下"}, &fakeAttr{}).Execute(context.Background(), Run{ID: 2, TaskID: 1})
		out := m.finished[2]
		if out.Status != RunFailed || !strings.Contains(out.RawExcerpt, "我只描述一下") || !errors.Is(ErrNoHTML, ErrNoHTML) {
			t.Fatalf("outcome = %+v", out)
		}
	})
	t.Run("upstream error", func(t *testing.T) {
		m := newMem()
		newTestExecutor(m, &fakeLLM{err: &HTTPError{Status: 502, Message: "bad"}}, &fakeAttr{}).Execute(context.Background(), Run{ID: 3, TaskID: 1})
		if out := m.finished[3]; out.Status != RunFailed || !strings.Contains(out.Error, "502") || out.ClientRequestID != "cid-1" {
			t.Fatalf("outcome = %+v", out)
		}
	})
	t.Run("key moved to another group", func(t *testing.T) {
		m := newMem()
		m.keys[7] = [2]any{"sk-7", int64(99)}
		llm := &fakeLLM{}
		newTestExecutor(m, llm, &fakeAttr{}).Execute(context.Background(), Run{ID: 4, TaskID: 1})
		if out := m.finished[4]; out.Status != RunFailed || !strings.Contains(out.Error, "不属于该分组") {
			t.Fatalf("outcome = %+v", out)
		}
		if llm.got.Prompt != "" {
			t.Fatal("must not call the model")
		}
	})
}

func TestSchedulerRespectsConcurrency(t *testing.T) {
	m := newMem()
	m.cfg.Concurrency = 1
	m.tasks[2] = Task{ID: 2, GroupID: 10, Model: "other", APIKeyID: 7, Retention: 3, IntervalMinutes: 30, Enabled: true, Thinking: ThinkingOff}
	block := make(chan struct{})
	llm := &blockingLLM{release: block}
	s := NewScheduler(m, m, newTestExecutor(m, llm, &fakeAttr{}))
	s.Tick(context.Background())
	waitFor(t, func() bool { return llm.started() == 1 })
	if len(m.queued) != 1 {
		t.Fatalf("second task should wait in queue, queued=%d", len(m.queued))
	}
	s.Tick(context.Background()) // 并发已满，不应再启动
	if llm.started() != 1 {
		t.Fatal("concurrency limit exceeded")
	}
	close(block)
	s.Wait()
	s.Tick(context.Background())
	s.Wait()
	if len(m.finished) != 2 {
		t.Fatalf("finished = %d", len(m.finished))
	}
}

type blockingLLM struct {
	mu      sync.Mutex
	n       int
	release chan struct{}
}

func (b *blockingLLM) started() int { b.mu.Lock(); defer b.mu.Unlock(); return b.n }
func (b *blockingLLM) Complete(context.Context, CompletionRequest) (Completion, error) {
	b.mu.Lock()
	b.n++
	b.mu.Unlock()
	<-b.release
	return Completion{Text: "<html></html>"}, nil
}
