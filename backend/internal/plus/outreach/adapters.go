// Package outreach — 适配器：把现有 service / 其他 plus 模块包装成 ports.go 的接口
package outreach

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type emailMailer struct{ svc *service.EmailService }

// NewMailer 复用现有 SMTP 发信（后台「系统设置 → 邮件」里配置的那一套）
func NewMailer(svc *service.EmailService) Mailer { return &emailMailer{svc: svc} }

func (m *emailMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	return m.svc.SendEmail(ctx, to, subject, htmlBody)
}

type settingSite struct{ svc *service.SettingService }

func NewSiteInfo(svc *service.SettingService) SiteInfo { return &settingSite{svc: svc} }

func (s *settingSite) SiteName(ctx context.Context) string {
	ps, err := s.svc.GetPublicSettings(ctx)
	if err != nil || ps == nil || ps.SiteName == "" {
		return "Sub2API"
	}
	return ps.SiteName
}

func (s *settingSite) FrontendURL(ctx context.Context) string { return s.svc.GetFrontendURL(ctx) }

type pointsGranter struct{}

// NewPointsGranter 基于 plus/points 账本发积分
func NewPointsGranter() PointsGranter { return pointsGranter{} }

func (pointsGranter) GrantPoints(ctx context.Context, tx DBTX, userID, campaignID, pts int64) error {
	if _, err := points.Credit(ctx, tx, userID, pts, RewardReason, campaignID, "邮件活动奖励"); err != nil {
		return fmt.Errorf("outreach: grant %d points to user %d: %w", pts, userID, err)
	}
	return nil
}
