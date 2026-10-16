package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type AccountUsageTrendPoint = usagestats.AccountUsageTrendPoint
type AccountSummaryItem = usagestats.AccountSummaryItem
type AccountSummaryResponse = usagestats.AccountSummaryResponse
type AccountModelStat = usagestats.AccountModelStat

// accountUsageRatesCTE 取每个账号当前的上游声明倍率（不含高峰的基准倍率）。
// 来源是上游计费探测写在 accounts.extra 里的快照；探测失败时快照保留最后一次成功的数据。
// 嵌套 CASE 保证先判断 JSON 类型再做 numeric 转换，避免脏数据导致整条查询报错。
const accountUsageRatesCTE = `
	account_rates AS (
		SELECT
			a.id AS account_id,
			CASE WHEN jsonb_typeof(a.extra #> '{upstream_billing_probe,data,resolved_rate_multiplier}') = 'number' THEN
				CASE WHEN (a.extra #>> '{upstream_billing_probe,data,resolved_rate_multiplier}')::numeric >= 0
					THEN (a.extra #>> '{upstream_billing_probe,data,resolved_rate_multiplier}')::numeric
				END
			END AS upstream_rate
		FROM accounts a
	)`

// 成本（上游扣费）：有上游声明倍率时按 原价 × 当前上游倍率；
// 否则退回系统原有的账号成本口径（原价 × 账号倍率快照），这部分同时计入估算金额。
const (
	accountUsageFallbackCostExpr  = `COALESCE(u.account_stats_cost, u.total_cost) * COALESCE(u.account_rate_multiplier, 1)`
	accountUsageUpstreamCostExpr  = `CASE WHEN ar.upstream_rate IS NOT NULL THEN COALESCE(u.account_stats_cost, u.total_cost) * ar.upstream_rate ELSE ` + accountUsageFallbackCostExpr + ` END`
	accountUsageEstimatedCostExpr = `CASE WHEN ar.upstream_rate IS NULL THEN ` + accountUsageFallbackCostExpr + ` ELSE 0 END`
)

