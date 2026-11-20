// Package channelstatus — [plus] 默认评分策略：错误率 + TTFT P50 加权平均；缺失的维度按权重重新归一化。
// 缓存命中率不参与：它取决于用户的使用方式（是否复用上下文等），不反映渠道是否可用。
package channelstatus

// DefaultScorer 是 Scorer 的默认实现，无状态，可并发使用
type DefaultScorer struct{}

func (DefaultScorer) Score(s Signal, r Rules) *float64 {
	if s.Requests < r.MinSample || s.Requests <= 0 {
		return nil
	}
	var weightSum, scoreSum float64
	add := func(score, weight float64) {
		weightSum += weight
		scoreSum += weight * score
	}

	errRate := float64(s.Errors) / float64(s.Requests)
	add(errorRateScore(errRate, r.CriticalErrorRate), r.ErrorWeight)

	if s.TTFTP50Ms != nil && s.TTFTSamples >= r.MinSample {
		add(ttftScore(float64(*s.TTFTP50Ms), float64(r.TargetTTFTMs), float64(r.CriticalTTFTMs)), r.TTFTWeight)
	}
	if weightSum <= 0 {
		return nil
	}
	v := scoreSum / weightSum
	return &v
}

// errorRateScore：0% 得 100，达到严重阈值得 0，中间线性
func errorRateScore(rate, critical float64) float64 {
	if critical <= 0 {
		critical = 0.05
	}
	if rate <= 0 {
		return 100
	}
	if rate >= critical {
		return 0
	}
	return 100 * (1 - rate/critical)
}

// ttftScore：不高于目标得 100，不低于严重阈值得 0，中间线性
func ttftScore(p50, target, critical float64) float64 {
	if p50 <= target {
		return 100
	}
	if p50 >= critical {
		return 0
	}
	return 100 * (1 - (p50-target)/(critical-target))
}
