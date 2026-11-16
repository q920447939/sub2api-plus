// Package feedback — [plus] 类型策略与状态机
//
// 问题和建议的差异全部收敛在 KindPolicy 里：可选结论、是否可发积分。
// 新增类型只需在 policies 里加一项，状态机和服务层不用改。
package feedback

// KindPolicy 某个反馈类型的规则
type KindPolicy struct {
	Kind     Kind
	Verdicts []Status // 可选结论（终态）
	Rewarded Status   // 该结论下可发积分；空 = 这个类型不发积分
}

var policies = map[Kind]KindPolicy{
	KindIssue: {
		Kind:     KindIssue,
		Verdicts: []Status{StatusResolved, StatusDeclined},
	},
	KindSuggestion: {
		Kind:     KindSuggestion,
		Verdicts: []Status{StatusAdopted, StatusDeclined},
		Rewarded: StatusAdopted,
	},
}

// PolicyOf 返回类型规则；未知类型按问题处理（最保守：不发积分）
func PolicyOf(k Kind) KindPolicy {
	if p, ok := policies[k]; ok {
		return p
	}
	return policies[KindIssue]
}

// Allows 该类型能否以 verdict 结案
func (p KindPolicy) Allows(verdict Status) bool {
	for _, v := range p.Verdicts {
		if v == verdict {
			return true
		}
	}
	return false
}

// Rewardable 以 verdict 结案时能否发积分
func (p KindPolicy) Rewardable(verdict Status) bool {
	return p.Rewarded != "" && p.Rewarded == verdict
}

// CloseRequest 结案参数
type CloseRequest struct {
	Verdict      Status `json:"verdict"`
	Reason       string `json:"reason"`        // 不处理时必填
	RewardPoints int64  `json:"reward_points"` // 0 = 不发
	Message      string `json:"message"`       // 给用户的一句话（选填）
}

// ValidateClose 校验结案参数与当前反馈是否匹配（纯函数，不碰数据库）
func ValidateClose(f *Feedback, req CloseRequest) error {
	if !f.Status.Open() {
		return ErrClosed
	}
	pol := PolicyOf(f.Kind)
	if !pol.Allows(req.Verdict) {
		return inputErr("这个类型不能选这个结论")
	}
	if req.Verdict == StatusDeclined && req.Reason == "" {
		return inputErr("请填写不处理的原因")
	}
	if req.RewardPoints < 0 {
		return inputErr("积分不能为负")
	}
	if req.RewardPoints > 0 && !pol.Rewardable(req.Verdict) {
		return inputErr("只有已采纳的建议可以发积分")
	}
	return nil
}

// statusAfterUserReply 用户补充后的状态：待补充 → 待处理；待处理保持
func statusAfterUserReply(s Status) (Status, error) {
	if !s.Open() {
		return s, ErrClosed
	}
	return StatusPending, nil
}

// statusAfterAdminMessage 管理员发消息后的状态：回复 → 已回复（等用户），备注不变
func statusAfterAdminMessage(s Status, kind MsgKind) (Status, error) {
	switch kind {
	case MsgNote:
		return s, nil
	case MsgReply:
		if !s.Open() {
			return s, ErrClosed
		}
		return StatusAwaiting, nil
	}
	return s, inputErr("不支持的消息类型")
}

// userVisible 管理员消息里哪些会让用户端出现红点
func userVisible(kind MsgKind) bool { return kind == MsgReply }