func (r *usageLogRepository) GetAccountUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) (results []AccountUsageTrendPoint, err error) {
	dateFormat := safeDateFormat(granularity)

	query := fmt.Sprintf(`
		WITH %s,
		top_accounts AS (
			SELECT u.account_id
			FROM usage_logs u
			LEFT JOIN account_rates ar ON ar.account_id = u.account_id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.account_id
			ORDER BY SUM(%s) DESC, u.account_id ASC
			LIMIT $3
		)
		SELECT
			TO_CHAR(u.created_at, '%s') as date,
			u.account_id,
			COALESCE(a.name, '') as account_name,
			COALESCE(a.platform, '') as platform,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens,
			COALESCE(SUM(u.total_cost), 0) as standard_cost,
			COALESCE(SUM(u.actual_cost), 0) as actual_cost,
			COALESCE(SUM(%s), 0) as upstream_cost
		FROM usage_logs u
		LEFT JOIN accounts a ON u.account_id = a.id
		LEFT JOIN account_rates ar ON ar.account_id = u.account_id
		WHERE u.account_id IN (SELECT account_id FROM top_accounts)
		  AND u.created_at >= $4 AND u.created_at < $5
		GROUP BY date, u.account_id, a.name, a.platform
		ORDER BY date ASC, upstream_cost DESC
	`, accountUsageRatesCTE, accountUsageUpstreamCostExpr, dateFormat, accountUsageUpstreamCostExpr)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]AccountUsageTrendPoint, 0)
	for rows.Next() {
		var row AccountUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.AccountID, &row.AccountName, &row.Platform, &row.Requests, &row.Tokens, &row.StandardCost, &row.ActualCost, &row.UpstreamCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *usageLogRepository) GetAccountSummaryList(ctx context.Context, startTime, endTime time.Time, limit int) (result *AccountSummaryResponse, err error) {
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		WITH %s,
		account_spend AS (
			SELECT
				u.account_id,
				COUNT(*) as requests,
				COALESCE(SUM(u.input_tokens), 0) as input_tokens,
				COALESCE(SUM(u.output_tokens), 0) as output_tokens,
				COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as total_tokens,
				COALESCE(SUM(u.total_cost), 0) as standard_cost,
				COALESCE(SUM(u.actual_cost), 0) as actual_cost,
				COALESCE(SUM(%s), 0) as upstream_cost,
				COALESCE(SUM(%s), 0) as estimated_cost
			FROM usage_logs u
			LEFT JOIN account_rates ar ON ar.account_id = u.account_id
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.account_id
		),
		ranked AS (
			SELECT
				s.*,
				SUM(s.standard_cost) OVER () as total_standard_cost,
				SUM(s.actual_cost) OVER () as total_actual_cost,
				SUM(s.upstream_cost) OVER () as total_upstream_cost,
				SUM(s.estimated_cost) OVER () as total_estimated_cost,
				SUM(s.requests) OVER () as total_requests,
				SUM(s.total_tokens) OVER () as total_tokens_all
			FROM account_spend s
			ORDER BY s.upstream_cost DESC, s.total_tokens DESC, s.account_id ASC
			LIMIT $3
		)
		SELECT
			rk.account_id,
			COALESCE(a.name, ''),
			COALESCE(a.platform, ''),
			COALESCE(a.type, ''),
			rk.requests, rk.input_tokens, rk.output_tokens, rk.total_tokens,
			rk.standard_cost, rk.actual_cost, rk.upstream_cost, rk.estimated_cost,
			ar.upstream_rate::float8,
			a.rate_multiplier::float8,
			COALESCE(a.extra->>'upstream_billing_probe_enabled', '') = 'true',
			COALESCE(a.extra->>'upstream_billing_rate_sync_enabled', '') = 'true',
			COALESCE(a.extra #>> '{upstream_billing_probe,status}', ''),
			COALESCE(a.extra #>> '{upstream_billing_probe,received_at}', ''),
			COALESCE(a.extra #>> '{upstream_billing_probe,fresh_until}', ''),
			COALESCE(a.extra #>> '{upstream_billing_probe,last_error}', ''),
			rk.total_standard_cost, rk.total_actual_cost, rk.total_upstream_cost, rk.total_estimated_cost,
			rk.total_requests, rk.total_tokens_all
		FROM ranked rk
		LEFT JOIN accounts a ON a.id = rk.account_id
		LEFT JOIN account_rates ar ON ar.account_id = rk.account_id
		ORDER BY rk.upstream_cost DESC, rk.total_tokens DESC, rk.account_id ASC
	`, accountUsageRatesCTE, accountUsageUpstreamCostExpr, accountUsageEstimatedCostExpr)

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	resp := &AccountSummaryResponse{Items: make([]AccountSummaryItem, 0)}
	for rows.Next() {
		var row AccountSummaryItem
		var upstreamRate, accountRate *float64
		var receivedAt, freshUntil string
		if err = rows.Scan(
			&row.AccountID, &row.AccountName, &row.Platform, &row.AccountType,
			&row.Requests, &row.InputTokens, &row.OutputTokens, &row.TotalTokens,
			&row.StandardCost, &row.ActualCost, &row.UpstreamCost, &row.EstimatedCost,
			&upstreamRate, &accountRate,
			&row.ProbeEnabled, &row.RateSyncEnabled,
			&row.ProbeStatus, &receivedAt, &freshUntil, &row.ProbeLastError,
			&resp.TotalStandardCost, &resp.TotalActualCost, &resp.TotalUpstreamCost, &resp.TotalEstimatedCost,
			&resp.TotalRequests, &resp.TotalTokens,
		); err != nil {
			return nil, err
		}
		row.UpstreamRate = upstreamRate
		row.AccountRate = accountRate
		row.ProbeReceivedAt = parseAccountUsageProbeTime(receivedAt)
		row.ProbeFreshUntil = parseAccountUsageProbeTime(freshUntil)
		resp.Items = append(resp.Items, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetAccountModelStats returns per-model usage of one account with the reconciliation cost columns.
func (r *usageLogRepository) GetAccountModelStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (results []AccountModelStat, err error) {
	modelExpr := resolveModelDimensionExpressionWithAlias(usagestats.ModelSourceRequested, "u")

	query := fmt.Sprintf(`
		WITH %s
		SELECT
			%s as model,
			COUNT(*) as requests,
			COALESCE(SUM(u.input_tokens), 0) as input_tokens,
			COALESCE(SUM(u.output_tokens), 0) as output_tokens,
			COALESCE(SUM(u.cache_creation_tokens), 0) as cache_creation_tokens,
			COALESCE(SUM(u.cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as total_tokens,
			COALESCE(SUM(u.total_cost), 0) as standard_cost,
			COALESCE(SUM(u.actual_cost), 0) as actual_cost,
			COALESCE(SUM(%s), 0) as upstream_cost
		FROM usage_logs u
		LEFT JOIN account_rates ar ON ar.account_id = u.account_id
		WHERE u.account_id = $1 AND u.created_at >= $2 AND u.created_at < $3
		GROUP BY %s
		ORDER BY upstream_cost DESC, total_tokens DESC
	`, accountUsageRatesCTE, modelExpr, accountUsageUpstreamCostExpr, modelExpr)

	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]AccountModelStat, 0)
	for rows.Next() {
		var row AccountModelStat
		if err = rows.Scan(
			&row.Model, &row.Requests,
			&row.InputTokens, &row.OutputTokens, &row.CacheCreationTokens, &row.CacheReadTokens, &row.TotalTokens,
			&row.StandardCost, &row.ActualCost, &row.UpstreamCost,
		); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func parseAccountUsageProbeTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil
	}
	return &t
}
