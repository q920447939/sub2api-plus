//go:build unit

package channelstatus

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fixedSource map[int64]map[int64]Signal

func (f fixedSource) Fetch(context.Context, []int64, time.Time, time.Time, time.Duration) (map[int64]map[int64]Signal, error) {
	out := map[int64]map[int64]Signal{}
	for g, m := range f {
		out[g] = map[int64]Signal{}
		for b, s := range m {
			out[g][b] = s
		}
	}
	return out, nil
}

type fixedCounter map[int64]map[int64]int64

func (f fixedCounter) Count(context.Context, []int64, time.Time, time.Time, time.Duration) (map[int64]map[int64]int64, error) {
	return f, nil
}

func TestExclusionSource_SubtractsFromRequestsAndErrors(t *testing.T) {
	// 特价分组实况：成功 433、错误 1008，其中 946 为余额不足
	src := fixedSource{11: {0: {Requests: 1441, Errors: 1008}, 60: {Requests: 5, Errors: 5}}}
	ex := fixedCounter{11: {0: 946, 60: 5}}
	on := NewExclusionSource(src, ex, func(context.Context) bool { return true })
	got, err := on.Fetch(context.Background(), []int64{11}, time.Time{}, time.Time{}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if s := got[11][0]; s.Requests != 495 || s.Errors != 62 {
		t.Fatalf("want 495/62, got %+v", s)
	}
	if _, ok := got[11][60]; ok {
		t.Fatal("全部被排除的桶应视为无数据")
	}
	// 排除后错误率 12.5%，默认规则下不再是 0 分
	if sc := (DefaultScorer{}).Score(got[11][0], DefaultRules()); sc == nil || *sc < 30 {
		t.Fatalf("score too low: %v", sc)
	}

	off := NewExclusionSource(src, ex, func(context.Context) bool { return false })
	got, _ = off.Fetch(context.Background(), []int64{11}, time.Time{}, time.Time{}, time.Minute)
	if got[11][0].Errors != 1008 {
		t.Fatal("关闭时不应扣除")
	}
}

func TestSubtractExcluded_Clamps(t *testing.T) {
	if s := subtractExcluded(Signal{Requests: 10, Errors: 2}, 5); s.Requests != 5 || s.Errors != 0 {
		t.Fatalf("got %+v", s)
	}
	if s := subtractExcluded(Signal{Requests: 3, Errors: 3}, 9); s.Requests != 0 || s.Errors != 0 {
		t.Fatalf("got %+v", s)
	}
}

func TestRules_NullIgnoredCategoriesBecomesEmptyArray(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(`{"rules":{"ignored_error_categories":null}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Rules.normalize()
	b, _ := json.Marshal(cfg.Rules)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m["ignored_error_categories"].([]any); !ok {
		t.Fatalf("应输出数组: %s", b)
	}
	if !cfg.Rules.ExcludeUserSideErrors {
		t.Fatal("旧配置缺字段时应默认开启排除")
	}
}
