// Package emailverify — [plus] 业务逻辑层
package emailverify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

var (
	ErrMailerUnavailable = errors.New("emailverify: invite mailer not available")
	ErrAdminExempt       = errors.New("emailverify: admin users are exempt from verification")
	ErrAlreadyVerified   = errors.New("emailverify: email already verified")
	ErrEmailInvalid      = errors.New("emailverify: email address is marked invalid, please change it first")
	ErrCodeInvalid       = errors.New("emailverify: invalid or expired code")
	ErrCodeMaxAttempts   = errors.New("emailverify: too many failed attempts")
	ErrCodeTooFrequent   = errors.New("emailverify: code requested too frequently")
)

// Deps 服务依赖（全部通过接口注入）
type Deps struct {
	Repo       *Repository
	DB         *sql.DB
	Config     *ConfigStore
	Users      UserReader
	CodeSender CodeSender
	Rewarder   Rewarder // nil = 积分模块不可用，跳过奖励
	SiteName   SiteNameReader
}

// Service 邮箱验证业务编排
type Service struct {
	deps   Deps
	mailer InviteMailer
}

func NewService(deps Deps) *Service { return &Service{deps: deps} }

// SetInviteMailer 组合根注入发信能力（触达中心）；未注入时单发邀请不可用
func (s *Service) SetInviteMailer(m InviteMailer) { s.mailer = m }

func (s *Service) cfg(ctx context.Context) Config { return s.deps.Config.Cfg(ctx) }

// ─── 对其他模块开放的能力（触达中心通过自己定义的接口使用，本包不知道调用方是谁） ─────

// RewardPoints 当前验证奖励积分（邮件文案里展示）
func (s *Service) RewardPoints(ctx context.Context) int64 { return s.cfg(ctx).RewardPoints }

// IssueInviteToken 签发一次性验证 token；有效期由本模块决定
func (s *Service) IssueInviteToken(ctx context.Context, userID int64, email string) (string, error) {
	ttl := time.Duration(s.cfg(ctx).InviteTTLDays) * 24 * time.Hour
	return s.deps.Repo.CreateInviteToken(ctx, userID, email, ttl)
}

// MarkInvalid 发信硬退信：标记这个邮箱无效
func (s *Service) MarkInvalid(ctx context.Context, userID int64, email string) error {
	return s.deps.Repo.MarkInvalid(ctx, userID, email)
}

// RecordExternal 上游流程（注册 / 绑定邮箱 / 第三方登录）已经用验证码证明了邮箱归属，补记验证
func (s *Service) RecordExternal(ctx context.Context, userID int64, email, source string) error {
	if _, ok := SourceLabel[source]; !ok {
		return fmt.Errorf("emailverify: unknown source %q", source)
	}
	u, err := s.user(ctx, userID)
	if err != nil {
		return err
	}
	if u.Role == AdminRoleValue || !strings.EqualFold(u.Email, email) {
		return nil // 管理员不记录；邮箱对不上说明回调到达前又变了，以当前邮箱为准
	}
	_, err = s.markVerified(ctx, u, source)
	return err
}

// ─── 用户端 ───────────────────────────────────────────────────────────────────

func (s *Service) user(ctx context.Context, userID int64) (*EmailVerifyUser, error) {
	u, err := s.deps.Users.GetEmailVerifyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("emailverify: user %d not found", userID)
	}
	return u, nil
}

func (s *Service) rewardEligible(u *EmailVerifyUser, pts int64) bool {
	return s.deps.Rewarder != nil && u.TotalRecharged > 0 && pts > 0
}

// GetStatusView 用户的邮箱验证状态
func (s *Service) GetStatusView(ctx context.Context, userID int64) (*UserEmailStatusView, error) {
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Role == AdminRoleValue {
		return &UserEmailStatusView{Status: StatusExempt, Email: u.Email}, nil
	}
	st, err := s.deps.Repo.GetUserState(ctx, userID)
	if err != nil {
		return nil, err
	}
	pts := s.cfg(ctx).RewardPoints
	claimed := st.RewardClaimedAt != nil
	status := st.Status()
	return &UserEmailStatusView{
		Status:         status,
		Email:          u.Email,
		RewardEligible: s.rewardEligible(u, pts) && !claimed,
		RewardClaimed:  claimed,
		RewardPoints:   pts,
		Remind:         status == StatusPending || status == StatusInvalid,
	}, nil
}

// checkPending 当前邮箱必须是待验证
func (s *Service) checkPending(ctx context.Context, u *EmailVerifyUser) error {
	if u.Role == AdminRoleValue {
		return ErrAdminExempt
	}
	st, err := s.deps.Repo.GetUserState(ctx, u.ID)
	if err != nil {
		return err
	}
	switch st.Status() {
	case StatusVerified:
		return ErrAlreadyVerified
	case StatusInvalid:
		return ErrEmailInvalid
	}
	return nil
}

