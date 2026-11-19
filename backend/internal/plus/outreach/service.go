// Package outreach — 业务编排：只依赖注册表（目的 / 权益 / 指标 / 变量）与端口，不认识具体目的。
package outreach

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// Deps 全部通过接口 / 注册表注入
type Deps struct {
	Repo     *Repository
	Settings *SettingsStore
	Mailer   Mailer
	Site     SiteInfo
	Purposes *PurposeRegistry
	Benefits *BenefitRegistry
	Metrics  *MetricRegistry
	Vars     Variables
	Verify   Verification // 邮箱验证模块；nil 表示不可用（「验证邮箱」目的不能发送）
}

type Service struct{ deps Deps }

func NewService(deps Deps) *Service { return &Service{deps: deps} }

func (s *Service) cfg(ctx context.Context) DeliveryConfig { return s.deps.Settings.Cfg(ctx) }

// linkBase 邮件里所有链接使用的域名：优先「触达链接域名」，否则回落到系统设置的站点地址
func (s *Service) linkBase(ctx context.Context) string {
	if b := strings.TrimRight(strings.TrimSpace(s.cfg(ctx).LinkBaseURL), "/"); b != "" {
		return b
	}
	return strings.TrimRight(strings.TrimSpace(s.deps.Site.FrontendURL(ctx)), "/")
}

// allowedBases 落地页允许的域名
func (s *Service) allowedBases(ctx context.Context) []string {
	return []string{s.cfg(ctx).LinkBaseURL, s.deps.Site.FrontendURL(ctx)}
}

var ErrFrontendURLMissing = badInput("请先在「发送设置」配置触达链接域名，或在系统设置里配置「站点地址」，邮件里的链接依赖它")

// ─── 设置 / 元数据 ───────────────────────────────────────────────────────────

func (s *Service) GetSettings(ctx context.Context) (DeliveryConfig, int64, error) {
	return s.deps.Settings.Get(ctx)
}

// SaveSettings 发送设置保存；同目的间隔只接受已注册的目的
func (s *Service) SaveSettings(ctx context.Context, cfg DeliveryConfig, ver int64) (int64, error) {
	gaps := map[string]int{}
	for k, v := range cfg.PurposeGapDays {
		if _, ok := s.deps.Purposes.Get(k); ok {
			gaps[k] = v
		}
	}
	cfg.PurposeGapDays = gaps
	return s.deps.Settings.Save(ctx, cfg, ver)
}

func (s *Service) Meta() MetaView {
	m := MetaView{Purposes: []PurposeMeta{}, Benefits: []BenefitMeta{}}
	for _, p := range s.deps.Purposes.All() {
		m.Purposes = append(m.Purposes, PurposeMeta{
			Key: p.Key(), Label: p.Label(), Description: p.Description(), Category: p.Policy().Category,
			StateText: p.State().Text, RequiresCTA: p.RequiresCTA(), AllowsBenefits: p.AllowsBenefits() && len(s.deps.Benefits.All()) > 0,
			AllowsLanding: p.AllowsLanding(), DefaultGapDays: p.DefaultGapDays(), Variables: s.deps.Vars.Docs(p, nil),
		})
	}
	for _, b := range s.deps.Benefits.All() {
		m.Benefits = append(m.Benefits, BenefitMeta{Kind: b.Kind(), Label: b.Label(), Fields: b.Fields(), Variables: b.Variables()})
	}
	return m
}

// ─── 内容校验（活动 / 模板共用）────────────────────────────────────────────

