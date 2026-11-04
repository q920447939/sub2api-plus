// Package channelstatus — [plus] 时间轴：按粒度计算历史条的对齐区间（站点时区）
package channelstatus

import "time"

// Timeline 历史条区间；桶 i 的起点 = Start + i*Bucket，最后一格是当前未结束的周期
type Timeline struct {
	Start  time.Time
	End    time.Time
	Bucket time.Duration
	Count  int
}

// BuildTimeline 小时按整点、天按自然日
func BuildTimeline(g Granularity, now time.Time, loc *time.Location) Timeline {
	spec := g.Spec()
	t := now.In(loc)
	var cur time.Time
	switch g {
	case GranDay:
		cur = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	case GranWeek:
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		offset := (int(d.Weekday()) + 6) % 7 // 周一=0
		cur = d.AddDate(0, 0, -offset)
	default:
		cur = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
	}
	start := cur.Add(-time.Duration(spec.Count-1) * spec.Bucket)
	return Timeline{Start: start, End: cur.Add(spec.Bucket), Bucket: spec.Bucket, Count: spec.Count}
}

// BucketStart 第 i 格起点
func (tl Timeline) BucketStart(i int) time.Time {
	return tl.Start.Add(time.Duration(i) * tl.Bucket)
}
