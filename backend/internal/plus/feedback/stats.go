// Package feedback — [plus] 分析看板：场景、严重度、用户、趋势、建议池
package feedback

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// Stats 看板数据；除建议池外都按时间范围（提交时间）统计
type Stats struct {
	Days     int             `json:"days"`
	Overview Overview        `json:"overview"`
	Scenario []ScenarioStat  `json:"scenarios"`
	Severity []SeverityCell  `json:"severity"`
	Trend    []TrendPoint    `json:"trend"`
	Users    []UserStat      `json:"users"`
	Pool     []SuggestionRow `json:"pool"`
}

type Overview struct {
	Total         int64   `json:"total"`
	Issues        int64   `json:"issues"`
	Suggestions   int64   `json:"suggestions"`
	Pending       int64   `json:"pending"`  // 当前积压，不受时间范围影响
	Awaiting      int64   `json:"awaiting"` // 同上
	Resolved      int64   `json:"resolved"`
	Adopted       int64   `json:"adopted"`
	AvgCloseHours float64 `json:"avg_close_hours"`
	AutoClosed    int64   `json:"auto_closed"`
	RewardPoints  int64   `json:"reward_points"`
}

type ScenarioStat struct {
	Kind     Kind   `json:"kind"`
	Scenario string `json:"scenario"`
	Total    int64  `json:"total"`
	Open     int64  `json:"open"`
	Accepted int64  `json:"accepted"` // 已解决 / 已采纳
	Declined int64  `json:"declined"`
	Fixed    int64  `json:"reclassified"` // 管理员修正过场景的条数，用来判断场景文案是否让人看不懂
}

type SeverityCell struct {
	Scenario string   `json:"scenario"`
	Severity Severity `json:"severity"` // 空 = 未定级
	Count    int64    `json:"count"`
	Open     int64    `json:"open"`
}

type TrendPoint struct {
	Date        string `json:"date"`
	Issues      int64  `json:"issues"`
	Suggestions int64  `json:"suggestions"`
	Closed      int64  `json:"closed"`
}

type UserStat struct {
	UserID      int64            `json:"user_id"`
	Email       string           `json:"email"`
	Total       int64            `json:"total"`
	Issues      int64            `json:"issues"`
	Suggestions int64            `json:"suggestions"`
	Accepted    int64            `json:"accepted"`
	Invalid     int64            `json:"invalid"` // 不处理 + 定级为无效
	Reward      int64            `json:"reward"`
	Scenarios   map[string]int64 `json:"scenarios"`
}

type SuggestionRow struct {
	ID       int64     `json:"id"`
	Scenario string    `json:"scenario"`
	Summary  string    `json:"summary"`
	Status   Status    `json:"status"`
	Severity Severity  `json:"severity,omitempty"`
	Impact   int64     `json:"impact"`
	At       time.Time `json:"created_at"`
}