// checkContent 目的相关的校验：权益、按钮、落地页、未知变量
func (s *Service) checkContent(ctx context.Context, p Purpose, subject, body string, benefits []BenefitSpec, landing *string) error {
	if len(benefits) > 0 && !p.AllowsBenefits() {
		return badInput(fmt.Sprintf("「%s」不能附带权益", p.Label()))
	}
	if err := s.deps.Benefits.Validate(benefits); err != nil {
		return err
	}
	needCTA := p.RequiresCTA() || len(benefits) > 0
	if needCTA && !strings.Contains(body, "{{cta_url}}") {
		if len(benefits) > 0 {
			return badInput("附带权益时正文必须有按钮 {{cta_url}}，用户点它领取")
		}
		return badInput(fmt.Sprintf("「%s」的正文必须有按钮 {{cta_url}}", p.Label()))
	}
	if strings.Contains(subject, "{{cta_url}}") || strings.Contains(subject, "{{unsubscribe_url}}") {
		return badInput("主题里不能放链接变量")
	}
	if landing != nil {
		if !p.AllowsLanding() && strings.TrimSpace(*landing) != "" {
			return badInput(fmt.Sprintf("「%s」的按钮去向固定，不能设置落地页", p.Label()))
		}
		l, err := normalizeLanding(*landing, s.allowedBases(ctx))
		if err != nil {
			return err
		}
		*landing = l
	}
	if unk := s.deps.Vars.Unknown(p, benefits, subject, body); len(unk) > 0 {
		return badInput(fmt.Sprintf("「%s」不支持这些变量：%s", p.Label(), wrapVars(unk)))
	}
	return nil
}

func (s *Service) checkLayout(ctx context.Context, id *int64) error {
	ok, err := s.deps.Repo.layoutExists(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return badInput("所选外壳不存在")
	}
	return nil
}

// ─── 外壳 ────────────────────────────────────────────────────────────────────

func (s *Service) ListLayouts(ctx context.Context) ([]Layout, error) {
	return s.deps.Repo.ListLayouts(ctx)
}

func (s *Service) GetLayout(ctx context.Context, id int64) (*Layout, error) {
	return s.deps.Repo.GetLayout(ctx, id)
}

func (s *Service) CreateLayout(ctx context.Context, in LayoutInput) (*Layout, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	return s.deps.Repo.CreateLayout(ctx, in)
}

func (s *Service) UpdateLayout(ctx context.Context, id int64, in LayoutInput) (*Layout, error) {
	if err := in.normalize(); err != nil {
		return nil, err
	}
	return s.deps.Repo.UpdateLayout(ctx, id, in)
}

func (s *Service) DeleteLayout(ctx context.Context, id int64) error {
	return s.deps.Repo.DeleteLayout(ctx, id)
}

func (s *Service) SetDefaultLayout(ctx context.Context, id int64) error {
	return s.deps.Repo.SetDefaultLayout(ctx, id)
}

// ─── 模板 ────────────────────────────────────────────────────────────────────

func (s *Service) ListTemplates(ctx context.Context, purpose string) ([]Template, error) {
	return s.deps.Repo.ListTemplates(ctx, purpose)
}

func (s *Service) GetTemplate(ctx context.Context, id int64) (*Template, error) {
	return s.deps.Repo.GetTemplate(ctx, id)
}

func (s *Service) prepareTemplate(ctx context.Context, in *TemplateInput) error {
	if err := in.normalize(); err != nil {
		return err
	}
	p, err := s.deps.Purposes.Lookup(in.Purpose)
	if err != nil {
		return err
	}
	// 模板不带权益；附带权益在活动里加，所以这里只要求目的本身要求的按钮
	if err := s.checkContent(ctx, p, in.Subject, in.BodyMD, nil, nil); err != nil {
		return err
	}
	return s.checkLayout(ctx, in.LayoutID)
}

func (s *Service) CreateTemplate(ctx context.Context, in TemplateInput) (*Template, error) {
	if err := s.prepareTemplate(ctx, &in); err != nil {
		return nil, err
	}
	return s.deps.Repo.CreateTemplate(ctx, in)
}

func (s *Service) UpdateTemplate(ctx context.Context, id int64, in TemplateInput) (*Template, error) {
	cur, err := s.deps.Repo.GetTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	in.Purpose = cur.Purpose
	if err := s.prepareTemplate(ctx, &in); err != nil {
		return nil, err
	}
	return s.deps.Repo.UpdateTemplate(ctx, id, in)
}

func (s *Service) DeleteTemplate(ctx context.Context, id int64) error {
	return s.deps.Repo.DeleteTemplate(ctx, id)
}

