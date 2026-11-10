//go:build unit

package hook

import "testing"

func TestFilterPauseAndFactor(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{
		{GroupID: 1, AccountID: 10, Action: ActionPause},
		{GroupID: 1, AccountID: 11, Action: ActionFactor, Factor: 0.3},
	})
	e.SetRandom(func() float64 { return 0.5 }) // ≥ 0.3 → 拿掉
	drop := e.Filter(1, []int64{10, 11, 12}, 0)
	if drop == nil || !drop[0] || !drop[1] || drop[2] {
		t.Fatalf("unexpected drop %v", drop)
	}
	e.SetRandom(func() float64 { return 0.1 }) // < 0.3 → 保留
	drop = e.Filter(1, []int64{10, 11, 12}, 0)
	if !drop[0] || drop[1] || drop[2] {
		t.Fatalf("unexpected drop %v", drop)
	}
	hits := e.DrainHits()
	if hits[HitKey{1, 10, ActionPause}] != 2 || hits[HitKey{1, 11, ActionFactor}] != 1 {
		t.Fatalf("hits %v", hits)
	}
}

func TestFilterKeepsStickyForFactorButNotPause(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.SetRandom(func() float64 { return 0.99 })
	e.Replace([]Rule{{GroupID: 1, AccountID: 11, Action: ActionFactor, Factor: 0.3}, {GroupID: 1, AccountID: 10, Action: ActionPause}})
	if drop := e.Filter(1, []int64{11, 12}, 11); drop != nil {
		t.Fatalf("sticky account must be kept for factor: %v", drop)
	}
	if drop := e.Filter(1, []int64{10, 12}, 10); drop == nil || !drop[0] {
		t.Fatalf("paused sticky account must be dropped: %v", drop)
	}
}

func TestFilterNeverEmpties(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Action: ActionPause}})
	if drop := e.Filter(1, []int64{10}, 0); drop != nil {
		t.Fatalf("must keep the only candidate: %v", drop)
	}
	if e.DrainHits()[HitKey{1, 0, "fallback"}] != 1 {
		t.Fatal("fallback not counted")
	}
}

func TestDisabledAndOtherGroups(t *testing.T) {
	e := New()
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Action: ActionPause}})
	if e.Filter(1, []int64{10, 11}, 0) != nil || e.Paused(1, 10) {
		t.Fatal("disabled engine must not filter")
	}
	e.SetEnabled(true)
	if e.Filter(2, []int64{10, 11}, 0) != nil || e.Paused(2, 10) {
		t.Fatal("rule must be scoped to its group")
	}
	if !e.Paused(1, 10) {
		t.Fatal("paused expected")
	}
}

func TestReplaceIgnoresInvalid(t *testing.T) {
	e := New()
	e.SetEnabled(true)
	e.Replace([]Rule{{GroupID: 1, AccountID: 10, Action: ActionFactor, Factor: 1.5}, {GroupID: 1, AccountID: 11, Action: "x"}})
	if e.Filter(1, []int64{10, 11, 12}, 0) != nil {
		t.Fatal("invalid rules must be ignored")
	}
}
