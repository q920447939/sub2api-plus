// Package outreach — 发送设置：只有「怎么投递」的参数（存 plus_settings，key = outreach）。
//
// 不在这里的东西，以及它们归谁：
//   - 效果统计口径（回访 / 付费窗口）：指标定义的一部分，由目的声明（metric.go / purpose.go）
//   - 权益领取有效期：活动的属性，随批次冻结（Campaign.ClaimDays）
//   - 验证奖励、验证链接有效期：邮箱验证模块自己的设置
package outreach

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/plus/plusconfig"
)

// MinBatchRetentionDays 批次保留期下限
const MinBatchRetentionDays = 30

// DeliveryConfig 投递规则，全部可在后台修改，不需要重启
type DeliveryConfig struct {
	// 触达邮件里链接（点击跟踪 / 领取 / 退订）使用的域名；留空回落到系统设置的「站点地址」
	LinkBaseURL string `json:"link_base_url"`

	// 发送限速（按 SMTP 服务商额度设置）。超出后收件人留在队列，额度恢复后自动续发。
	PerMinute int `json:"per_minute"`
	PerDay    int `json:"per_day"` // 滚动 24 小时

	// 频控
	MarketingGapDays int            `json:"marketing_gap_days"` // 任意两封营销邮件之间
	PurposeGapDays   map[string]int `json:"purpose_gap_days"`   // 同目的两封之间；缺省值由目的声明

	// 熔断：单批次硬退信率超过阈值自动暂停（样本不足时不判断）
	BreakerPercent    int `json:"breaker_percent"`
	BreakerMinSamples int `json:"breaker_min_samples"`

	// 已发过邮件的批次被删除后至少保留多少天才物理清理
	BatchRetentionDays int `json:"batch_retention_days"`
}

func DefaultDeliveryConfig() DeliveryConfig {
	return DeliveryConfig{
		PerMinute: 20, PerDay: 250,
		MarketingGapDays: 7, PurposeGapDays: map[string]int{},
		BreakerPercent: 5, BreakerMinSamples: 20,
		BatchRetentionDays: MinBatchRetentionDays,
	}
}

// NormalizeBaseURL 去掉首尾空白与结尾斜杠；非空时必须是 http(s) 地址
func NormalizeBaseURL(raw string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("触达链接域名必须是 http(s):// 开头的地址，例如 https://news.example.com")
	}
	return s, nil
}

// Validate 校验并规整
func (c *DeliveryConfig) Validate() error {
	base, err := NormalizeBaseURL(c.LinkBaseURL)
	if err != nil {
		return err
	}
	c.LinkBaseURL = base
	if c.PurposeGapDays == nil {
		c.PurposeGapDays = map[string]int{}
	}
	switch {
	case c.PerMinute < 1 || c.PerMinute > 600:
		return errors.New("每分钟上限需在 1 到 600 之间")
	case c.PerDay < 1 || c.PerDay > 1_000_000:
		return errors.New("每日上限需在 1 到 1000000 之间")
	case c.MarketingGapDays < 0 || c.MarketingGapDays > 365:
		return errors.New("营销邮件间隔需在 0 到 365 天之间")
	case c.BreakerPercent < 1 || c.BreakerPercent > 100:
		return errors.New("熔断退信率需在 1 到 100 之间")
	case c.BreakerMinSamples < 1 || c.BreakerMinSamples > 10000:
		return errors.New("熔断最小样本数需在 1 到 10000 之间")
	case c.BatchRetentionDays < MinBatchRetentionDays || c.BatchRetentionDays > 3650:
		return fmt.Errorf("批次保留天数需在 %d 到 3650 之间", MinBatchRetentionDays)
	}
	for _, d := range c.PurposeGapDays {
		if d < 0 || d > 365 {
			return errors.New("同目的邮件间隔需在 0 到 365 天之间")
		}
	}
	return nil
}

// SettingsStore 发送设置存储
type SettingsStore = plusconfig.Store[DeliveryConfig]

func NewSettingsStore(db *sql.DB) *SettingsStore {
	return plusconfig.New(db, "outreach", DefaultDeliveryConfig, (*DeliveryConfig).Validate)
}
