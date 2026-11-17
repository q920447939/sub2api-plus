// Package feedback — [plus] 用户端用例：提交、列表、详情、补充
package feedback

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UserScenario 用户端看到的场景。停用的也返回（Enabled=false），历史反馈要能显示名称；提交时只能选启用的
type UserScenario struct {
	Key     string  `json:"key"`
	Kind    Kind    `json:"kind"`
	Label   string  `json:"label"`
	Icon    string  `json:"icon"`
	Hint    string  `json:"hint"`
	Fields  []Field `json:"fields"`
	Enabled bool    `json:"enabled"`
}

// Meta 用户端初始化数据
type Meta struct {
	Enabled     bool           `json:"enabled"`
	FloatButton bool           `json:"float_button"`
	Notice      string         `json:"notice"` // 空 = 不显示
	Scenarios   []UserScenario `json:"scenarios"`
	MaxImages   int            `json:"max_images"`
	MaxImageMB  int            `json:"max_image_mb"`
	ContentMin  int            `json:"content_min"`
	ContentMax  int            `json:"content_max"`
	// AskTimeoutDays 管理员回复后用户多少天没回应会自动结案，0 = 不自动结案
	AskTimeoutDays int `json:"ask_timeout_days"`
}

func (s *Service) Meta(ctx context.Context) (*Meta, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	m := &Meta{
		Enabled: cfg.Enabled, FloatButton: cfg.Enabled && cfg.FloatButton,
		MaxImages: cfg.Attachments.MaxCount, MaxImageMB: cfg.Attachments.MaxMB,
		ContentMin: ContentMin, ContentMax: ContentMax, Scenarios: []UserScenario{},
		AskTimeoutDays: cfg.AskTimeoutDays,
	}
	// 总开关关闭时也返回场景名称，用户还能看懂自己的历史反馈
	for _, sc := range cfg.Scenarios {
		m.Scenarios = append(m.Scenarios, UserScenario{Key: sc.Key, Kind: sc.Kind, Label: sc.Label, Icon: sc.Icon, Hint: sc.Hint, Fields: sc.Fields, Enabled: sc.Enabled})
	}
	if !cfg.Enabled {
		return m, nil
	}
	if cfg.Notice.Enabled {
		m.Notice = strings.TrimSpace(cfg.Notice.Markdown)
	}
	return m, nil
}

// SubmitInput 用户提交
type SubmitInput struct {
	Kind        Kind              `json:"kind"`
	Scenario    string            `json:"scenario"`
	Content     string            `json:"content"`
	Extra       map[string]string `json:"extra"`
	Attachments []int64           `json:"attachments"`
	PrevID      *int64            `json:"prev_id"`
	Page        string            `json:"page"`
	Version     string            `json:"version"`
}

// ClientInfo 由 HTTP 层填入，用户不可见也不可改
type ClientInfo struct {
	IP        string
	UserAgent string
}

// submitContext 自动采集的信息
type submitContext struct {
	Page      string           `json:"page,omitempty"`
	Version   string           `json:"version,omitempty"`
	UserAgent string           `json:"user_agent,omitempty"`
	IP        string           `json:"ip,omitempty"`
	Account   *AccountSnapshot `json:"account,omitempty"`
}

