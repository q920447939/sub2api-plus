// Package emailverify — [plus] 适配器：将现有 service 包装成本模块的接口
package emailverify

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/plus/points"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ─── UserReader 适配器 ────────────────────────────────────────────────────────

type serviceUserReader struct {
	svc *service.UserService
}

// NewUserReader 包装 UserService 实现 UserReader 接口
func NewUserReader(svc *service.UserService) UserReader {
	return &serviceUserReader{svc: svc}
}

func (a *serviceUserReader) GetEmailVerifyUser(ctx context.Context, userID int64) (*EmailVerifyUser, error) {
	u, err := a.svc.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("emailverify: get user %d: %w", userID, err)
	}
	if u == nil {
		return nil, nil
	}
	return &EmailVerifyUser{
		ID:             u.ID,
		Email:          u.Email,
		Role:           u.Role,
		TotalRecharged: u.TotalRecharged,
	}, nil
}

// ─── CodeSender 适配器 ────────────────────────────────────────────────────────

type emailServiceCodeSender struct {
	svc *service.EmailService
}

// NewCodeSender 包装 EmailService 实现 CodeSender 接口
func NewCodeSender(svc *service.EmailService) CodeSender {
	return &emailServiceCodeSender{svc: svc}
}

func (a *emailServiceCodeSender) SendVerifyCode(ctx context.Context, email, siteName string, locale ...string) error {
	return codeErr(a.svc.SendVerifyCode(ctx, email, siteName, locale...))
}

func (a *emailServiceCodeSender) VerifyCode(ctx context.Context, email, code string) error {
	return codeErr(a.svc.VerifyCode(ctx, email, code))
}

// codeErr 把上游验证码错误翻译成本模块的错误（上游文案是英文，且不应泄漏到本模块之外）
func codeErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, service.ErrInvalidVerifyCode):
		return ErrCodeInvalid
	case errors.Is(err, service.ErrVerifyCodeMaxAttempts):
		return ErrCodeMaxAttempts
	case errors.Is(err, service.ErrVerifyCodeTooFrequent):
		return ErrCodeTooFrequent
	}
	return err
}

// ─── Rewarder 适配器 ──────────────────────────────────────────────────────────

type pointsRewarder struct{}

// NewRewarder 返回基于 plus/points 包的积分发放适配器
func NewRewarder() Rewarder {
	return &pointsRewarder{}
}

func (a *pointsRewarder) Grant(ctx context.Context, tx DBTX, userID int64, pts int64) (int64, error) {
	entry, err := points.Credit(ctx, tx, userID, pts, RewardReason, userID, "邮箱验证奖励")
	if err != nil {
		return 0, fmt.Errorf("emailverify: grant reward to user %d: %w", userID, err)
	}
	return entry.ID, nil
}

// ─── SiteNameReader 适配器 ────────────────────────────────────────────────────

type settingSiteNameReader struct {
	svc *service.SettingService
}

// NewSiteNameReader 包装 SettingService 实现 SiteNameReader 接口
func NewSiteNameReader(svc *service.SettingService) SiteNameReader {
	return &settingSiteNameReader{svc: svc}
}

func (a *settingSiteNameReader) GetSiteName(ctx context.Context) string {
	settings, err := a.svc.GetPublicSettings(ctx)
	if err != nil || settings == nil {
		return "Sub2API"
	}
	if settings.SiteName == "" {
		return "Sub2API"
	}
	return settings.SiteName
}
