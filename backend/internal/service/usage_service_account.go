package service

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// accountUsageAnalyticsRepo 是二开的账号维度对账统计扩展。
// 不并入上游 UsageLogRepository 接口，避免合并冲突和破坏上游测试 mock；
// 运行时通过类型断言从 usageRepo 取得实现。
type accountUsageAnalyticsRepo interface {
	GetAccountUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.AccountUsageTrendPoint, error)
	GetAccountSummaryList(ctx context.Context, startTime, endTime time.Time, limit int) (*usagestats.AccountSummaryResponse, error)
	GetAccountModelStats(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]usagestats.AccountModelStat, error)
}

var errAccountUsageAnalyticsUnsupported = errors.New("account usage analytics not supported by usage repository")

// 上游声明倍率与账号倍率的比较容差，与 accounts.rate_multiplier 的 4 位小数精度一致。
const accountUsageRateMismatchTolerance = 0.00005

func (s *UsageService) accountUsageRepo() (accountUsageAnalyticsRepo, error) {
	repo, ok := s.usageRepo.(accountUsageAnalyticsRepo)
	if !ok {
		return nil, errAccountUsageAnalyticsUnsupported
	}
	return repo, nil
}

// GetAccountUsageTrend returns per-account usage trend for the top N accounts by upstream cost.
func (s *UsageService) GetAccountUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]usagestats.AccountUsageTrendPoint, error) {
	repo, err := s.accountUsageRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAccountUsageTrend(ctx, startTime, endTime, granularity, limit)
}

// GetAccountSummaryList returns aggregated usage stats per account, annotated with
// where each account's cost multiplier came from. globalProbeEnabled is the
// site-wide upstream billing probe switch.
func (s *UsageService) GetAccountSummaryList(ctx context.Context, startTime, endTime time.Time, limit int, globalProbeEnabled bool) (*usagestats.AccountSummaryResponse, error) {
	repo, err := s.accountUsageRepo()
	if err != nil {
		return nil, err
	}
	resp, err := repo.GetAccountSummaryList(ctx, startTime, endTime, limit)
	if err != nil || resp == nil {
		return resp, err
	}
	now := time.Now()
	for i := range resp.Items {
		annotateAccountRateSource(&resp.Items[i], globalProbeEnabled, now)
	}
	return resp, nil
}

// GetAccountModelStats returns per-model usage for one account.
func (s *UsageService) GetAccountModelStats(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]usagestats.AccountModelStat, error) {
	repo, err := s.accountUsageRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAccountModelStats(ctx, accountID, startTime, endTime)
}

func annotateAccountRateSource(item *usagestats.AccountSummaryItem, globalProbeEnabled bool, now time.Time) {
	if item.UpstreamRate != nil {
		fresh := item.ProbeStatus == UpstreamBillingProbeStatusOK &&
			item.ProbeFreshUntil != nil && now.Before(*item.ProbeFreshUntil)
		if fresh {
			item.RateSource = usagestats.AccountRateSourceUpstream
		} else {
			item.RateSource = usagestats.AccountRateSourceUpstreamStale
		}
		accountRate := 1.0
		if item.AccountRate != nil {
			accountRate = *item.AccountRate
		}
		item.RateMismatch = math.Abs(*item.UpstreamRate-accountRate) > accountUsageRateMismatchTolerance
		return
	}

	switch {
	case !IsUpstreamBillingProbeIdentity(item.Platform, item.AccountType),
		item.ProbeStatus == UpstreamBillingProbeStatusUnsupported:
		item.RateSource = usagestats.AccountRateSourceUnsupported
	case !globalProbeEnabled || !item.ProbeEnabled:
		item.RateSource = usagestats.AccountRateSourceProbeOff
	default:
		item.RateSource = usagestats.AccountRateSourceProbePending
	}
}
