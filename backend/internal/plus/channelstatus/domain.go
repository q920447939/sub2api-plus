// Package channelstatus — [plus] 渠道状态：平台 Tab → 分组行，被动统计为主，可选兜底探测。
// 本文件只放纯领域类型，不依赖任何基础设施。
package channelstatus

import "time"

// Status 用户端看到的状态
type Status string

const (
	StatusUp      Status = "up"      // 正常
	StatusWarn    Status = "warn"    // 降级
	StatusDown    Status = "down"    // 异常
	StatusUnknown Status = "unknown" // 暂无数据（没有新鲜证据，绝不显示绿色）
)

// Granularity 历史条粒度
type Granularity string

const (
	GranHour Granularity = "hour"
	GranDay  Granularity = "day"
	GranWeek Granularity = "week"
)

// GranSpec 粒度对应的单格时长与格数
type GranSpec struct {
	Bucket time.Duration
	Count  int
}

var granSpecs = map[Granularity]GranSpec{
	GranHour: {Bucket: time.Hour, Count: 48},
	GranDay:  {Bucket: 24 * time.Hour, Count: 30},
	GranWeek: {Bucket: 7 * 24 * time.Hour, Count: 12},
}

func ParseGranularity(s string) (Granularity, bool) {
	g := Granularity(s)
	_, ok := granSpecs[g]
	return g, ok
}

func (g Granularity) Spec() GranSpec { return granSpecs[g] }

// Bucket 历史条中的一格
type Bucket struct {
	Start  time.Time `json:"t"`
	Score  *float64  `json:"score"`
	Status Status    `json:"status"`
}

// GroupView 用户端一行（一个分组）
type GroupView struct {
	GroupID  int64    `json:"group_id"`
	Name     string   `json:"name"`
	Subtitle string   `json:"subtitle"`
	Status   Status   `json:"status"`
	Score    *float64 `json:"score"`
	History  []Bucket `json:"history"`
}

// PlatformView 用户端一个平台 Tab
type PlatformView struct {
	Platform string      `json:"platform"`
	Name     string      `json:"name"`
	Groups   []GroupView `json:"groups"`
}

// Snapshot 用户端接口的完整响应
type Snapshot struct {
	Granularity    Granularity    `json:"granularity"`
	RefreshSeconds int            `json:"refresh_seconds"`
	Platforms      []PlatformView `json:"platforms"`
}
