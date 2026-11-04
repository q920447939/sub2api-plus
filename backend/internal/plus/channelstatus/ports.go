// Package channelstatus — [plus] 端口接口：service 只依赖这些接口，便于替换实现和单测 mock。
package channelstatus

import (
	"context"
	"time"
)

// Signal 某分组在某个时间窗口内的观测信号（Errors 已排除被忽略的错误分类）
type Signal struct {
	Requests    int64
	Errors      int64
	TTFTP50Ms   *int64
	TTFTSamples int64
	CacheRate   float64
	CacheDenom  int64
}

// SignalSource 信号来源（被动统计、探测结果等都实现它，可用责任链组合）。
// 返回 map[groupID]map[桶起点 unix 秒]Signal；没有数据的桶不出现在 map 中。
type SignalSource interface {
	Fetch(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) (map[int64]map[int64]Signal, error)
}

// Scorer 评分策略：样本不足返回 nil（表示没有证据，不是 0 分）
type Scorer interface {
	Score(sig Signal, r Rules) *float64
}

// Classifier 分类策略：分数转状态，nil 分数必须返回 StatusUnknown
type Classifier interface {
	Classify(score *float64, r Rules) Status
}

// Verdict 兜底判定结果
type Verdict struct {
	Status Status
	Score  *float64
}

// FallbackJudge 兜底判定（如按需探测）：只对被动样本不足的分组调用。
// lacking 为 分组 → 判定窗口内的真实请求数（不足最小样本，可能为 0）。
// 调用本身也代表"有人在看"，实现方可据此异步触发探测；拿不到新鲜结论的分组不出现在返回值中。
type FallbackJudge interface {
	Judge(ctx context.Context, cfg Config, lacking map[int64]int64) map[int64]Verdict
}

// GroupInfo sub2api 分组的只读信息
type GroupInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// GroupCatalog 分组目录（只读上游 groups 表，仅返回未删除且启用的分组）
type GroupCatalog interface {
	ListGroups(ctx context.Context) ([]GroupInfo, error)
}

// ConfigStore 配置存储（单行 + 乐观锁）
type ConfigStore interface {
	Load(ctx context.Context) (Config, int64, error)
	Save(ctx context.Context, cfg Config, expectVer int64) (int64, error)
}