// ─── 活动 ────────────────────────────────────────────────────────────────────

func (s *Service) prepareCampaign(ctx context.Context, in *CampaignInput) error {
	if err := in.normalize(); err != nil {
		return err
	}
	p, err := s.deps.Purposes.Lookup(in.Purpose)
	if err != nil {
		return err
	}
	if err := s.checkContent(ctx, p, in.Subject, in.BodyMD, in.Benefits, &in.LandingURL); err != nil {
		return err
	}
	return s.checkLayout(ctx, in.LayoutID)
}

func (s *Service) Create(ctx context.Context, in CampaignInput, adminID int64) (*Campaign, error) {
	if err := s.prepareCampaign(ctx, &in); err != nil {
		return nil, err
	}
	return s.deps.Repo.CreateCampaign(ctx, in, adminID)
}

func (s *Service) Update(ctx context.Context, id int64, in CampaignInput) (*Campaign, error) {
	if err := s.prepareCampaign(ctx, &in); err != nil {
		return nil, err
	}
	return s.deps.Repo.UpdateCampaign(ctx, id, in)
}

func (s *Service) Get(ctx context.Context, id int64) (*CampaignDetail, error) {
	d, err := s.deps.Repo.GetCampaignDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	p, err := s.deps.Purposes.Lookup(d.Purpose)
	if err != nil {
		return nil, err
	}
	d.Rules = describeAudience(p, d.Segment)
	return d, nil
}

func (s *Service) Duplicate(ctx context.Context, id, adminID int64) (*Campaign, error) {
	return s.deps.Repo.DuplicateCampaign(ctx, id, adminID)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.deps.Repo.DeleteCampaign(ctx, id)
}

// primaryOf 一封邮件的核心指标
func (s *Service) primaryOf(purpose, body string, benefits []BenefitSpec, st StatRow) *MetricView {
	p, ok := s.deps.Purposes.Get(purpose)
	if !ok {
		return nil
	}
	keys := s.deps.Metrics.Applicable(p, strings.Contains(body, "{{cta_url}}"), len(benefits) > 0)
	if len(keys) == 0 {
		return nil
	}
	v := s.deps.Metrics.Views(keys[:1], st.Values)[0]
	return &v
}

func (s *Service) List(ctx context.Context, page, size int) ([]CampaignRow, int64, error) {
	rows, stats, total, err := s.deps.Repo.ListCampaigns(ctx, page, size, s.deps.Metrics)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].Primary = s.primaryOf(rows[i].Purpose, rows[i].BodyMD, rows[i].Benefits, stats[i])
	}
	return rows, total, nil
}

// Preview 人群预估。campaignID > 0 时附带「本活动已发送 / 排队」及两种选人范围的可选人数。
func (s *Service) Preview(ctx context.Context, purpose string, sg Segment, campaignID int64) (*AudiencePreview, error) {
	p, err := s.deps.Purposes.Lookup(purpose)
	if err != nil {
		return nil, err
	}
	if err := sg.validate(); err != nil {
		return nil, badInput(err.Error())
	}
	out, err := s.deps.Repo.PreviewAudience(ctx, buildAudience(p, sg, s.cfg(ctx), campaignID))
	if err != nil {
		return nil, err
	}
	out.Rules = describeAudience(p, sg)
	return out, nil
}

// ─── 渲染（预览 / 测试 / 逐人预览 / 发送共用）──────────────────────────────

// mailSpec 一封邮件的内容来源（活动、批次快照、模板）
type mailSpec struct {
	Purpose  Purpose
	Subject  string
	BodyMD   string
	Layout   LayoutConfig
	Benefits []BenefitSpec
}

// compose 渲染管线的唯一入口
func (s *Service) compose(ctx context.Context, spec mailSpec, rc RenderContext) (subject, html string) {
	rc.Purpose, rc.Benefits = spec.Purpose, spec.Benefits
	rc.SiteName = s.deps.Site.SiteName(ctx)
	return renderMail(MailInput{
		Policy: spec.Purpose.Policy(), Subject: spec.Subject, BodyMD: spec.BodyMD, Layout: spec.Layout,
		Vars: s.deps.Vars.Resolve(ctx, &rc), SiteName: rc.SiteName, UnsubURL: rc.UnsubURL,
	})
}

