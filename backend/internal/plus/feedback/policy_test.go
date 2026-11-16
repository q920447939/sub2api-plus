//go:build unit

package feedback

import (
	"errors"
	"testing"
)

func TestValidateClose(t *testing.T) {
	issue := &Feedback{ID: 1, Kind: KindIssue, Status: StatusPending}
	idea := &Feedback{ID: 2, Kind: KindSuggestion, Status: StatusAwaiting}
	closed := &Feedback{ID: 3, Kind: KindSuggestion, Status: StatusAdopted}

	cases := []struct {
		name string
		f    *Feedback
		req  CloseRequest
		ok   bool
	}{
		{"问题可以已解决", issue, CloseRequest{Verdict: StatusResolved}, true},
		{"问题不能已采纳", issue, CloseRequest{Verdict: StatusAdopted}, false},
		{"问题不能发积分", issue, CloseRequest{Verdict: StatusResolved, RewardPoints: 500}, false},
		{"建议已采纳可发积分", idea, CloseRequest{Verdict: StatusAdopted, RewardPoints: 500}, true},
		{"建议不能已解决", idea, CloseRequest{Verdict: StatusResolved}, false},
		{"建议不处理不能发积分", idea, CloseRequest{Verdict: StatusDeclined, Reason: "x", RewardPoints: 100}, false},
		{"不处理必须填原因", idea, CloseRequest{Verdict: StatusDeclined}, false},
		{"重复已下线", issue, CloseRequest{Verdict: StatusDuplicate}, false},
		{"积分不能为负", idea, CloseRequest{Verdict: StatusAdopted, RewardPoints: -1}, false},
	}
	for _, c := range cases {
		err := ValidateClose(c.f, c.req)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	if err := ValidateClose(closed, CloseRequest{Verdict: StatusAdopted}); !errors.Is(err, ErrClosed) {
		t.Errorf("已结案应返回 ErrClosed，got %v", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	if s, _ := statusAfterUserReply(StatusAwaiting); s != StatusPending {
		t.Errorf("待补充的用户回复后应回到待处理，got %s", s)
	}
	if _, err := statusAfterUserReply(StatusResolved); !errors.Is(err, ErrClosed) {
		t.Error("已结案不能补充")
	}
	if s, _ := statusAfterAdminMessage(StatusPending, MsgReply); s != StatusAwaiting {
		t.Errorf("回复后应为已回复，got %s", s)
	}
	if _, err := statusAfterAdminMessage(StatusPending, MsgAsk); err == nil {
		t.Error("追问已并入回复")
	}
	if _, err := statusAfterAdminMessage(StatusResolved, MsgReply); !errors.Is(err, ErrClosed) {
		t.Error("已结案不能回复")
	}
	if s, _ := statusAfterAdminMessage(StatusAdopted, MsgNote); s != StatusAdopted {
		t.Errorf("备注不改状态，got %s", s)
	}
	if _, err := statusAfterAdminMessage(StatusPending, MsgSupplement); err == nil {
		t.Error("管理员不能发用户补充")
	}
}

func TestDefaultConfigValid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("默认配置应合法: %v", err)
	}
	kinds := map[Kind]int{}
	for _, s := range cfg.Scenarios {
		kinds[s.Kind]++
	}
	if kinds[KindIssue] == 0 || kinds[KindSuggestion] == 0 {
		t.Fatal("问题和建议都要有场景")
	}
	if cfg.Reward.Default != 500 || cfg.Reward.Step != 100 {
		t.Fatal("积分默认 500，步长 100")
	}
}

func TestConfigValidate(t *testing.T) {
	bad := func(mut func(c *Config)) error {
		c := DefaultConfig()
		mut(&c)
		return c.Validate()
	}
	if bad(func(c *Config) { c.Scenarios[1].Key = c.Scenarios[0].Key }) == nil {
		t.Error("场景 key 重复应报错")
	}
	if bad(func(c *Config) { c.Scenarios[0].Key = "Bad-Key" }) == nil {
		t.Error("场景 key 格式错误应报错")
	}
	if bad(func(c *Config) {
		for i := range c.Scenarios {
			if c.Scenarios[i].Kind == KindSuggestion {
				c.Scenarios[i].Enabled = false
			}
		}
	}) == nil {
		t.Error("建议没有启用场景时应报错")
	}
	if bad(func(c *Config) { c.Scenarios[0].Fields = []Field{{Key: "x", Label: "X", Type: FieldSelect}} }) == nil {
		t.Error("下拉字段没有选项应报错")
	}
	if bad(func(c *Config) { c.Reward.Step = 0 }) == nil {
		t.Error("步长为 0 应报错")
	}
}

func TestScenarioLookupAndExtra(t *testing.T) {
	cfg := DefaultConfig()
	if _, ok := cfg.UsableScenario(KindSuggestion, "call_error"); ok {
		t.Error("问题场景不能用于建议")
	}
	sc, ok := cfg.UsableScenario(KindIssue, "access")
	if !ok {
		t.Fatal("access 应可用")
	}
	got := sc.CleanExtra(map[string]string{"client": "Cursor", "unknown": "x"})
	if got["client"] != "Cursor" || len(got) != 1 {
		t.Errorf("只保留定义过的字段: %v", got)
	}
	if got := sc.CleanExtra(map[string]string{"client": "不在选项里"}); len(got) != 0 {
		t.Errorf("下拉字段的非法值应丢弃: %v", got)
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" a ", "a", "", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v", got)
	}
}

func TestReencodeRejectsNonImage(t *testing.T) {
	if _, _, _, _, _, err := reencode([]byte("<svg onload=alert(1)>")); err == nil {
		t.Error("非图片应拒绝")
	}
}
