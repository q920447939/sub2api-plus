// Package channelstatus — [plus] 兜底探测的判定与触发策略（纯函数，无 IO，便于单测）。
//
// 规则：
//  1. 探测只在「有人在看 + 被动样本不足」时由 FallbackJudge 触发，不按时钟定时跑；
//  2. 成功结果在保质期内有效，过期后若还不能再探（冷却/上限）就显示「暂无数据」，绝不显示绿色；
//  3. 失败结果持续有效（保持异常），直到新的探测成功或超过冷却上限，避免"挂了却显示灰色"；
//  4. 成功后的冷却 = 冷却起点 × 2^连续无流量次数，封顶冷却上限；窗口内出现任何真实请求，冷却立即回到起点；
//  5. 失败后按较短的复测间隔重试；所有探测受每分组每日上限约束。
package channelstatus

import "time"

// ProbeState 某分组最近一次探测的状态
type ProbeState struct {
	GroupID       int64      `json:"group_id"`
	LastStartedAt *time.Time `json:"last_started_at"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	LastOK        *bool      `json:"last_ok"`
	LatencyMs     *int       `json:"latency_ms"`
	Message       string     `json:"message"`
	IdleStreak    int        `json:"idle_streak"`
	Day           string     `json:"day"` // YYYY-MM-DD
	DayCount      int        `json:"day_count"`
}

// ProbePolicy 根据配置做判定
type ProbePolicy struct {
	P   Probe
	Loc *time.Location
}

func (pp ProbePolicy) fresh() time.Duration { return time.Duration(pp.P.FreshMinutes) * time.Minute }

// Cooldown 成功后的冷却时长；hadTraffic=true 表示窗口内出现过真实请求，冷却回到起点
func (pp ProbePolicy) Cooldown(idleStreak int, hadTraffic bool) time.Duration {
	base := time.Duration(pp.P.BaseCooldownMins) * time.Minute
	max := time.Duration(pp.P.MaxCooldownHours) * time.Hour
	if hadTraffic || idleStreak <= 0 {
		return base
	}
	d := base
	for i := 0; i < idleStreak && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// UsableResult 最近一次探测结果是否还能作为当前状态的依据
func (pp ProbePolicy) UsableResult(st *ProbeState, now time.Time) bool {
	if st == nil || st.LastCheckedAt == nil || st.LastOK == nil {
		return false
	}
	age := now.Sub(*st.LastCheckedAt)
	if *st.LastOK {
		return age < pp.fresh()
	}
	return age < time.Duration(pp.P.MaxCooldownHours)*time.Hour
}

// ShouldProbe 现在是否应该（异步）发起一次探测
func (pp ProbePolicy) ShouldProbe(st *ProbeState, now time.Time, hadTraffic bool, timeout time.Duration) bool {
	if st == nil || st.LastStartedAt == nil {
		return true
	}
	// 进行中（另一个实例或本实例已抢到），超时 2 倍后视为丢失
	if st.LastCheckedAt == nil || st.LastStartedAt.After(*st.LastCheckedAt) {
		if now.Sub(*st.LastStartedAt) < 2*timeout {
			return false
		}
	}
	if pp.DailyExhausted(st, now) {
		return false
	}
	// 成功结果仍在保质期内，不需要探
	if st.LastOK != nil && *st.LastOK && st.LastCheckedAt != nil && now.Sub(*st.LastCheckedAt) < pp.fresh() {
		return false
	}
	gap := time.Duration(pp.P.RetryMinutes) * time.Minute
	if st.LastOK == nil || *st.LastOK {
		gap = pp.Cooldown(st.IdleStreak, hadTraffic)
	}
	return now.Sub(*st.LastStartedAt) >= gap
}

// DailyExhausted 今日探测次数是否已达上限
func (pp ProbePolicy) DailyExhausted(st *ProbeState, now time.Time) bool {
	return st != nil && st.Day == pp.Today(now) && st.DayCount >= pp.P.DailyLimit
}

func (pp ProbePolicy) Today(now time.Time) string {
	loc := pp.Loc
	if loc == nil {
		loc = time.Local
	}
	return now.In(loc).Format("2006-01-02")
}
