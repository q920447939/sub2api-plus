package dispatch

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// AdjustService 调度调整：调度台里就地调整，执行层（hook）只认这里的「当前调整」。
// 人工、采纳建议、以后的程序自动和 AI 都走同一个入口，后来的覆盖前面的。
type AdjustService struct {
	Boards  BoardLoader
	Store   AdjustStore
	Events  EventStore
	Traffic TrafficStore
	DB      *sql.DB
	Audit   Auditor
	Reload  func() // 刷新本实例的钩子规则
}

// AdjustRequest 一次调整
type AdjustRequest struct {
	GroupID   int64   `json:"group_id"`
	AccountID int64   `json:"account_id"`
	Action    string  `json:"action"` // normal（恢复）| pause | factor
	Factor    float64 `json:"factor"`
	Reason    string  `json:"reason"`
	Source    string  `json:"source"` // manual | suggestion
}

func (r *AdjustRequest) normalize(requireReason bool) error {
	r.Reason = strings.TrimSpace(r.Reason)
	switch r.Action {
	case "normal":
		r.Factor = 1
	case "pause":
		r.Factor = 0
	case "factor":
		r.Factor = math.Round(r.Factor*100) / 100
		if r.Factor <= 0 || r.Factor >= 1 {
			return fmt.Errorf("%w：系数要在 0 到 1 之间（不含 0 和 1）；想多给某个账号流量，就降低其他账号", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w：动作只能是 normal / pause / factor", ErrInvalid)
	}
	if requireReason && r.Action != "normal" && len([]rune(r.Reason)) < 2 {
		return fmt.Errorf("%w：请写一下调整原因（以后回看时知道为什么这么调）", ErrInvalid)
	}
	if r.Source != "suggestion" {
		r.Source = "manual"
	}
	return nil
}

// Preview 调整前后的份额预测
type Preview struct {
	Basis     string            `json:"basis"`
	Before    map[int64]float64 `json:"before"`
	Predicted map[int64]float64 `json:"predicted"`
	Current   map[int64]string  `json:"current"` // 各账号当前的调整
	Affected  []string          `json:"affected"`
	Warning   string            `json:"warning,omitempty"`
	Mode      string            `json:"mode"`
}

func (s *AdjustService) preview(b *Board, req AdjustRequest) (Preview, error) {
	cat := b.Snap.Catalog
	g := cat.Groups[req.GroupID]
	if g == nil {
		return Preview{}, ErrNotFound
	}
	if !contains(int64Strings(g.Accounts), itoa(req.AccountID)) {
		return Preview{}, fmt.Errorf("%w：这个账号不在该分组", ErrInvalid)
	}
	before, basis, _ := b.HourShares(req.GroupID)
	full := map[int64]float64{}
	factors := map[int64]float64{}
	cur := map[int64]string{}
	for _, id := range g.Accounts {
		full[id] = round1(before[id])
		if adj, ok := b.Adjust[[2]int64{req.GroupID, id}]; ok {
			cur[id] = adjustText(adj.Action, adj.Factor)
		}
	}
	// 「之前」是实际份额（已经包含了现有调整的效果），所以只按这次改动的账号换算
	old := 1.0
	if adj, ok := b.Adjust[[2]int64{req.GroupID, req.AccountID}]; ok {
		old = adj.Factor
		if adj.Action == "pause" {
			old = 0
		}
	}
	switch {
	case old > 0:
		factors[req.AccountID] = req.Factor / old
	case req.Factor > 0:
		// 原来是暂停（实际份额 ≈ 0），恢复后的份额没法从实际数据推算，按同组平均估
		n := float64(len(g.Accounts))
		full[req.AccountID] = 100 / n
	}
	p := Preview{Basis: basis, Before: full, Predicted: PredictShares(full, factors), Current: cur, Mode: b.Setup.Config.Mode}
	if a := cat.Accounts[req.AccountID]; a != nil && len(a.Groups) > 1 {
		p.Affected = cat.GroupNames(a.Groups)
	}
	// 安全阀：同组至少留一个没被暂停、正在接客的账号
	alive := 0
	for _, id := range g.Accounts {
		a := cat.Accounts[id]
		if a == nil || !a.Serving() {
			continue
		}
		act := ""
		if adj, ok := b.Adjust[[2]int64{req.GroupID, id}]; ok {
			act = adj.Action
		}
		if id == req.AccountID {
			act = req.Action
		}
		if act != "pause" {
			alive++
		}
	}
	if alive == 0 {
		return p, fmt.Errorf("%w：这样调整后本组没有可用的账号了，至少要留一个没被暂停的账号", ErrInvalid)
	}
	if b.Setup.Config.Mode != ModeManual {
		p.Warning = "现在是「" + modeNames[b.Setup.Config.Mode] + "」档：调整会保存，但要切到「人工调整」档才会真正影响调度"
	}
	return p, nil
}

func int64Strings(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = itoa(id)
	}
	return out
}

func (s *AdjustService) Preview(ctx context.Context, req AdjustRequest) (Preview, error) {
	if err := req.normalize(false); err != nil {
		return Preview{}, err
	}
	b, err := s.Boards.Load(ctx)
	if err != nil {
		return Preview{}, err
	}
	return s.preview(b, req)
}

// Apply 调整（或恢复）
func (s *AdjustService) Apply(ctx context.Context, actor Actor, req AdjustRequest) (*AdjustmentLog, error) {
	if err := req.normalize(true); err != nil {
		return nil, err
	}
	b, err := s.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	pv, err := s.preview(b, req)
	if err != nil {
		return nil, err
	}
	from, fromF := "normal", 1.0
	if adj, ok := b.Adjust[[2]int64{req.GroupID, req.AccountID}]; ok {
		from, fromF = adj.Action, adj.Factor
	}
	if from == req.Action && fromF == req.Factor {
		return nil, fmt.Errorf("%w：和当前状态一样，不需要调整", ErrInvalid)
	}
	res := b.DimResults(req.GroupID, req.AccountID, "")
	snapshot := map[string]any{
		"basis": pv.Basis, "before": pv.Before, "predicted": pv.Predicted, "mode": b.Setup.Config.Mode,
		"one_line": b.OneLine(res), "worst": b.Worst(res), "affected": pv.Affected,
	}
	for _, r := range b.Simulate(req.GroupID).Rows {
		if r.AccountID == req.AccountID {
			snapshot["suggest"] = r
		}
	}
	email := actor.Email
	if email == "" {
		email = s.Audit.Store.OperatorEmail(ctx, actor.ID)
	}
	op := actor.ID
	log := AdjustmentLog{GroupID: req.GroupID, AccountID: req.AccountID, FromAction: from, FromFactor: fromF, ToAction: req.Action,
		ToFactor: req.Factor, Source: req.Source, Reason: req.Reason, Snapshot: snapshot, OperatorID: &op, Operator: email}
	id, err := s.Store.ApplyAdjustment(ctx, Adjustment{GroupID: req.GroupID, AccountID: req.AccountID, Action: req.Action,
		Factor: req.Factor, Source: req.Source, Reason: req.Reason, CreatedBy: &op}, log)
	if err != nil {
		return nil, err
	}
	log.ID, log.CreatedAt = id, time.Now()
	cat := b.Snap.Catalog
	summary := fmt.Sprintf("%s · %s：%s → %s", cat.Groups[req.GroupID].Name, cat.Accounts[req.AccountID].Name,
		adjustText(from, fromF), adjustText(req.Action, req.Factor))
	if req.Reason != "" {
		summary += "（" + req.Reason + "）"
	}
	s.Audit.Record(ctx, Actor{ID: actor.ID, Email: email}, "adjust", "adjustment", fmt.Sprintf("%d:%d", req.GroupID, req.AccountID), summary,
		map[string]any{"action": from, "factor": fromF}, map[string]any{"action": req.Action, "factor": req.Factor, "log_id": id})
	if s.Reload != nil {
		s.Reload()
	}
	return &log, nil
}

// Active 生效中的调整 + 「依据已消失，建议恢复」提示
func (s *AdjustService) Active(ctx context.Context) ([]map[string]any, error) {
	b, err := s.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	hint := time.Duration(b.Setup.Config.RestoreHint) * time.Hour
	var out []map[string]any
	keys := make([][2]int64, 0, len(b.Adjust))
	for k := range b.Adjust {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return b.Adjust[keys[i]].CreatedAt.After(b.Adjust[keys[j]].CreatedAt) })
	cat := b.Snap.Catalog
	for _, k := range keys {
		a := b.Adjust[k]
		row := map[string]any{"adjustment": a, "text": adjustText(a.Action, a.Factor), "duration": since(a.CreatedAt)}
		if g := cat.Groups[a.GroupID]; g != nil {
			row["group"] = g.Name
		}
		if acc := cat.Accounts[a.AccountID]; acc != nil {
			row["account"] = acc.Name
		}
		res := b.DimResults(a.GroupID, a.AccountID, "")
		row["worst"], row["one_line"] = b.Worst(res), b.OneLine(res)
		row["share"] = round1(b.Shares(a.GroupID)[a.AccountID])
		openProblem := false
		for _, e := range b.Open {
			if e.Kind == "dim" && e.GroupID == a.GroupID && e.AccountID == a.AccountID && e.State != EventFalse {
				openProblem = true
			}
		}
		if !openProblem && b.Worst(res) == LevelOK && time.Since(a.CreatedAt) > hint {
			last, err := s.Events.LastResolved(ctx, a.GroupID, a.AccountID)
			if err == nil && (last == nil || time.Since(*last) > hint) {
				row["restore_hint"] = fmt.Sprintf("各维度已经恢复正常满 %d 小时，建议恢复", b.Setup.Config.RestoreHint)
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// Logs 调整历史
func (s *AdjustService) Logs(ctx context.Context, groupID, accountID int64) ([]map[string]any, error) {
	b, err := s.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	logs, err := s.Store.ListAdjustmentLogs(ctx, groupID, accountID, 200)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		row := map[string]any{"log": l, "from": adjustText(l.FromAction, l.FromFactor), "to": adjustText(l.ToAction, l.ToFactor)}
		if g := cat.Groups[l.GroupID]; g != nil {
			row["group"] = g.Name
		}
		if a := cat.Accounts[l.AccountID]; a != nil {
			row["account"] = a.Name
		}
		out = append(out, row)
	}
	return out, nil
}

// Badges 账号列表上的小标签：账号 → 在哪些分组被调整
func (s *AdjustService) Badges(ctx context.Context) (map[int64][]map[string]any, error) {
	adj, err := s.Store.ListAdjustments(ctx)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name FROM groups WHERE deleted_at IS NULL`)
	if err == nil {
		for rows.Next() {
			var (
				id   int64
				name string
			)
			if rows.Scan(&id, &name) == nil {
				names[id] = name
			}
		}
		_ = rows.Close()
	}
	out := map[int64][]map[string]any{}
	for _, a := range adj {
		out[a.AccountID] = append(out[a.AccountID], map[string]any{"group_id": a.GroupID, "group": names[a.GroupID],
			"action": a.Action, "factor": a.Factor, "text": adjustText(a.Action, a.Factor)})
	}
	return out, nil
}

// ---------- 效果对比 ----------

// Effect 一次调整的效果：流量去哪了、分组整体有没有变好、接收方扛不扛得住、钩子介入了多少次
func (s *AdjustService) Effect(ctx context.Context, logID int64) (map[string]any, error) {
	l, err := s.Store.GetAdjustmentLog(ctx, logID)
	if err != nil {
		return nil, err
	}
	b, err := s.Boards.Load(ctx)
	if err != nil {
		return nil, err
	}
	cat := b.Snap.Catalog
	g := cat.Groups[l.GroupID]
	if g == nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	t0 := l.CreatedAt
	start := t0.Add(-3 * time.Hour)
	end := t0.Add(6 * time.Hour)
	if end.After(now) {
		end = now
	}
	// ① 流量去向：10 分钟一格的份额
	series, err := s.shareSeries(ctx, l.GroupID, start, end, t0)
	if err != nil {
		return nil, err
	}
	// ② 分组整体：调整前 1 小时 vs 调整后（最多 1 小时）
	afterEnd := t0.Add(time.Hour)
	if afterEnd.After(now) {
		afterEnd = now
	}
	beforeAgg, err := s.Traffic.Aggregate(ctx, t0.Add(-time.Hour), t0, false)
	if err != nil {
		return nil, err
	}
	afterAgg, err := s.Traffic.Aggregate(ctx, t0, afterEnd, false)
	if err != nil {
		return nil, err
	}
	sumGroup := func(data map[TKey]map[int64]Agg) (Agg, map[int64]Agg) {
		var total Agg
		per := map[int64]Agg{}
		for k, m := range data {
			if k.Group != l.GroupID {
				continue
			}
			for _, a := range m {
				total = total.Add(a)
				per[k.Account] = per[k.Account].Add(a)
			}
		}
		return total, per
	}
	bt, bper := sumGroup(beforeAgg)
	at, aper := sumGroup(afterAgg)
	costOf := func(per map[int64]Agg) *float64 {
		var w, n float64
		for id, a := range per {
			if acc := cat.Accounts[id]; acc != nil {
				w += acc.Rate * float64(a.Total())
				n += float64(a.Total())
			}
		}
		if n == 0 {
			return nil
		}
		return fptr(w / n)
	}
	type cmp struct {
		Key    string   `json:"key"`
		Name   string   `json:"name"`
		Unit   string   `json:"unit"`
		Better string   `json:"better"`
		Before *float64 `json:"before"`
		After  *float64 `json:"after"`
		Trend  string   `json:"trend"` // better | worse | flat
	}
	trend := func(better string, b, a *float64, tol float64) string {
		if b == nil || a == nil {
			return ""
		}
		d := *a - *b
		if math.Abs(d) <= tol*math.Max(math.Abs(*b), 1e-9) {
			return "flat"
		}
		if (d < 0) == (better == "low") {
			return "better"
		}
		return "worse"
	}
	val := func(m Metric, a Agg) *float64 {
		if v, ok := m.Value(a); ok {
			return fptr(v)
		}
		return nil
	}
	var group []cmp
	for _, key := range []string{"err", "ttft", "cache", "r429"} {
		m, _ := metricByKey(key)
		bv, av := val(m, bt), val(m, at)
		group = append(group, cmp{Key: key, Name: m.Name, Unit: m.Unit, Better: m.Better, Before: bv, After: av, Trend: trend(m.Better, bv, av, 0.05)})
	}
	bc, ac := costOf(bper), costOf(aper)
	group = append(group, cmp{Key: "cost", Name: "成本（倍率加权）", Better: "low", Before: bc, After: ac, Trend: trend("low", bc, ac, 0.02)})

	// ③ 接收方
	shareOf := func(per map[int64]Agg) map[int64]float64 {
		var total int64
		for _, a := range per {
			total += a.Total()
		}
		out := map[int64]float64{}
		for id, a := range per {
			if total > 0 {
				out[id] = round1(float64(a.Total()) * 100 / float64(total))
			}
		}
		return out
	}
	bs, as := shareOf(bper), shareOf(aper)
	// 窗口内没有请求时份额未知（返回 null，界面显示「—」），不能当成 0%
	shareVal := func(m map[int64]float64, id int64) any {
		if len(m) == 0 {
			return nil
		}
		return m[id]
	}
	ttft, _ := metricByKey("ttft")
	r429, _ := metricByKey("r429")
	var receivers []map[string]any
	warnRecv := false
	for _, id := range g.Accounts {
		if id == l.AccountID {
			continue
		}
		acc := cat.Accounts[id]
		if acc == nil {
			continue
		}
		res := b.DimResults(l.GroupID, id, "")
		worst := b.Worst(res)
		if worst.Problem() {
			warnRecv = true
		}
		receivers = append(receivers, map[string]any{"account_id": id, "name": acc.Name, "share_before": shareVal(bs, id), "share_after": shareVal(as, id),
			"ttft_before": val(ttft, bper[id]), "ttft_after": val(ttft, aper[id]), "r429_before": val(r429, bper[id]), "r429_after": val(r429, aper[id]),
			"worst": worst, "one_line": b.OneLine(res)})
	}
	// ④ 钩子介入次数
	hits, err := s.Store.HookHits(ctx, l.GroupID, t0)
	if err != nil {
		return nil, err
	}
	// 结论
	target := cat.Accounts[l.AccountID]
	predicted := 0.0
	if p, ok := l.Snapshot["predicted"].(map[string]any); ok {
		if v, ok := p[itoa(l.AccountID)].(float64); ok {
			predicted = v
		}
	}
	var conclusion []string
	if at.Total() == 0 {
		conclusion = append(conclusion, "调整后还没有新的请求，稍后再看")
	} else {
		conclusion = append(conclusion, fmt.Sprintf("%s 的份额 %.0f%% → %.0f%%（预计 %.0f%%）", target.Name, bs[l.AccountID], as[l.AccountID], predicted))
		for _, c := range group {
			if c.Key == "err" || c.Key == "ttft" {
				switch c.Trend {
				case "better":
					conclusion = append(conclusion, "分组"+c.Name+"好转")
				case "worse":
					conclusion = append(conclusion, "分组"+c.Name+"变差")
				}
			}
		}
		if warnRecv {
			conclusion = append(conclusion, "有接收流量的账号出现可疑 / 异常，建议回看是否回调")
		} else {
			conclusion = append(conclusion, "接收流量的账号都正常")
		}
		if l.ToAction == "factor" && as[l.AccountID] > predicted+5 {
			conclusion = append(conclusion, "份额还没降到预计值：降份额不打断已经粘住的会话，这部分流量要等会话自然结束才会转走")
		}
	}
	mode := b.Setup.Config.Mode
	if mode != ModeManual {
		conclusion = append([]string{"注意：现在不是「人工调整」档，这次调整没有影响实际调度"}, conclusion...)
	}
	names := map[int64]string{}
	for _, id := range g.Accounts {
		names[id] = cat.Accounts[id].Name
	}
	return map[string]any{
		"log": l, "group": g, "account": target, "names": names, "series": series, "t0": t0,
		"group_compare": group, "requests_before": bt.Total(), "requests_after": at.Total(), "after_minutes": int(afterEnd.Sub(t0).Minutes()),
		"receivers": receivers, "hook_hits": hits[l.AccountID], "hook_fallback": hits[0], "conclusion": strings.Join(conclusion, "；"),
		"mode": mode,
	}, nil
}

// shareSeries 分组里各账号每 10 分钟的请求占比
func (s *AdjustService) shareSeries(ctx context.Context, groupID int64, from, to, origin time.Time) (map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT date_bin('10 minutes', created_at, $3) AS b, account_id, COUNT(*)
		FROM usage_logs WHERE group_id = $1 AND created_at >= $2 AND created_at < $4 GROUP BY 1, 2 ORDER BY 1`, groupID, from, origin, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	per := map[int64]map[int64]int64{}
	totals := map[int64]int64{}
	for rows.Next() {
		var (
			b  time.Time
			id int64
			n  int64
		)
		if err := rows.Scan(&b, &id, &n); err != nil {
			return nil, err
		}
		if per[id] == nil {
			per[id] = map[int64]int64{}
		}
		per[id][b.Unix()] = n
		totals[b.Unix()] += n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var labels []time.Time
	for t := origin; !t.Before(from); t = t.Add(-10 * time.Minute) {
		labels = append([]time.Time{t}, labels...)
	}
	for t := origin.Add(10 * time.Minute); t.Before(to); t = t.Add(10 * time.Minute) {
		labels = append(labels, t)
	}
	lines := map[int64][]*float64{}
	for id, m := range per {
		vals := make([]*float64, len(labels))
		for i, t := range labels {
			if tot := totals[t.Unix()]; tot > 0 {
				v := round1(float64(m[t.Unix()]) * 100 / float64(tot))
				vals[i] = &v
			}
		}
		lines[id] = vals
	}
	counts := make([]int64, len(labels))
	for i, t := range labels {
		counts[i] = totals[t.Unix()]
	}
	return map[string]any{"labels": labels, "lines": lines, "requests": counts}, nil
}

// SetEventState 待处理：确认 / 误报 / 忽略 / 移回
func (s *AdjustService) SetEventState(ctx context.Context, actor Actor, id int64, state string) (Event, error) {
	switch state {
	case EventTodo, EventConfirmed, EventFalse, EventIgnored:
	default:
		return Event{}, fmt.Errorf("%w：状态只能是 todo / confirmed / false / ignored", ErrInvalid)
	}
	old, e, err := s.Events.SetEventState(ctx, id, state, actor.ID)
	if err != nil {
		return e, err
	}
	names := map[string]string{EventTodo: "移回待处理", EventConfirmed: "确认有问题", EventFalse: "标为误报", EventIgnored: "忽略"}
	s.Audit.Record(ctx, actor, "event", "event", itoa(id), names[state]+"："+e.Title, map[string]any{"state": old.State},
		map[string]any{"state": state})
	return e, nil
}
