package dispatch

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plus/channelstatus"
)

// Health 健康分：和渠道状态同一套算法（失败率 + 首字耗时中位数 + 缓存，加权），按任意范围算
type Health struct {
	Rules channelstatus.Rules
}

var (
	scorer     channelstatus.DefaultScorer
	classifier channelstatus.ThresholdClassifier
)

// Status 正常 / 降级 / 异常 / 暂无数据
type Status = channelstatus.Status

func (h Health) signal(a Agg) channelstatus.Signal {
	s := channelstatus.Signal{Requests: a.Total(), Errors: a.Failed()} // 上游报错和限流都算没接住
	if a.P50N > 0 {
		v := int64(a.TTFTP50)
		s.TTFTP50Ms, s.TTFTSamples = &v, a.P50N
	}
	if a.CacheTotal > 0 {
		s.CacheRate, s.CacheDenom = float64(a.CacheRead)/float64(a.CacheTotal), a.Total()
	}
	return s
}

// Score 样本不够返回 nil
func (h Health) Score(a Agg, minSample int64) *int {
	r := h.Rules
	r.MinSample = minSample
	v := scorer.Score(h.signal(a), r)
	if v == nil {
		return nil
	}
	n := int(*v + 0.5)
	return &n
}

// StatusOf 分数 → 状态
func (h Health) StatusOf(score *int) Status {
	if score == nil {
		return channelstatus.StatusUnknown
	}
	f := float64(*score)
	return classifier.Classify(&f, h.Rules)
}

// Slot 状态条的一格
type Slot struct {
	T       int64    `json:"t"`
	N       int64    `json:"n"`
	Score   *int     `json:"score"`
	Status  Status   `json:"status"`
	ErrPct  *float64 `json:"err_pct,omitempty"`
	TTFT    *float64 `json:"ttft,omitempty"` // 秒
	Adjusts int      `json:"adjusts,omitempty"`
}

// Gran 状态条的粒度
type Gran string

const (
	GranHour Gran = "hour" // 每小时一格
	GranDay  Gran = "day"  // 每天一格
)

// slotStarts 最近 n 格的起点（旧 → 新）
func slotStarts(now time.Time, g Gran, n int) []int64 {
	out := make([]int64, n)
	if g == GranDay {
		y, m, d := now.In(time.Local).Date()
		today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
		for i := 0; i < n; i++ {
			out[i] = today.AddDate(0, 0, i-n+1).Unix()
		}
		return out
	}
	cur := now.Truncate(time.Hour)
	for i := 0; i < n; i++ {
		out[i] = cur.Add(time.Duration(i-n+1) * time.Hour).Unix()
	}
	return out
}

// Timeline 状态条（旧 → 新）
func (h Health) Timeline(t *TrafficSet, f TFilter, g Gran, n int) []Slot {
	starts := slotStarts(t.NowAt, g, n)
	f.From = starts[0]
	sums := make([]Agg, n)
	t.Each(f, func(r *TRow) {
		i := slotIndex(starts, r.Bucket)
		if i >= 0 {
			sums[i] = sums[i].Add(r.Agg)
		}
	})
	min := h.Rules.HistoryMinSample
	if g == GranDay {
		min = h.Rules.MinSample // 一天的量大，用「当前状态」的门槛
	}
	out := make([]Slot, n)
	for i, a := range sums {
		sc := h.Score(a, min)
		s := Slot{T: starts[i], N: a.Total(), Score: sc, Status: h.StatusOf(sc)}
		if a.Total() > 0 {
			e := round1(float64(a.Failed()) * 100 / float64(a.Total()))
			s.ErrPct = &e
		}
		if v, ok := metricVal("ttft", a); ok {
			v = round1(v)
			s.TTFT = &v
		}
		out[i] = s
	}
	return out
}

func slotIndex(starts []int64, b int64) int {
	// starts 递增；返回最后一个 <= b 的下标
	lo, hi := 0, len(starts)-1
	if len(starts) == 0 || b < starts[0] {
		return -1
	}
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= b {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// Current 现在的状态：先看近 1 小时（含当前不满一小时的桶），请求不够就往前拉长，最多 24 小时
type Current struct {
	Hours  int    `json:"hours"`
	Score  *int   `json:"score"`
	Status Status `json:"status"`
	Agg    Agg    `json:"-"`
}

var currentWindows = []int{1, 2, 4, 6, 12, 24}

func (h Health) Current(t *TrafficSet, f TFilter) Current {
	nb := t.NowBucket()
	f.From = nb - 24*3600
	byBucket := map[int64]Agg{}
	t.Each(f, func(r *TRow) { byBucket[r.Bucket] = byBucket[r.Bucket].Add(r.Agg) })
	var a Agg
	hours := 0
	for _, w := range currentWindows {
		a = Agg{}
		for b, x := range byBucket {
			if b > nb-int64(w)*3600 {
				a = a.Add(x)
			}
		}
		hours = w
		if a.Total() >= h.Rules.MinSample {
			break
		}
	}
	sc := h.Score(a, h.Rules.MinSample)
	return Current{Hours: hours, Score: sc, Status: h.StatusOf(sc), Agg: a}
}

func metricVal(key string, a Agg) (float64, bool) {
	for _, m := range metrics {
		if m.Key == key {
			return m.Value(a)
		}
	}
	return 0, false
}

func statusRank(s Status) int {
	switch s {
	case channelstatus.StatusDown:
		return 3
	case channelstatus.StatusWarn:
		return 2
	case channelstatus.StatusUp:
		return 1
	}
	return 0
}
