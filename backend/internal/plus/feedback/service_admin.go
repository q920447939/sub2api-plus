// Package feedback — [plus] 管理端用例：列表、详情、回复/追问/备注、定级与分类、结案、批量、超时自动结案
package feedback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AdminList 列表 + 各状态数量
func (s *Service) AdminList(ctx context.Context, aq AdminQuery) ([]ListItem, int64, int, int, error) {
	aq.Page, aq.PageSize = page(aq.Page, aq.PageSize)
	list, total, err := s.Repo.ListAdmin(ctx, aq)
	return list, total, aq.Page, aq.PageSize, err
}

func (s *Service) StatusCounts(ctx context.Context) (map[Status]int64, error) {
	return s.Repo.StatusCounts(ctx)
}

func (s *Service) PendingCount(ctx context.Context) (int64, error) { return s.Repo.PendingCount(ctx) }

func (s *Service) Tags(ctx context.Context) ([]string, error) { return s.Repo.Tags(ctx) }

// AdminDetail 管理端详情：时间线 + 上下文面板
type AdminDetail struct {
	Feedback *Feedback  `json:"feedback"`
	Messages []Message  `json:"messages"`
	Related  []Related  `json:"related"`
	User     *UserCard  `json:"user"`
	Errors   []ErrorLog `json:"errors"`
	Impact   int64      `json:"impact"`
}

// 错误请求窗口：提交前 24 小时到提交后 1 小时
const (
	errorsBefore = 24 * time.Hour
	errorsAfter  = time.Hour
	errorsLimit  = 30
)

func (s *Service) AdminDetail(ctx context.Context, id int64) (*AdminDetail, error) {
	f, err := s.Repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &AdminDetail{Feedback: f, Impact: 1}
	if d.Messages, err = s.Repo.Messages(ctx, id, true); err != nil {
		return nil, err
	}
	if d.Related, err = s.Repo.Related(ctx, f, false); err != nil {
		return nil, err
	}
	for _, r := range d.Related {
		if r.Relation == "merged_in" {
			d.Impact++
		}
	}
	if d.User, err = s.Accounts.Card(ctx, f.UserID); err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if d.Errors, err = s.Errors.Around(ctx, f.UserID, f.CreatedAt.Add(-errorsBefore), f.CreatedAt.Add(errorsAfter), errorsLimit); err != nil {
		return nil, err
	}
	return d, nil
}