func (s *Service) Stats(ctx context.Context, days int) (*Stats, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	loc := timezone.Location()
	today := timezone.StartOfDay(s.Now().In(loc))
	since := today.AddDate(0, 0, -(days - 1))
	db := s.Repo.db
	st := &Stats{Days: days}

	o := &st.Overview
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE kind = 'issue'), COUNT(*) FILTER (WHERE kind = 'suggestion'),
		        COUNT(*) FILTER (WHERE status = 'resolved'), COUNT(*) FILTER (WHERE status = 'adopted'),
		        COALESCE(AVG(EXTRACT(EPOCH FROM closed_at - created_at) / 3600) FILTER (WHERE closed_at IS NOT NULL), 0),
		        COALESCE(SUM(reward_points), 0),
		        (SELECT COUNT(*) FROM plus_feedback WHERE status = 'pending'),
		        (SELECT COUNT(*) FROM plus_feedback WHERE status = 'awaiting'),
		        (SELECT COUNT(DISTINCT m.feedback_id) FROM plus_feedback_messages m JOIN plus_feedback f2 ON f2.id = m.feedback_id
		          WHERE m.kind = 'event' AND m.event->>'type' = 'auto_close' AND f2.created_at >= $1)
		 FROM plus_feedback WHERE created_at >= $1`, since).
		Scan(&o.Total, &o.Issues, &o.Suggestions, &o.Resolved, &o.Adopted, &o.AvgCloseHours, &o.RewardPoints, &o.Pending, &o.Awaiting, &o.AutoClosed); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx,
		`SELECT kind, scenario, COUNT(*),
		        COUNT(*) FILTER (WHERE status IN ('pending','awaiting')),
		        COUNT(*) FILTER (WHERE status IN ('resolved','adopted')),
		        COUNT(*) FILTER (WHERE status = 'declined'),
		        COUNT(*) FILTER (WHERE scenario <> scenario_user)
		 FROM plus_feedback WHERE created_at >= $1 GROUP BY kind, scenario ORDER BY COUNT(*) DESC`, since)
	if err != nil {
		return nil, err
	}
	st.Scenario = []ScenarioStat{}
	if err := scanAll(rows, func() error {
		var x ScenarioStat
		if err := rows.Scan(&x.Kind, &x.Scenario, &x.Total, &x.Open, &x.Accepted, &x.Declined, &x.Fixed); err != nil {
			return err
		}
		st.Scenario = append(st.Scenario, x)
		return nil
	}); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx,
		`SELECT scenario, COALESCE(severity, ''), COUNT(*), COUNT(*) FILTER (WHERE status IN ('pending','awaiting'))
		 FROM plus_feedback WHERE created_at >= $1 AND status <> 'duplicate' GROUP BY scenario, severity`, since)
	if err != nil {
		return nil, err
	}
	st.Severity = []SeverityCell{}
	if err := scanAll(rows, func() error {
		var x SeverityCell
		if err := rows.Scan(&x.Scenario, &x.Severity, &x.Count, &x.Open); err != nil {
			return err
		}
		st.Severity = append(st.Severity, x)
		return nil
	}); err != nil {
		return nil, err
	}

	if st.Trend, err = s.trend(ctx, since, days, loc); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx,
		`SELECT f.user_id, u.email, COUNT(*),
		        COUNT(*) FILTER (WHERE f.kind = 'issue'), COUNT(*) FILTER (WHERE f.kind = 'suggestion'),
		        COUNT(*) FILTER (WHERE f.status IN ('resolved','adopted')),
		        COUNT(*) FILTER (WHERE f.status = 'declined' OR f.severity = 'invalid'),
		        COALESCE(SUM(f.reward_points), 0),
		        (SELECT jsonb_object_agg(scenario, n) FROM (
		            SELECT scenario, COUNT(*) AS n FROM plus_feedback x WHERE x.user_id = f.user_id AND x.created_at >= $1 GROUP BY scenario) s)
		 FROM plus_feedback f JOIN users u ON u.id = f.user_id
		 WHERE f.created_at >= $1 GROUP BY f.user_id, u.email ORDER BY COUNT(*) DESC, f.user_id LIMIT 50`, since)
	if err != nil {
		return nil, err
	}
	st.Users = []UserStat{}
	if err := scanAll(rows, func() error {
		var (
			x   UserStat
			mix []byte
		)
		if err := rows.Scan(&x.UserID, &x.Email, &x.Total, &x.Issues, &x.Suggestions, &x.Accepted, &x.Invalid, &x.Reward, &mix); err != nil {
			return err
		}
		x.Scenarios = map[string]int64{}
		_ = json.Unmarshal(mix, &x.Scenarios)
		st.Users = append(st.Users, x)
		return nil
	}); err != nil {
		return nil, err
	}

	// 建议池：所有未结案或已采纳的主建议，按影响人数、严重度排序（不受时间范围限制，它就是产品待办）
	rows, err = db.QueryContext(ctx,
		`SELECT f.id, f.scenario, `+summarySQL+`, f.status, COALESCE(f.severity, ''),
		        1 + (SELECT COUNT(*) FROM plus_feedback m WHERE m.merged_into_id = f.id) AS impact, f.created_at
		 FROM plus_feedback f
		 WHERE f.kind = 'suggestion' AND f.status IN ('pending','awaiting','adopted')
		 ORDER BY impact DESC,
		          CASE f.severity WHEN 'critical' THEN 0 WHEN 'major' THEN 1 WHEN 'minor' THEN 2 WHEN 'invalid' THEN 4 ELSE 3 END,
		          f.created_at DESC
		 LIMIT 30`)
	if err != nil {
		return nil, err
	}
	st.Pool = []SuggestionRow{}
	if err := scanAll(rows, func() error {
		var x SuggestionRow
		if err := rows.Scan(&x.ID, &x.Scenario, &x.Summary, &x.Status, &x.Severity, &x.Impact, &x.At); err != nil {
			return err
		}
		st.Pool = append(st.Pool, x)
		return nil
	}); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Service) trend(ctx context.Context, since time.Time, days int, loc *time.Location) ([]TrendPoint, error) {
	tz := loc.String()
	rows, err := s.Repo.db.QueryContext(ctx,
		`SELECT d, SUM(issues), SUM(suggestions), SUM(closed) FROM (
		    SELECT (created_at AT TIME ZONE $2::text)::date AS d,
		           COUNT(*) FILTER (WHERE kind = 'issue') AS issues, COUNT(*) FILTER (WHERE kind = 'suggestion') AS suggestions, 0 AS closed
		    FROM plus_feedback WHERE created_at >= $1 GROUP BY 1
		    UNION ALL
		    SELECT (closed_at AT TIME ZONE $2::text)::date, 0, 0, COUNT(*)
		    FROM plus_feedback WHERE closed_at >= $1 GROUP BY 1
		 ) t GROUP BY d`, since, tz)
	if err != nil {
		return nil, err
	}
	byDate := map[string]TrendPoint{}
	if err := scanAll(rows, func() error {
		var (
			d time.Time
			p TrendPoint
		)
		if err := rows.Scan(&d, &p.Issues, &p.Suggestions, &p.Closed); err != nil {
			return err
		}
		p.Date = d.Format("2006-01-02")
		byDate[p.Date] = p
		return nil
	}); err != nil {
		return nil, err
	}
	out := make([]TrendPoint, 0, days)
	for i := 0; i < days; i++ {
		k := since.AddDate(0, 0, i).Format("2006-01-02")
		p, ok := byDate[k]
		if !ok {
			p = TrendPoint{Date: k}
		}
		out = append(out, p)
	}
	return out, nil
}

type rowsIter interface {
	Next() bool
	Err() error
	Close() error
}

func scanAll(rows rowsIter, fn func() error) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := fn(); err != nil {
			return err
		}
	}
	return rows.Err()
}
