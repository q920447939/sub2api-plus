// Package channelstatus — [plus] 默认分类策略：分数 → 状态
package channelstatus

// ThresholdClassifier 按配置的两条分数线分档；没有分数一律是 unknown，绝不显示绿色
type ThresholdClassifier struct{}

func (ThresholdClassifier) Classify(score *float64, r Rules) Status {
	if score == nil {
		return StatusUnknown
	}
	switch {
	case *score >= r.UpMin:
		return StatusUp
	case *score >= r.WarnMin:
		return StatusWarn
	default:
		return StatusDown
	}
}
