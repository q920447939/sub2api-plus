package usagestats

import "time"

// 账号对账统计（二开扩展）的费用口径：
//   - StandardCost: 标准（原价），usage_logs.total_cost，不含任何倍率
//   - ActualCost:   实际（用户扣费），usage_logs.actual_cost，受本站分组倍率影响
//   - UpstreamCost: 成本（上游扣费），原价 × 账号当前的上游声明倍率（不含高峰）；
//     账号没有上游声明倍率时退回原价 × 账号倍率快照，这部分同时计入 EstimatedCost

// 账号成本所用倍率的来源。
const (
	AccountRateSourceUpstream      = "upstream"                // 上游声明倍率，探测结果新鲜
	AccountRateSourceUpstreamStale = "upstream_stale"          // 上游声明倍率，但探测已过期/失败，沿用最后一次结果
	AccountRateSourceProbeOff      = "estimated_probe_off"     // 未开启上游倍率探测，按账号倍率估算
	AccountRateSourceProbePending  = "estimated_probe_pending" // 已开启探测但从未成功，按账号倍率估算
	AccountRateSourceUnsupported   = "estimated_unsupported"   // 账号类型或上游不支持探测，按账号倍率估算
)

type AccountUsageTrendPoint struct {
	Date         string  `json:"date"`
	AccountID    int64   `json:"account_id"`
	AccountName  string  `json:"account_name"`
	Platform     string  `json:"platform"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	StandardCost float64 `json:"standard_cost"`
	ActualCost   float64 `json:"actual_cost"`
	UpstreamCost float64 `json:"upstream_cost"`
}

type AccountSummaryItem struct {
	AccountID     int64   `json:"account_id"`
	AccountName   string  `json:"account_name"`
	Platform      string  `json:"platform"`
	AccountType   string  `json:"account_type"`
	Requests      int64   `json:"requests"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	StandardCost  float64 `json:"standard_cost"`
	ActualCost    float64 `json:"actual_cost"`
	UpstreamCost  float64 `json:"upstream_cost"`
	EstimatedCost float64 `json:"estimated_cost"`

	RateSource      string     `json:"rate_source"`
	UpstreamRate    *float64   `json:"upstream_rate"`
	AccountRate     *float64   `json:"account_rate"`
	RateSyncEnabled bool       `json:"rate_sync_enabled"`
	RateMismatch    bool       `json:"rate_mismatch"`
	ProbeReceivedAt *time.Time `json:"probe_received_at"`
	ProbeLastError  string     `json:"probe_last_error,omitempty"`

	// 以下字段仅供 service 层推导 RateSource，不对外输出。
	ProbeEnabled    bool       `json:"-"`
	ProbeStatus     string     `json:"-"`
	ProbeFreshUntil *time.Time `json:"-"`
}

type AccountSummaryResponse struct {
	Items              []AccountSummaryItem `json:"items"`
	TotalStandardCost  float64              `json:"total_standard_cost"`
	TotalActualCost    float64              `json:"total_actual_cost"`
	TotalUpstreamCost  float64              `json:"total_upstream_cost"`
	TotalEstimatedCost float64              `json:"total_estimated_cost"`
	TotalRequests      int64                `json:"total_requests"`
	TotalTokens        int64                `json:"total_tokens"`
}

type AccountModelStat struct {
	Model               string  `json:"model"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	StandardCost        float64 `json:"standard_cost"`
	ActualCost          float64 `json:"actual_cost"`
	UpstreamCost        float64 `json:"upstream_cost"`
}