func (s *Service) Submit(ctx context.Context, userID int64, in SubmitInput, ci ClientInfo) (*Feedback, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrDisabled
	}
	if !in.Kind.Valid() {
		return nil, inputErr("请选择问题或建议")
	}
	sc, ok := cfg.UsableScenario(in.Kind, in.Scenario)
	if !ok {
		return nil, inputErr("请选择场景")
	}
	in.Content = strings.TrimSpace(in.Content)
	if n := runeLen(in.Content); n < ContentMin || n > ContentMax {
		return nil, inputErr(fmt.Sprintf("描述需要 %d-%d 字", ContentMin, ContentMax))
	}
	if len(in.Attachments) > cfg.Attachments.MaxCount {
		return nil, inputErr(fmt.Sprintf("最多 %d 张图片", cfg.Attachments.MaxCount))
	}
	if err := s.checkRate(ctx, userID, cfg.Limits); err != nil {
		return nil, err
	}
	if in.PrevID != nil {
		prev, err := s.Repo.Get(ctx, *in.PrevID)
		if err != nil || prev.UserID != userID {
			return nil, inputErr("关联的反馈不存在")
		}
	}

	sctx := submitContext{Page: clip(in.Page, 200), Version: clip(in.Version, 40), UserAgent: clip(ci.UserAgent, 300), IP: ci.IP}
	if snap, err := s.Accounts.Snapshot(ctx, userID); err == nil {
		sctx.Account = &snap
	}
	extra, _ := json.Marshal(sc.CleanExtra(in.Extra))
	ctxJSON, _ := json.Marshal(sctx)
	f := &Feedback{
		UserID: userID, Kind: in.Kind, Scenario: sc.Key, Content: in.Content,
		Extra: extra, Context: ctxJSON, Attachments: nonNil(in.Attachments), PrevID: in.PrevID,
	}
	err = s.Repo.WithTx(ctx, func(q *queries, _ *sql.Tx) error {
		if err := q.Insert(ctx, f); err != nil {
			return err
		}
		if err := q.BindAttachments(ctx, f.Attachments, userID, f.ID); err != nil {
			return err
		}
		if f.PrevID != nil {
			return event(ctx, q, *f.PrevID, nil, false, Event{Type: EventFollowUp, RefID: f.ID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	f.KindUser, f.ScenarioUser = f.Kind, f.Scenario
	f.Tags = []string{}
	return f, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// checkRate 防刷：间隔、每日上限、同时未结案上限
func (s *Service) checkRate(ctx context.Context, userID int64, l Limits) error {
	now := s.Now()
	if l.IntervalSeconds > 0 {
		last, err := s.Repo.LastSubmitAt(ctx, userID)
		if err != nil {
			return err
		}
		if last != nil {
			if wait := time.Duration(l.IntervalSeconds)*time.Second - now.Sub(*last); wait > 0 {
				return &RateError{Msg: fmt.Sprintf("提交太快了，%d 秒后再试", int(wait.Seconds())+1)}
			}
		}
	}
	if l.DailyMax > 0 {
		n, err := s.Repo.CountSince(ctx, userID, now.Add(-24*time.Hour))
		if err != nil {
			return err
		}
		if n >= int64(l.DailyMax) {
			return &RateError{Msg: "今天提交的反馈已经够多了，明天再来吧"}
		}
	}
	if l.OpenMax > 0 {
		n, err := s.Repo.OpenCount(ctx, userID)
		if err != nil {
			return err
		}
		if n >= int64(l.OpenMax) {
			return &RateError{Msg: fmt.Sprintf("你有 %d 条反馈还在处理中，可以在原反馈里补充", n)}
		}
	}
	return nil
}

func (s *Service) ListMine(ctx context.Context, uq UserQuery) ([]ListItem, int64, int, int, error) {
	uq.Page, uq.PageSize = page(uq.Page, uq.PageSize)
	list, total, err := s.Repo.ListUser(ctx, uq)
	return list, total, uq.Page, uq.PageSize, err
}

// UserDetail 用户端详情（不含自动采集信息和内部条目）
type UserDetail struct {
	Feedback *Feedback `json:"feedback"`
	Messages []Message `json:"messages"`
	Related  []Related `json:"related"`
}

func (s *Service) MineDetail(ctx context.Context, userID, id int64) (*UserDetail, error) {
	f, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.UserID != userID {
		return nil, ErrNotFound
	}
	msgs, err := s.Repo.Messages(ctx, id, false)
	if err != nil {
		return nil, err
	}
	rel, err := s.Repo.Related(ctx, f, true)
	if err != nil {
		return nil, err
	}
	if f.UserUnread {
		if err := s.Repo.MarkRead(ctx, id); err != nil {
			return nil, err
		}
	}
	f.Context, f.Severity, f.Tags, f.KindUser, f.ScenarioUser = nil, "", nil, "", ""
	return &UserDetail{Feedback: f, Messages: msgs, Related: rel}, nil
}

// MessageInput 一条时间线消息
type MessageInput struct {
	Kind        MsgKind `json:"kind"` // 管理端：reply / ask / note；用户端忽略
	Body        string  `json:"body"`
	Attachments []int64 `json:"attachments"`
}

func (m *MessageInput) normalize(maxImages int) error {
	m.Body = strings.TrimSpace(m.Body)
	if m.Body == "" && len(m.Attachments) == 0 {
		return inputErr("内容不能为空")
	}
	if runeLen(m.Body) > ReplyMax {
		return inputErr(fmt.Sprintf("最多 %d 字", ReplyMax))
	}
	if len(m.Attachments) > maxImages {
		return inputErr(fmt.Sprintf("最多 %d 张图片", maxImages))
	}
	return nil
}

// Supplement 用户补充信息：待补充 → 待处理
func (s *Service) Supplement(ctx context.Context, userID, id int64, in MessageInput) (*Message, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if err := in.normalize(cfg.Attachments.MaxCount); err != nil {
		return nil, err
	}
	msg := &Message{FeedbackID: id, AuthorType: AuthorUser, AuthorID: &userID, Kind: MsgSupplement, Body: in.Body, Attachments: nonNil(in.Attachments)}
	err = s.mutate(ctx, id, func(q *queries, _ *sql.Tx, f *Feedback) error {
		if f.UserID != userID {
			return ErrNotFound
		}
		next, err := statusAfterUserReply(f.Status)
		if err != nil {
			return err
		}
		if err := q.BindAttachments(ctx, msg.Attachments, userID, id); err != nil {
			return err
		}
		if err := q.AddMessage(ctx, msg); err != nil {
			return err
		}
		return q.Apply(ctx, id, (&patch{}).set("status", next).raw("asked_at = NULL").raw("last_activity_at = NOW()"))
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *Service) Unread(ctx context.Context, userID int64) (int64, error) {
	return s.Repo.UnreadCount(ctx, userID)
}
