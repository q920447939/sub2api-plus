// Package checkin — [plus] 管理端：签到记录、签到统计；以及向积分模块注册"去签到"获取方式
package checkin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/plus/points"
)

// EarnWay 积分获取方式：签到开启且今天还没签时可用
func (s *Service) EarnWay(ctx context.Context, userID int64) (*points.EarnWay, error) {
	cfg, _, err := s.Config(ctx)
	if err != nil || !cfg.Enabled {
		return nil, err
	}
	log, err := s.repo.TodayLog(ctx, userID)
	if err != nil || log != nil {
		return nil, err
	}
	streak, err := s.repo.YesterdayStreak(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &points.EarnWay{
		Key: points.ReasonCheckin, Label: "去签到",
		Hint:   fmt.Sprintf("今日签到可领 %d 积分", cfg.PointsForDayN(streak+1)),
		Action: "tab:checkin", Order: 10,
	}, nil
}

// AdminLog 管理端签到记录
type AdminLog struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	Email         string    `json:"email"`
	CheckedDate   string    `json:"checked_date"`
	Streak        int       `json:"streak"`
	PointsGranted int64     `json:"points_granted"`
	CreatedAt     time.Time `json:"created_at"`
}

// LogQuery 签到记录筛选
type LogQuery struct {
	Date     string // YYYY-MM-DD，空=全部
	Keyword  string // 邮箱/用户名模糊，纯数字匹配用户 ID
	Page     int
	PageSize int
}

func normPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return page, size
}

// AdminLogs 签到记录分页
func (s *Service) AdminLogs(ctx context.Context, q LogQuery) ([]AdminLog, int64, int, int, error) {
	page, size := normPage(q.Page, q.PageSize)
	where := ` WHERE TRUE`
	var args []any
	if q.Date != "" {
		if _, err := time.Parse("2006-01-02", q.Date); err == nil {
			args = append(args, q.Date)
			where += fmt.Sprintf(" AND l.checked_date = $%d", len(args))
		}
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		if id, err := strconv.ParseInt(kw, 10, 64); err == nil {
			args = append(args, id)
			where += fmt.Sprintf(" AND l.user_id = $%d", len(args))
		} else {
			args = append(args, "%"+strings.ToLower(kw)+"%")
			where += fmt.Sprintf(" AND (LOWER(u.email) LIKE $%d OR LOWER(u.username) LIKE $%d)", len(args), len(args))
		}
	}
	from := ` FROM plus_checkin_logs l JOIN users u ON u.id = l.user_id` + where
	var total int64
	if err := s.repo.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, 0, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := s.repo.db.QueryContext(ctx,
		`SELECT l.id, l.user_id, u.email, l.checked_date, l.streak, l.points_granted, l.created_at`+from+
			fmt.Sprintf(` ORDER BY l.checked_date DESC, l.created_at DESC, l.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer func() { _ = rows.Close() }()
	list := []AdminLog{}
	for rows.Next() {
		var it AdminLog
		var d time.Time
		if err := rows.Scan(&it.ID, &it.UserID, &it.Email, &d, &it.Streak, &it.PointsGranted, &it.CreatedAt); err != nil {
			return nil, 0, 0, 0, err
		}
		it.CheckedDate = d.Format("2006-01-02")
		list = append(list, it)
	}
	return list, total, page, size, rows.Err()
}

// DayStat 某天的签到汇总
type DayStat struct {
	Date   string `json:"date"`
	Users  int64  `json:"users"`
	Points int64  `json:"points"`
}

// AdminStats 签到概况
type AdminStats struct {
	Today     DayStat `json:"today"`
	Yesterday DayStat `json:"yesterday"`
	// StreakDist 今日签到用户的连签分布：1-2 天 / 3-6 天 / 7 天及以上
	StreakDist []StreakBucket `json:"streak_dist"`
	Trend      []DayStat      `json:"trend"` // 近 days 天，含今天，按日期升序，缺失日补 0
}

type StreakBucket struct {
	Label string `json:"label"`
	Users int64  `json:"users"`
}

// AdminStats 统计近 days 天（含今天）的签到数据
func (s *Service) AdminStats(ctx context.Context, days int) (*AdminStats, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	today := timezone.StartOfDay(time.Now().In(timezone.Location()))
	start := today.AddDate(0, 0, -(days - 1))
	if days < 2 {
		start = today.AddDate(0, 0, -1)
	}
	rows, err := s.repo.db.QueryContext(ctx,
		`SELECT checked_date, COUNT(*), COALESCE(SUM(points_granted), 0)
		 FROM plus_checkin_logs WHERE checked_date >= $1 GROUP BY checked_date`, start.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byDate := map[string]DayStat{}
	for rows.Next() {
		var d time.Time
		var st DayStat
		if err := rows.Scan(&d, &st.Users, &st.Points); err != nil {
			return nil, err
		}
		st.Date = d.Format("2006-01-02")
		byDate[st.Date] = st
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	day := func(t time.Time) DayStat {
		k := t.Format("2006-01-02")
		if st, ok := byDate[k]; ok {
			return st
		}
		return DayStat{Date: k}
	}
	out := &AdminStats{Today: day(today), Yesterday: day(today.AddDate(0, 0, -1)), Trend: []DayStat{}}
	for i := days - 1; i >= 0; i-- {
		out.Trend = append(out.Trend, day(today.AddDate(0, 0, -i)))
	}

	var b1, b2, b3 int64
	if err := s.repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE streak < 3), COUNT(*) FILTER (WHERE streak BETWEEN 3 AND 6), COUNT(*) FILTER (WHERE streak >= 7)
		 FROM plus_checkin_logs WHERE checked_date = $1`, today.Format("2006-01-02")).Scan(&b1, &b2, &b3); err != nil {
		return nil, err
	}
	out.StreakDist = []StreakBucket{{"连签 1-2 天", b1}, {"连签 3-6 天", b2}, {"连签 7 天及以上", b3}}
	return out, nil
}