// trackedLinks 真实收件人的跟踪链接
func trackedLinks(base, token string) (cta, unsub string) {
	return base + "/api/v1/plus/outreach/r/" + token, base + "/unsubscribe?t=" + token
}

// sampleContext 示例数据（编辑器 / 外壳 / 模板预览）
func sampleContext() RenderContext {
	return RenderContext{
		Email: "demo@example.com", User: RecipientVars{Username: "小明", Balance: "12.50", LastActiveDays: 35},
		CTAURL: "https://example.com/preview", UnsubURL: "https://example.com/unsubscribe",
	}
}

// RenderRequest 编辑器预览入参：layout 优先于 layout_id（外壳编辑页直接传未保存的配置）
type RenderRequest struct {
	Purpose  string        `json:"purpose"`
	Subject  string        `json:"subject"`
	BodyMD   string        `json:"body_md"`
	LayoutID *int64        `json:"layout_id"`
	Layout   *LayoutConfig `json:"layout"`
	Benefits []BenefitSpec `json:"benefits"`
}

func (s *Service) RenderPreview(ctx context.Context, req RenderRequest) (string, string, error) {
	p, err := s.deps.Purposes.Lookup(req.Purpose)
	if err != nil {
		return "", "", err
	}
	_ = s.deps.Benefits.Validate(req.Benefits) // 预览时配置不完整也尽量渲染
	var lay LayoutConfig
	if req.Layout != nil {
		lay = *req.Layout
	} else if lay, err = s.deps.Repo.ResolveLayout(ctx, req.LayoutID); err != nil {
		return "", "", err
	}
	subject, html := s.compose(ctx, mailSpec{p, req.Subject, req.BodyMD, lay, req.Benefits}, sampleContext())
	return subject, html, nil
}

// campaignSpec 活动当前内容（外壳按当前设置解析）
func (s *Service) campaignSpec(ctx context.Context, c *Campaign) (mailSpec, error) {
	p, err := s.deps.Purposes.Lookup(c.Purpose)
	if err != nil {
		return mailSpec{}, err
	}
	lay, err := s.deps.Repo.ResolveLayout(ctx, c.LayoutID)
	if err != nil {
		return mailSpec{}, err
	}
	return mailSpec{p, c.Subject, c.BodyMD, lay, c.Benefits}, nil
}

// SendTest 只发给操作者自己；sampleEmail 非空时用该用户的真实数据渲染（看某个人实际会收到什么）。
// 不产生收件人记录、不写台账；按钮直接指向最终去向（验证邮箱目的没有真实 token，指向首页）。
func (s *Service) SendTest(ctx context.Context, id, adminID int64, sampleEmail string) (string, error) {
	c, err := s.deps.Repo.GetCampaign(ctx, id)
	if err != nil {
		return "", err
	}
	spec, err := s.campaignSpec(ctx, c)
	if err != nil {
		return "", err
	}
	to, err := s.deps.Repo.AdminEmail(ctx, adminID)
	if err != nil || to == "" {
		return "", fmt.Errorf("outreach: load admin email: %w", err)
	}
	base := s.linkBase(ctx)
	if base == "" {
		return "", ErrFrontendURLMissing
	}
	rc := RenderContext{Email: to, User: RecipientVars{Username: localPart(to), Balance: "12.50", LastActiveDays: 35}}
	if e := strings.TrimSpace(sampleEmail); e != "" {
		uid, err := s.deps.Repo.UserIDByEmail(ctx, e)
		if err != nil {
			return "", badInput("找不到这个邮箱对应的用户：" + e)
		}
		sc, err := s.deps.Repo.LoadSendContext(ctx, uid, spec.Purpose, s.cfg(ctx))
		if err != nil {
			return "", err
		}
		rc.Email, rc.User = sc.Email, sc.vars()
	}
	target, err := actionFor(spec.Purpose, false).Resolve(ActionTarget{Base: base, Landing: c.LandingURL})
	if err != nil {
		target = base + "/"
	}
	rc.CTAURL, rc.UnsubURL = target, base+"/unsubscribe?t=test"
	subject, body := s.compose(ctx, spec, rc)
	if err := s.deps.Mailer.Send(ctx, to, "[测试] "+subject, body); err != nil {
		return "", fmt.Errorf("发送测试邮件失败：%w", err)
	}
	return to, nil
}