// SendCode 向用户当前邮箱发送验证码
// locale 决定验证码邮件的语言（上游通知邮件模板分中 / 英两套），取前端界面语言；为空时用中文
func (s *Service) SendCode(ctx context.Context, userID int64, locale string) error {
	u, err := s.user(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.checkPending(ctx, u); err != nil {
		return err
	}
	if strings.TrimSpace(locale) == "" {
		locale = "zh"
	}
	return s.deps.CodeSender.SendVerifyCode(ctx, u.Email, s.deps.SiteName.GetSiteName(ctx), locale)
}

// ConfirmCode 校验验证码，成功后记录验证并发放奖励（如有资格）
func (s *Service) ConfirmCode(ctx context.Context, userID int64, code string) (*VerifyResult, error) {
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Role == AdminRoleValue {
		return nil, ErrAdminExempt
	}
	if err := s.deps.CodeSender.VerifyCode(ctx, u.Email, code); err != nil {
		return nil, err
	}
	return s.markVerified(ctx, u, SourceCode)
}

// ConsumeInviteToken 消费一次性邀请 token；token 签发后换了邮箱则失效
func (s *Service) ConsumeInviteToken(ctx context.Context, token string) (*VerifyResult, error) {
	userID, email, err := s.deps.Repo.ConsumeInviteToken(ctx, token)
	if err != nil {
		return nil, err
	}
	u, err := s.deps.Users.GetEmailVerifyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u == nil || !strings.EqualFold(u.Email, email) {
		return nil, ErrTokenNotFound
	}
	if u.Role == AdminRoleValue {
		return &VerifyResult{Status: StatusExempt}, nil
	}
	return s.markVerified(ctx, u, SourceInvite)
}

// ─── 管理端 ───────────────────────────────────────────────────────────────────

// SendInviteToUser 给单个用户发验证邀请（发信交给触达中心）
func (s *Service) SendInviteToUser(ctx context.Context, userID int64) error {
	u, err := s.user(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.checkPending(ctx, u); err != nil {
		return err
	}
	if s.mailer == nil {
		return ErrMailerUnavailable
	}
	return s.mailer.SendVerifyInvite(ctx, u.ID)
}

func (s *Service) GetOverviewStats(ctx context.Context) (AdminOverviewStats, error) {
	return s.deps.Repo.GetOverviewStats(ctx)
}

func (s *Service) ListRecords(ctx context.Context, q RecordQuery) ([]Record, int64, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}
	q.Q = strings.TrimSpace(q.Q)
	if _, ok := SourceLabel[q.Source]; !ok {
		q.Source = ""
	}
	return s.deps.Repo.ListRecords(ctx, q)
}

func (s *Service) GetConfig(ctx context.Context) (Config, int64, error) {
	return s.deps.Config.Get(ctx)
}

func (s *Service) SaveConfig(ctx context.Context, cfg Config, ver int64) (int64, error) {
	return s.deps.Config.Save(ctx, cfg, ver)
}

// ─── 内部 ─────────────────────────────────────────────────────────────────────

// markVerified 一个事务里：记验证（同一邮箱只记一次）→ 清无效标记 → 抢奖励资格 → 发积分
func (s *Service) markVerified(ctx context.Context, u *EmailVerifyUser, source string) (*VerifyResult, error) {
	res := &VerifyResult{Status: StatusVerified}
	tx, err := s.deps.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("emailverify: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := s.deps.Repo.InsertVerification(ctx, tx, u.ID, u.Email, source, time.Now())
	if err != nil {
		return nil, fmt.Errorf("emailverify: insert verification: %w", err)
	}
	if id == 0 {
		return res, nil // 这个邮箱早就验证过
	}
	if err := s.deps.Repo.ClearInvalid(ctx, tx, u.ID, u.Email); err != nil {
		return nil, fmt.Errorf("emailverify: clear invalid: %w", err)
	}
	if pts := s.cfg(ctx).RewardPoints; s.rewardEligible(u, pts) {
		won, err := s.deps.Repo.ClaimReward(ctx, tx, u.ID)
		if err != nil {
			return nil, fmt.Errorf("emailverify: claim reward: %w", err)
		}
		if won {
			if _, err := s.deps.Rewarder.Grant(ctx, tx, u.ID, pts); err != nil {
				return nil, fmt.Errorf("emailverify: grant reward: %w", err)
			}
			if err := s.deps.Repo.SetRecordReward(ctx, tx, id, pts); err != nil {
				return nil, err
			}
			res.RewardGranted, res.RewardPoints = true, pts
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("emailverify: commit: %w", err)
	}
	if res.RewardGranted {
		log.Printf("[plus.emailverify] user %d verified via %s, rewarded %d points", u.ID, source, res.RewardPoints)
	}
	return res, nil
}
