package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AccountUsageHandler 提供账号维度的对账统计（二开扩展）。
type AccountUsageHandler struct {
	usageService   *service.UsageService
	settingService *service.SettingService
}

func NewAccountUsageHandler(usageService *service.UsageService, settingService *service.SettingService) *AccountUsageHandler {
	return &AccountUsageHandler{usageService: usageService, settingService: settingService}
}

// GET /admin/account-usage/summary
func (h *AccountUsageHandler) GetSummary(c *gin.Context) {
	startTime, endTime, ok := parseAccountUsageDateRange(c)
	if !ok {
		return
	}

	limit := 50
	if v, err := strconv.Atoi(strings.TrimSpace(c.Query("limit"))); err == nil && v > 0 {
		limit = v
	}

	// 读不到全局设置时按上游默认值（开启）处理，只影响提示文案，不影响金额。
	globalProbeEnabled := true
	if h.settingService != nil {
		if settings, err := h.settingService.GetUpstreamBillingProbeSettings(c.Request.Context()); err == nil && settings != nil {
			globalProbeEnabled = settings.Enabled
		}
	}

	result, err := h.usageService.GetAccountSummaryList(c.Request.Context(), startTime, endTime, limit, globalProbeEnabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GET /admin/account-usage/trend
func (h *AccountUsageHandler) GetTrend(c *gin.Context) {
	startTime, endTime, ok := parseAccountUsageDateRange(c)
	if !ok {
		return
	}

	granularity := strings.TrimSpace(c.Query("granularity"))
	if granularity == "" {
		granularity = "day"
	}

	limit := 10
	if v, err := strconv.Atoi(strings.TrimSpace(c.Query("limit"))); err == nil && v > 0 {
		limit = v
	}

	results, err := h.usageService.GetAccountUsageTrend(c.Request.Context(), startTime, endTime, granularity, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, results)
}

// GET /admin/account-usage/model-stats
func (h *AccountUsageHandler) GetModelStats(c *gin.Context) {
	accountID, err := strconv.ParseInt(strings.TrimSpace(c.Query("account_id")), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account_id")
		return
	}

	startTime, endTime, ok := parseAccountUsageDateRange(c)
	if !ok {
		return
	}

	stats, err := h.usageService.GetAccountModelStats(c.Request.Context(), accountID, startTime, endTime)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

func parseAccountUsageDateRange(c *gin.Context) (startTime, endTime time.Time, ok bool) {
	loc := time.UTC
	if tz := strings.TrimSpace(c.Query("timezone")); tz != "" {
		parsed, err := time.LoadLocation(tz)
		if err != nil {
			response.BadRequest(c, "Invalid timezone")
			return time.Time{}, time.Time{}, false
		}
		loc = parsed
	}

	startDateStr := strings.TrimSpace(c.Query("start_date"))
	endDateStr := strings.TrimSpace(c.Query("end_date"))
	if startDateStr == "" || endDateStr == "" {
		response.BadRequest(c, "start_date and end_date are required")
		return time.Time{}, time.Time{}, false
	}

	startTime, err := time.ParseInLocation("2006-01-02", startDateStr, loc)
	if err != nil {
		response.BadRequest(c, "Invalid start_date format, expected YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	endTime, err = time.ParseInLocation("2006-01-02", endDateStr, loc)
	if err != nil {
		response.BadRequest(c, "Invalid end_date format, expected YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	// end_date 是包含当天的，查询上界取次日零点
	return startTime, endTime.AddDate(0, 0, 1), true
}