// AdminMessage 回复（不改状态）/ 追问（→ 待补充）/ 内部备注。结案后只能写备注
func (s *Service) AdminMessage(ctx context.Context, adminID, id int64, in MessageInput) (*Message, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return nil, err
	}
	if err := in.normalize(cfg.Attachments.MaxCount); err != nil {
		return nil, err
	}
	msg := &Message{FeedbackID: id, AuthorType: AuthorAdmin, AuthorID: &adminID, Kind: in.Kind, Body: in.Body,
		Attachments: nonNil(in.Attachments), Internal: in.Kind == MsgNote}
	err = s.mutate(ctx, id, func(q *queries, _ *sql.Tx, f *Feedback) error {
		if !f.Status.Open() && in.Kind != MsgNote {
			return ErrClosed
		}
		next, err := statusAfterAdminMessage(f.Status, in.Kind)
		if err != nil {
			return err
		}
		if err := q.BindAttachments(ctx, msg.Attachments, adminID, id); err != nil {
			return err
		}
		if err := q.AddMessage(ctx, msg); err != nil {
			return err
		}
		p := (&patch{}).set("status", next)
		if next == StatusAwaiting {
			p.raw("asked_at = NOW()")
		}
		if userVisible(in.Kind) {
			p.set("user_unread", true).raw("last_activity_at = NOW()")
		}
		return q.Apply(ctx, id, p)
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// UpdateInput 管理端修改分析字段；nil = 不改
type UpdateInput struct {
	Kind     *Kind     `json:"kind"`
	Scenario *string   `json:"scenario"`
	Severity *Severity `json:"severity"` // 空字符串 = 清除
	Tags     *[]string `json:"tags"`
}

func (s *Service) AdminUpdate(ctx context.Context, adminID, id int64, in UpdateInput) error {
	cfg, err := s.Config(ctx)
	if err != nil {
		return err
	}
	return s.mutate(ctx, id, func(q *queries, _ *sql.Tx, f *Feedback) error {
		p := &patch{}
		actor := &adminID

		kind, scenario := f.Kind, f.Scenario
		if in.Kind != nil {
			kind = *in.Kind
		}
		if in.Scenario != nil {
			scenario = *in.Scenario
		}
		if kind != f.Kind || scenario != f.Scenario {
			if kind != f.Kind && !f.Status.Open() {
				return inputErr("已结案的反馈不能修改类型")
			}
			if !kind.Valid() {
				return inputErr("类型无效")
			}
			sc, ok := cfg.Scenario(scenario)
			if !ok || sc.Kind != kind {
				return inputErr("请选择这个类型下的场景")
			}
			p.set("kind", kind).set("scenario", scenario)
			if err := event(ctx, q, id, actor, true, Event{Type: EventReclass, Kind: kind, Scenario: scenario}); err != nil {
				return err
			}
		}
		if in.Severity != nil && *in.Severity != f.Severity {
			sev := *in.Severity
			if sev == "" {
				p.raw("severity = NULL")
			} else if !sev.Valid() {
				return inputErr("严重等级无效")
			} else {
				p.set("severity", sev)
			}
			if err := event(ctx, q, id, actor, true, Event{Type: EventSeverity, Severity: sev}); err != nil {
				return err
			}
		}
		if in.Tags != nil {
			tags := NormalizeTags(*in.Tags)
			if !sameStrings(tags, f.Tags) {
				p.set("tags", pqStrings(tags))
				if err := event(ctx, q, id, actor, true, Event{Type: EventTags, Tags: tags}); err != nil {
					return err
				}
			}
		}
		return q.Apply(ctx, id, p)
	})
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Close 结案：校验结论 → 合并 → 发积分（同一事务）→ 写时间线 → 用户端红点
func (s *Service) Close(ctx context.Context, adminID, id int64, req CloseRequest) error {
	if req.RewardPoints > 0 && s.Rewarder == nil {
		return ErrRewardDisabled
	}
	req.Reason = clip(req.Reason, 500)
	req.Message = clip(req.Message, ReplyMax)
	return s.mutate(ctx, id, func(q *queries, tx *sql.Tx, f *Feedback) error {
		if err := ValidateClose(f, req); err != nil {
			return err
		}
		actor := &adminID
		p := (&patch{}).set("status", req.Verdict).set("user_unread", true).
			raw("closed_at = NOW()").raw("asked_at = NULL").raw("last_activity_at = NOW()")
		if req.Reason != "" {
			p.set("close_reason", req.Reason)
		}
		if req.Message != "" {
			if err := q.AddMessage(ctx, &Message{FeedbackID: id, AuthorType: AuthorAdmin, AuthorID: actor, Kind: MsgReply, Body: req.Message, Attachments: []int64{}}); err != nil {
				return err
			}
		}
		if err := event(ctx, q, id, actor, false, Event{Type: EventClosed, Status: req.Verdict}); err != nil {
			return err
		}
		if req.RewardPoints > 0 {
			ledgerID, err := s.Rewarder.Grant(ctx, tx, f.UserID, f.ID, req.RewardPoints)
			if err != nil {
				return fmt.Errorf("grant reward: %w", err)
			}
			p.set("reward_points", req.RewardPoints).set("reward_ledger_id", ledgerID)
			if err := event(ctx, q, id, actor, false, Event{Type: EventReward, Points: req.RewardPoints}); err != nil {
				return err
			}
		}
		return q.Apply(ctx, id, p)
	})
}

// BatchInput 批量操作：定级、加标签、批量不处理
type BatchInput struct {
	IDs      []int64   `json:"ids"`
	Severity *Severity `json:"severity"`
	AddTag   string    `json:"add_tag"`
	Decline  string    `json:"decline_reason"` // 非空 = 批量结案为不处理
}

// BatchResult 成功条数和失败原因（按 ID）
type BatchResult struct {
	Done   int              `json:"done"`
	Failed map[int64]string `json:"failed,omitempty"`
}

func (s *Service) Batch(ctx context.Context, adminID int64, in BatchInput) (*BatchResult, error) {
	if len(in.IDs) == 0 || len(in.IDs) > 100 {
		return nil, inputErr("请选择 1-100 条反馈")
	}
	res := &BatchResult{Failed: map[int64]string{}}
	for _, id := range in.IDs {
		err := s.batchOne(ctx, adminID, id, in)
		if err != nil {
			res.Failed[id] = errMessage(err)
			continue
		}
		res.Done++
	}
	return res, nil
}

func (s *Service) batchOne(ctx context.Context, adminID, id int64, in BatchInput) error {
	if in.Severity != nil || in.AddTag != "" {
		up := UpdateInput{Severity: in.Severity}
		if in.AddTag != "" {
			f, err := s.Repo.Get(ctx, id)
			if err != nil {
				return err
			}
			tags := append(append([]string{}, f.Tags...), in.AddTag)
			up.Tags = &tags
		}
		if err := s.AdminUpdate(ctx, adminID, id, up); err != nil {
			return err
		}
	}
	if in.Decline != "" {
		return s.Close(ctx, adminID, id, CloseRequest{Verdict: StatusDeclined, Reason: in.Decline})
	}
	return nil
}

func errMessage(err error) string {
	var ie *InputError
	switch {
	case errors.As(err, &ie):
		return ie.Msg
	case errors.Is(err, ErrClosed):
		return "已结案"
	case errors.Is(err, ErrNotFound):
		return "不存在"
	}
	return "操作失败"
}

// AutoCloseExpired 已回复后用户 N 天没有回应 → 不处理。返回结案条数
func (s *Service) AutoCloseExpired(ctx context.Context) (int, error) {
	cfg, err := s.Config(ctx)
	if err != nil || cfg.AskTimeoutDays <= 0 {
		return 0, err
	}
	before := s.Now().AddDate(0, 0, -cfg.AskTimeoutDays)
	reason := fmt.Sprintf("%d 天内没有收到回复，自动结案", cfg.AskTimeoutDays)
	closed := 0
	for {
		ids, err := s.Repo.ExpiredAwaiting(ctx, before, 100)
		if err != nil || len(ids) == 0 {
			return closed, err
		}
		for _, id := range ids {
			err := s.mutate(ctx, id, func(q *queries, _ *sql.Tx, f *Feedback) error {
				if f.Status != StatusAwaiting || f.AskedAt == nil || !f.AskedAt.Before(before) {
					return nil
				}
				if err := event(ctx, q, id, nil, false, Event{Type: EventAutoClose, Status: StatusDeclined}); err != nil {
					return err
				}
				closed++
				return q.Apply(ctx, id, (&patch{}).set("status", StatusDeclined).set("close_reason", reason).set("user_unread", true).
					raw("closed_at = NOW()").raw("asked_at = NULL").raw("last_activity_at = NOW()"))
			})
			if err != nil {
				return closed, err
			}
		}
		if len(ids) < 100 {
			return closed, nil
		}
	}
}