// ─── 批次 ────────────────────────────────────────────────────────────────────

// CreateBatch 生成批次：冻结内容 / 外壳 / 落地页 / 权益快照和名单，状态为「待执行」，此时不发任何邮件
func (s *Service) CreateBatch(ctx context.Context, campaignID int64, req BatchRequest, adminID int64) (*BatchCreated, error) {
	if err := req.normalize(); err != nil {
		return nil, err
	}
	c, err := s.deps.Repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	spec, err := s.campaignSpec(ctx, c)
	if err != nil {
		return nil, err
	}
	if missing := s.deps.Benefits.MissingKinds(c.Benefits); len(missing) > 0 {
		return nil, badInput("活动附带的权益当前不可用：" + strings.Join(missing, "、"))
	}
	res, err := s.deps.Repo.CreateBatch(ctx, c, spec.Layout, buildAudience(spec.Purpose, c.Segment, s.cfg(ctx), c.ID), req, adminID)
	if err != nil {
		return nil, err
	}
	if res.Matched == 0 {
		switch {
		case req.Mode == PickUsers:
			return nil, badInput("填写的邮箱里没有可选的用户（可能不存在、不在目标人群里、正在其他批次排队，或「只发未收到的人」时已经收到过）")
		case req.Scope == ScopeFresh:
			return nil, badInput("目标人群里的人都已收到过本活动的邮件（或正在排队）。如需再发一轮，请把选人范围改为「再发一轮」")
		}
		return nil, badInput("按当前条件没有可选的用户")
	}
	return res, nil
}

func (s *Service) GetBatch(ctx context.Context, id int64) (*Batch, error) {
	return s.deps.Repo.GetBatch(ctx, id)
}

func (s *Service) ListBatches(ctx context.Context, campaignID int64, page, size int) ([]BatchRow, int64, error) {
	if _, err := s.deps.Repo.GetCampaign(ctx, campaignID); err != nil {
		return nil, 0, err
	}
	return s.deps.Repo.ListBatches(ctx, campaignID, page, size)
}

func (s *Service) StartBatch(ctx context.Context, id int64) error {
	b, err := s.deps.Repo.GetBatch(ctx, id)
	if err != nil {
		return err
	}
	if b.Status != StatusPending {
		return ErrBadState
	}
	return s.activate(ctx, b, StatusPending)
}

// ResumeBatch 恢复已暂停的批次（熔断暂停后必须由人确认才会继续）
func (s *Service) ResumeBatch(ctx context.Context, id int64) error {
	b, err := s.deps.Repo.GetBatch(ctx, id)
	if err != nil {
		return err
	}
	if b.Status != StatusPaused {
		return ErrBadState
	}
	return s.activate(ctx, b, StatusPaused)
}

func (s *Service) activate(ctx context.Context, b *Batch, from string) error {
	if s.linkBase(ctx) == "" {
		return ErrFrontendURLMissing
	}
	p, err := s.deps.Purposes.Lookup(b.Purpose)
	if err != nil {
		return err
	}
	if missing := s.deps.Benefits.MissingKinds(b.Benefits); len(missing) > 0 {
		return badInput("批次附带的权益当前不可用：" + strings.Join(missing, "、"))
	}
	if _, isVerify := p.Action().(verifyAction); isVerify && s.deps.Verify == nil {
		return badInput("邮箱验证模块不可用")
	}
	return s.deps.Repo.ActivateBatch(ctx, b.ID, from)
}

func (s *Service) PauseBatch(ctx context.Context, id int64) error {
	return s.deps.Repo.SetBatchStatus(ctx, id, StatusPaused, "管理员手动暂停", StatusRunning)
}

func (s *Service) CancelBatch(ctx context.Context, id int64) error {
	return s.deps.Repo.CancelBatch(ctx, id)
}

func (s *Service) DeleteBatch(ctx context.Context, id int64) error {
	return s.deps.Repo.DeleteBatch(ctx, id)
}

// BatchStats 批次只看投递结果
func (s *Service) BatchStats(ctx context.Context, id int64) (*DeliveryStats, error) {
	if _, err := s.deps.Repo.GetBatch(ctx, id); err != nil {
		return nil, err
	}
	return s.deps.Repo.BatchDelivery(ctx, id)
}

// CampaignStats 累计效果：指标按活动当前内容挑选（各批次快照可能不同，以当前定义为准）
func (s *Service) CampaignStats(ctx context.Context, id int64) (*StatsView, error) {
	c, err := s.deps.Repo.GetCampaign(ctx, id)
	if err != nil {
		return nil, err
	}
	st, err := s.deps.Repo.CampaignStats(ctx, id, s.deps.Metrics)
	if err != nil {
		return nil, err
	}
	p, err := s.deps.Purposes.Lookup(c.Purpose)
	if err != nil {
		return nil, err
	}
	keys := s.deps.Metrics.Applicable(p, strings.Contains(c.BodyMD, "{{cta_url}}"), len(c.Benefits) > 0)
	return &StatsView{DeliveryStats: st.DeliveryStats, Metrics: s.deps.Metrics.Views(keys, st.Values)}, nil
}

func (s *Service) Recipients(ctx context.Context, batchID int64, status, q string, page, size int) ([]RecipientRow, int64, error) {
	if _, err := s.deps.Repo.GetBatch(ctx, batchID); err != nil {
		return nil, 0, err
	}
	return s.deps.Repo.ListRecipients(ctx, batchID, status, strings.TrimSpace(q), page, size)
}

// MailPreview 某个收件人的邮件内容
type MailPreview struct {
	Email   string `json:"email"`
	Status  string `json:"status"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

func batchSpec(p Purpose, b *Batch) mailSpec {
	return mailSpec{p, b.Subject, b.BodyMD, b.Layout, b.Benefits}
}

// RecipientPreview 用批次快照 + 该收件人的变量快照渲染：未发送的按名单生成时的数据，已发送的按实际发出时的数据
func (s *Service) RecipientPreview(ctx context.Context, batchID, rid int64) (*MailPreview, error) {
	b, err := s.deps.Repo.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	p, err := s.deps.Purposes.Lookup(b.Purpose)
	if err != nil {
		return nil, err
	}
	d, err := s.deps.Repo.GetRecipientPreview(ctx, batchID, rid)
	if err != nil {
		return nil, err
	}
	base := s.linkBase(ctx)
	if base == "" {
		return nil, ErrFrontendURLMissing
	}
	cta, unsub := trackedLinks(base, d.Token)
	subject, html := s.compose(ctx, batchSpec(p, b), RenderContext{Email: d.Email, User: d.Vars, CTAURL: cta, UnsubURL: unsub})
	return &MailPreview{Email: d.Email, Status: d.Status, Subject: subject, HTML: html}, nil
}

func (s *Service) PurgeDeleted(ctx context.Context) (batches, campaigns int64, err error) {
	return s.deps.Repo.PurgeDeleted(ctx, s.cfg(ctx).BatchRetentionDays)
}

// ─── 投递（worker 与直发共用）────────────────────────────────────────────────

// sendOutcome 一次投递的结果
type sendOutcome struct {
	SendErr error  // SMTP 返回的错误
	Skip    string // 发送前复核不通过的原因
	Retry   error  // 内部错误，稍后重试
	Halt    error  // 配置问题，需要人处理
}

// deliverOne 复核 → 签发验证 token（如需）→ 渲染 → 发送 → 记台账。
// checkFreq=false 用于直发（管理员明确要发给这个人，不受频控限制）。
func (s *Service) deliverOne(ctx context.Context, spec mailSpec, l Leased, checkFreq bool) sendOutcome {
	repo := s.deps.Repo
	sc, err := repo.LoadSendContext(ctx, l.UserID, spec.Purpose, s.cfg(ctx))
	if err != nil {
		return sendOutcome{Retry: fmt.Errorf("load user: %w", err)}
	}
	if why := skipReason(spec.Purpose, sc, l.Email, checkFreq); why != "" {
		return sendOutcome{Skip: why}
	}
	base := s.linkBase(ctx)
	if base == "" {
		return sendOutcome{Halt: ErrFrontendURLMissing}
	}
	if _, isVerify := spec.Purpose.Action().(verifyAction); isVerify && l.InviteToken == "" {
		if s.deps.Verify == nil {
			return sendOutcome{Halt: errors.New("邮箱验证模块不可用")}
		}
		tok, err := s.deps.Verify.IssueInviteToken(ctx, l.UserID, l.Email)
		if err != nil {
			return sendOutcome{Retry: fmt.Errorf("issue token: %w", err)}
		}
		if err := repo.SaveInviteToken(ctx, l.ID, tok); err != nil {
			return sendOutcome{Retry: fmt.Errorf("save token: %w", err)}
		}
	}
	cta, unsub := trackedLinks(base, l.Token)
	rv := sc.vars()
	subject, body := s.compose(ctx, spec, RenderContext{Email: l.Email, User: rv, CTAURL: cta, UnsubURL: unsub})
	if err := s.deps.Mailer.Send(ctx, l.Email, subject, body); err != nil {
		return sendOutcome{SendErr: err}
	}
	if err := repo.MarkSent(ctx, l.ID, spec.Purpose.Policy().Category, rv); err != nil {
		// 邮件已经发出：只记日志，绝不能当成失败去重试，否则会重复发信
		log.Printf("[plus.outreach] mark sent %d: %v", l.ID, err)
	}
	return sendOutcome{}
}

func skipReason(p Purpose, sc SendContext, snapshotEmail string, checkFreq bool) string {
	switch {
	case !sc.Exists:
		return "用户已不存在"
	case sc.Role == "admin":
		return "管理员不发送"
	case sc.Status != "active":
		return "用户已被禁用"
	case sc.Email != snapshotEmail:
		return "用户已更换邮箱"
	case sc.Invalid:
		return "邮箱已标记无效"
	}
	if why := p.State().skip(sc.Verified); why != "" {
		return why
	}
	if p.Policy().Marketing() && sc.Unsubscribed {
		return "用户已退订"
	}
	if checkFreq && sc.Recent {
		return "近期已收到邮件（频控）"
	}
	return ""
}

// SendVerifyInvite 直发：用「验证邮箱」的内置模板立即给单个用户发一封验证邀请。
// emailverify 模块通过它发邀请，自己不再拼邮件（emailverify.InviteMailer 端口）。
func (s *Service) SendVerifyInvite(ctx context.Context, userID int64) error {
	return s.DirectSend(ctx, PurposeVerifyEmail, userID)
}

// DirectSend 不经活动 / 批次，用某目的的内置模板立即发给一个用户；不受频控限制，但仍复核用户状态
func (s *Service) DirectSend(ctx context.Context, purpose string, userID int64) error {
	p, err := s.deps.Purposes.Lookup(purpose)
	if err != nil {
		return err
	}
	t, err := s.deps.Repo.BuiltinTemplate(ctx, purpose)
	if err != nil {
		return fmt.Errorf("outreach: builtin template for %s: %w", purpose, err)
	}
	lay, err := s.deps.Repo.ResolveLayout(ctx, t.LayoutID)
	if err != nil {
		return err
	}
	sc, err := s.deps.Repo.LoadSendContext(ctx, userID, p, s.cfg(ctx))
	if err != nil {
		return err
	}
	if !sc.Exists {
		return ErrNotFound
	}
	if why := skipReason(p, sc, sc.Email, false); why != "" {
		return badInput("不能发送：" + why)
	}
	rid, token, err := s.deps.Repo.CreateDirectRecipient(ctx, purpose, userID, sc.Email)
	if err != nil {
		return err
	}
	out := s.deliverOne(ctx, mailSpec{p, t.Subject, t.BodyMD, lay, nil}, Leased{ID: rid, UserID: userID, Email: sc.Email, Token: token, Attempts: 1}, false)
	switch {
	case out.SendErr != nil:
		_ = s.deps.Repo.MarkFailed(ctx, rid, classify(out.SendErr) == errHardBounce, out.SendErr.Error())
		return fmt.Errorf("发送失败：%w", out.SendErr)
	case out.Skip != "":
		_ = s.deps.Repo.MarkSkipped(ctx, rid, out.Skip)
		return badInput("不能发送：" + out.Skip)
	case out.Halt != nil:
		_ = s.deps.Repo.MarkSkipped(ctx, rid, out.Halt.Error())
		return out.Halt
	case out.Retry != nil:
		_ = s.deps.Repo.MarkFailed(ctx, rid, false, out.Retry.Error())
		return out.Retry
	}
	return nil
}

// ─── 用户侧：点击 / 领取 / 退订 ──────────────────────────────────────────────

// resolveRedirect 按目的 + 是否带权益解析去向
func (s *Service) resolveRedirect(ctx context.Context, rd Redirect, withClaim bool) (string, error) {
	p, err := s.deps.Purposes.Lookup(rd.Purpose)
	if err != nil {
		return "", err
	}
	return actionFor(p, withClaim).Resolve(ActionTarget{
		Base: s.linkBase(ctx), Landing: rd.Landing, Token: rd.Token, InviteToken: rd.InviteToken,
	})
}

// Click 记录点击，返回应跳转的地址
func (s *Service) Click(ctx context.Context, token string) (string, error) {
	rd, err := s.deps.Repo.RecordClick(ctx, token)
	if err != nil {
		return "", err
	}
	return s.resolveRedirect(ctx, *rd, rd.BenefitCount > 0)
}

// ClaimInfo 领取页展示
type ClaimInfo struct {
	Items   []string `json:"items"`
	Claimed bool     `json:"claimed"`
	Expired bool     `json:"expired"`
	Next    string   `json:"next"` // 领完之后去哪（由服务端解析，不接受前端传入，避免外站跳转）
}

func (s *Service) ClaimInfo(ctx context.Context, token string, userID int64) (*ClaimInfo, error) {
	cd, err := s.deps.Repo.GetClaimData(ctx, token, userID)
	if err != nil {
		return nil, err
	}
	next, err := s.resolveRedirect(ctx, cd.Next, false)
	if err != nil {
		next = s.linkBase(ctx) + "/"
	}
	return &ClaimInfo{Items: s.deps.Benefits.Describe(cd.Specs), Claimed: cd.Claimed, Expired: cd.Expired, Next: next}, nil
}

// Claim 领取：所有权益在同一事务内发放，要么全部到账要么都不到账
func (s *Service) Claim(ctx context.Context, token string, userID int64) ([]string, error) {
	return s.deps.Repo.Claim(ctx, token, userID,
		func(tx DBTX, req GrantRequest, specs []BenefitSpec) ([]string, error) {
			if missing := s.deps.Benefits.MissingKinds(specs); len(missing) > 0 {
				return nil, badInput("权益暂不可用，请稍后再试")
			}
			return s.deps.Benefits.GrantAll(ctx, tx, req, specs)
		})
}

// Unsubscribe 返回退订的目的名称；服务类邮件不可退订，返回空名称
func (s *Service) Unsubscribe(ctx context.Context, token string) (string, bool, error) {
	key, err := s.deps.Repo.Unsubscribe(ctx, token, func(purpose string) bool {
		p, ok := s.deps.Purposes.Get(purpose)
		return ok && p.Policy().Marketing()
	})
	if err != nil {
		return "", false, err
	}
	p, ok := s.deps.Purposes.Get(key)
	if !ok {
		return "", false, nil
	}
	return p.Label(), p.Policy().Marketing(), nil
}

// ─── 工具 ────────────────────────────────────────────────────────────────────

func localPart(email string) string {
	if i := strings.IndexByte(email, '@'); i > 0 {
		return email[:i]
	}
	return email
}

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
