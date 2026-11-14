package dispatch

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// ---------- 调整权重弹层：账号（或它的某个模型）在每个所在分组承接多少 ----------

// SheetRow 弹层里一个分组的草稿。mode: normal | share | pause；pct = 它在这个分组的目标占比
type SheetRow struct {
	GroupID int64   `json:"group_id"`
	Mode    string  `json:"mode"`
	Pct     float64 `json:"pct"`
	Hard    bool    `json:"hard"`
}

// SheetReq 预览 / 保存
type SheetReq struct {
	AccountID int64      `json:"account_id"`
	Model     string     `json:"model"` // 空 = 账号的全部模型
	Also      []string   `json:"also"`  // 同样设置一起套用到的其他模型（「把这些模型从它身上挪走」）
	Rows      []SheetRow `json:"rows"`
	Note      string     `json:"note"`
}

// SheetGroup 一个分组：现在 → 预计
type SheetGroup struct {
	GroupID int64      `json:"group_id"`
	Name    string     `json:"name"`
	Row     SheetRow   `json:"row"`
	Now     GroupView  `json:"now"`
	Next    GroupView  `json:"next"`
	From    *RuleState `json:"from"`
	To      *RuleState `json:"to"`
	Changed bool       `json:"changed"`
	Only    bool       `json:"only"` // 这个分组只有它一个账号
}

// Sheet 弹层内容
type Sheet struct {
	AccountID int64        `json:"account_id"`
	Name      string       `json:"name"`
	Model     string       `json:"model"`
	Also      []string     `json:"also"`
	Groups    []SheetGroup `json:"groups"`
	Changed   bool         `json:"changed"`
	Danger    bool         `json:"danger"` // 保存后有请求会失败（一起停掉了只有它能跑的部分）
	Names     NameMap      `json:"names"`
	next      []Rule
}

func stateOf(r *Rule) *RuleState {
	if r == nil {
		return nil
	}
	return &RuleState{Share: r.Share, Hard: r.Hard, Pct: r.Pct}
}

func exactRule(rules []Rule, accountID, groupID int64, model string) *Rule {
	for i := range rules {
		if rules[i].AccountID == accountID && rules[i].GroupID == groupID && rules[i].Model == model {
			return &rules[i]
		}
	}
	return nil
}

// ruleText 正常 / 暂停 / 40%
func ruleText(s *RuleState) string {
	switch {
	case s == nil:
		return "正常"
	case s.Share <= 0 && s.Hard:
		return "暂停（全停）"
	case s.Share <= 0:
		return "暂停"
	case s.Pct != nil:
		return fmt.Sprintf("权重 %.0f%%", *s.Pct)
	}
	return fmt.Sprintf("权重 %.0f%%", s.Share*100)
}

func sameState(a, b *RuleState) bool { return ruleText(a) == ruleText(b) }

// SheetOpen 打开弹层：每个分组现在的设置
func (s *Service) SheetOpen(ctx context.Context, accountID int64, model string) (*Sheet, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	a := st.cat.Accounts[accountID]
	if a == nil {
		return nil, ErrNotFound
	}
	req := SheetReq{AccountID: accountID, Model: model}
	l := st.lanes()
	for _, gid := range a.Groups {
		if st.cat.Groups[gid] == nil {
			continue
		}
		gv := l.GroupView(accountID, model, gid, st.rules)
		r := exactRule(st.rules, accountID, gid, model)
		row := SheetRow{GroupID: gid, Mode: "normal", Pct: gv.Pct}
		switch {
		case r == nil && gv.Total == 0:
			continue // 这个分组里它没有流量：没得调
		case r == nil:
		case r.Share <= 0:
			row.Mode, row.Pct, row.Hard = "pause", 0, r.Hard
		default:
			row.Mode = "share"
			if r.Pct != nil {
				row.Pct = *r.Pct
			} else {
				row.Pct = gv.Locked + (gv.Reach-gv.Locked)*r.Share
			}
		}
		req.Rows = append(req.Rows, row)
	}
	return s.sheetCompute(st, req)
}

// SheetPreview 草稿改了：重新算预计
func (s *Service) SheetPreview(ctx context.Context, req SheetReq) (*Sheet, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return s.sheetCompute(st, req)
}

func (s *Service) sheetCompute(st *state, req SheetReq) (*Sheet, error) {
	a := st.cat.Accounts[req.AccountID]
	if a == nil {
		return nil, ErrNotFound
	}
	l := st.lanes()
	rules := append([]Rule(nil), st.rules...)
	models := append([]string{req.Model}, req.Also...)
	for _, m := range models {
		for _, row := range req.Rows {
			base := l.GroupView(a.ID, m, row.GroupID, st.rules)
			switch row.Mode {
			case "normal":
				rules = setRules(rules, []RuleChange{{AccountID: a.ID, GroupID: row.GroupID, Model: m}})
			case "pause":
				zero := 0.0
				rules = setRules(rules, []RuleChange{{AccountID: a.ID, GroupID: row.GroupID, Model: m, Share: &zero, Hard: row.Hard}})
			case "share":
				if base.Reach-base.Locked > 0.5 {
					x := clampF((row.Pct-base.Locked)/(base.Reach-base.Locked), 0.01, 1)
					rules = l.setShare(rules, a.ID, m, row.GroupID, x)
				}
			default:
				return nil, fmt.Errorf("%w：mode 只能是 normal / share / pause", ErrInvalid)
			}
		}
	}
	// 这次动到的（包括同组里被动让出空间的账号）：记下它在这个分组预计承接多少
	old := map[ruleKey]Rule{}
	for _, r := range st.rules {
		old[r.key()] = r
	}
	for i := range rules {
		r := &rules[i]
		if o, ok := old[r.key()]; ok && math.Abs(o.Share-r.Share) < 1e-9 && o.Hard == r.Hard {
			r.Pct = o.Pct
			continue
		}
		if r.Share > 0 {
			p := math.Round(l.GroupView(r.AccountID, r.Model, r.GroupID, rules).Pct)
			r.Pct = &p
		} else {
			r.Pct = nil
		}
	}
	sh := &Sheet{AccountID: a.ID, Name: a.Name, Model: req.Model, Also: req.Also, Groups: []SheetGroup{}, Names: st.names(), next: rules}
	if sh.Also == nil {
		sh.Also = []string{}
	}
	for _, row := range req.Rows {
		g := SheetGroup{GroupID: row.GroupID, Name: st.groupName(row.GroupID), Row: row,
			Now: l.GroupView(a.ID, req.Model, row.GroupID, st.rules), Next: l.GroupView(a.ID, req.Model, row.GroupID, rules),
			From: stateOf(exactRule(st.rules, a.ID, row.GroupID, req.Model)), To: stateOf(exactRule(rules, a.ID, row.GroupID, req.Model)),
			Only: len(l.Serving(row.GroupID)) == 1}
		g.Changed = !sameState(g.From, g.To) || (g.From != nil && g.To != nil && g.From.Hard != g.To.Hard)
		sh.Changed = sh.Changed || g.Changed
		if row.Mode == "pause" && row.Hard && len(g.Next.Dead) > 0 {
			sh.Danger = true
		}
		sh.Groups = append(sh.Groups, g)
	}
	// 一起套用的其他模型也算改动
	for _, m := range req.Also {
		for _, row := range req.Rows {
			if !sameState(stateOf(exactRule(st.rules, a.ID, row.GroupID, m)), stateOf(exactRule(rules, a.ID, row.GroupID, m))) {
				sh.Changed = true
			}
		}
	}
	return sh, nil
}

// diffKeys 两份规则之间变了的键
func diffKeys(from, to []Rule) []ruleKey {
	m := map[ruleKey][2]*RuleState{}
	for i := range from {
		x := m[from[i].key()]
		x[0] = stateOf(&from[i])
		m[from[i].key()] = x
	}
	for i := range to {
		x := m[to[i].key()]
		x[1] = stateOf(&to[i])
		m[to[i].key()] = x
	}
	var out []ruleKey
	for k, v := range m {
		if !sameState(v[0], v[1]) || (v[0] != nil && v[1] != nil && v[0].Hard != v[1].Hard) {
			out = append(out, k)
		}
	}
	return out
}

func lookup(rules []Rule, k ruleKey) *Rule { return exactRule(rules, k.Account, k.Group, k.Model) }

// SheetSave 保存：每个模型一条调整记录（同组被连带改动的账号记在第一条里，撤销时一起还原）
func (s *Service) SheetSave(ctx context.Context, req SheetReq, actor Actor) (*Sheet, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	sh, err := s.sheetCompute(st, req)
	if err != nil {
		return nil, err
	}
	if !sh.Changed {
		return nil, fmt.Errorf("%w：没有改动", ErrInvalid)
	}
	touched := diffKeys(st.rules, sh.next)
	var side []RecordSide
	for _, k := range touched {
		if k.Account != req.AccountID {
			side = append(side, RecordSide{AccountID: k.Account, GroupID: k.Group, Model: k.Model,
				From: stateOf(lookup(st.rules, k)), To: stateOf(lookup(sh.next, k))})
		}
	}
	op := s.operator(ctx, actor)
	var opID *int64
	if actor.ID > 0 {
		opID = &actor.ID
	}
	l := st.lanes()
	note := strings.TrimSpace(req.Note)
	var lines []string
	for i, m := range append([]string{req.Model}, req.Also...) {
		rec := Record{AccountID: req.AccountID, Model: m, Kind: "adjust", Note: note, OperatorID: opID, Operator: op}
		for _, g := range sh.Groups {
			from := stateOf(exactRule(st.rules, req.AccountID, g.GroupID, m))
			to := stateOf(exactRule(sh.next, req.AccountID, g.GroupID, m))
			if sameState(from, to) && !(from != nil && to != nil && from.Hard != to.Hard) {
				continue
			}
			rec.Groups = append(rec.Groups, RecordGroup{GroupID: g.GroupID, From: from, To: to})
			rec.Predicted = append(rec.Predicted, RecordPredict{GroupID: g.GroupID,
				Now: round1(l.GroupView(req.AccountID, m, g.GroupID, st.rules).Pct), Next: round1(l.GroupView(req.AccountID, m, g.GroupID, sh.next).Pct)})
			lines = append(lines, fmt.Sprintf("%s分组「%s」%s → %s", modelPrefix(m), g.Name, ruleText(from), ruleText(to)))
		}
		if len(rec.Groups) == 0 {
			continue
		}
		keys := []ruleKey{}
		for _, k := range touched {
			if k.Account == req.AccountID && k.Model == m {
				keys = append(keys, k)
			}
		}
		if i == 0 {
			rec.Side = side
			for _, k := range touched {
				if k.Account != req.AccountID {
					keys = append(keys, k)
				}
			}
		}
		if _, err := s.Rules.ApplyRecord(ctx, rec, keys, sh.next, 0); err != nil {
			return nil, err
		}
	}
	for _, x := range side {
		lines = append(lines, fmt.Sprintf("连带：%s %s分组「%s」%s → %s", st.accountName(x.AccountID), modelPrefix(x.Model), st.groupName(x.GroupID), ruleText(x.From), ruleText(x.To)))
	}
	sum := st.accountName(req.AccountID) + "：" + strings.Join(lines, "；")
	if note != "" {
		sum += "（" + note + "）"
	}
	s.audit(ctx, actor, "weight", "adjust", itoa(req.AccountID), sum, nil, req)
	s.rulesChanged()
	return sh, nil
}

func modelPrefix(m string) string {
	if m == "" {
		return ""
	}
	return m + " · "
}

func (s *Service) rulesChanged() {
	if s.OnRules != nil {
		s.OnRules()
	}
}

// Restore 把一条生效中的设置去掉（恢复正常）
func (s *Service) Restore(ctx context.Context, accountID, groupID int64, model string, actor Actor) error {
	st, err := s.load(ctx)
	if err != nil {
		return err
	}
	r := exactRule(st.rules, accountID, groupID, model)
	if r == nil {
		return ErrNotFound
	}
	next := setRules(st.rules, []RuleChange{{AccountID: accountID, GroupID: groupID, Model: model}})
	l := st.lanes()
	rec := Record{AccountID: accountID, Model: model, Kind: "restore", Note: "恢复正常", Operator: s.operator(ctx, actor),
		Groups:    []RecordGroup{{GroupID: groupID, From: stateOf(r)}},
		Predicted: []RecordPredict{{GroupID: groupID, Now: round1(l.GroupView(accountID, model, groupID, st.rules).Pct), Next: round1(l.GroupView(accountID, model, groupID, next).Pct)}}}
	if actor.ID > 0 {
		rec.OperatorID = &actor.ID
	}
	if _, err := s.Rules.ApplyRecord(ctx, rec, []ruleKey{r.key()}, next, 0); err != nil {
		return err
	}
	s.audit(ctx, actor, "weight", "restore", itoa(accountID), fmt.Sprintf("%s：%s分组「%s」%s → 正常", st.accountName(accountID), modelPrefix(model),
		st.groupName(groupID), ruleText(stateOf(r))), nil, nil)
	s.rulesChanged()
	return nil
}

// Undo 把一次调整整体恢复原样（连带改动的其他账号一起还原）
func (s *Service) Undo(ctx context.Context, id int64, actor Actor) error {
	rec, err := s.Rules.GetRecord(ctx, id)
	if err != nil {
		return err
	}
	if rec.UndoneAt != nil || rec.Kind == "undo" {
		return fmt.Errorf("%w：这条调整已经恢复过了", ErrConflict)
	}
	st, err := s.load(ctx)
	if err != nil {
		return err
	}
	set := func(rules []Rule, acc, g int64, m string, to *RuleState) []Rule {
		c := RuleChange{AccountID: acc, GroupID: g, Model: m}
		if to != nil {
			v := to.Share
			c.Share, c.Hard = &v, to.Hard
		}
		out := setRules(rules, []RuleChange{c})
		if to != nil && to.Pct != nil {
			if r := exactRule(out, acc, g, m); r != nil {
				r.Pct = to.Pct
			}
		}
		return out
	}
	next := append([]Rule(nil), st.rules...)
	l := st.lanes()
	back := Record{AccountID: rec.AccountID, Model: rec.Model, Kind: "undo", Note: "恢复原样", Operator: s.operator(ctx, actor)}
	if actor.ID > 0 {
		back.OperatorID = &actor.ID
	}
	var keys []ruleKey
	for _, g := range rec.Groups {
		before := l.GroupView(rec.AccountID, rec.Model, g.GroupID, next)
		next = set(next, rec.AccountID, g.GroupID, rec.Model, g.From)
		keys = append(keys, ruleKey{rec.AccountID, g.GroupID, rec.Model})
		back.Groups = append(back.Groups, RecordGroup{GroupID: g.GroupID, From: stateOf(exactRule(st.rules, rec.AccountID, g.GroupID, rec.Model)), To: g.From})
		back.Predicted = append(back.Predicted, RecordPredict{GroupID: g.GroupID, Now: round1(before.Pct), Next: round1(l.GroupView(rec.AccountID, rec.Model, g.GroupID, next).Pct)})
	}
	for _, x := range rec.Side {
		next = set(next, x.AccountID, x.GroupID, x.Model, x.From)
		keys = append(keys, ruleKey{x.AccountID, x.GroupID, x.Model})
		back.Side = append(back.Side, RecordSide{AccountID: x.AccountID, GroupID: x.GroupID, Model: x.Model, From: x.To, To: x.From})
	}
	if _, err := s.Rules.ApplyRecord(ctx, back, keys, next, rec.ID); err != nil {
		return err
	}
	var gs []string
	for _, g := range rec.Groups {
		gs = append(gs, st.groupName(g.GroupID))
	}
	s.audit(ctx, actor, "weight", "undo", itoa(rec.AccountID), fmt.Sprintf("恢复原样：%s%s（%s）", st.accountName(rec.AccountID), strings.TrimSuffix(" · "+modelPrefix(rec.Model), " · "),
		strings.Join(gs, "、")), nil, nil)
	s.rulesChanged()
	return nil
}

// ---------- 调整记录 ----------

// RecordView 调整记录 + 效果
type RecordView struct {
	Record
	Effect Effect `json:"effect"`
}

func parseWindow(w string) int {
	switch w {
	case "1":
		return 1
	case "6":
		return 6
	case "24":
		return 24
	}
	return 0
}

func (s *Service) records(ctx context.Context, st *state, accountID int64, window string) ([]RecordView, error) {
	recs, err := s.Rules.ListRecords(ctx, accountID, 100)
	if err != nil {
		return nil, err
	}
	out := make([]RecordView, 0, len(recs))
	w := parseWindow(window)
	for _, r := range recs {
		if r.Groups == nil {
			r.Groups = []RecordGroup{}
		}
		if r.Side == nil {
			r.Side = []RecordSide{}
		}
		if r.Predicted == nil {
			r.Predicted = []RecordPredict{}
		}
		out = append(out, RecordView{Record: r, Effect: st.effect(r, w)})
	}
	return out, nil
}

// ActiveRule 生效中的设置（带上是谁、什么时候、为什么）
type ActiveRule struct {
	Rule
	Operator string `json:"operator"`
	Note     string `json:"note"`
}

// Changes 调整记录页：生效中的设置 + 历史
type Changes struct {
	Apply   bool         `json:"apply"`
	Active  []ActiveRule `json:"active"`
	Records []RecordView `json:"records"`
	Names   NameMap      `json:"names"`
}

func (s *Service) Changes(ctx context.Context, window string) (*Changes, error) {
	st, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	recs, err := s.records(ctx, st, 0, window)
	if err != nil {
		return nil, err
	}
	byID := map[int64]Record{}
	for _, r := range recs {
		byID[r.ID] = r.Record
	}
	out := &Changes{Apply: st.cfg.Apply, Active: []ActiveRule{}, Records: recs, Names: st.names()}
	for _, r := range st.rules {
		a := ActiveRule{Rule: r}
		if r.RecordID != nil {
			if rec, ok := byID[*r.RecordID]; ok {
				a.Operator = rec.Operator
				a.Note = rec.Note
				if rec.AccountID != r.AccountID {
					a.Note = "随 " + st.accountName(rec.AccountID) + " 一起调整"
				}
			}
		}
		out.Active = append(out.Active, a)
	}
	return out, nil
}

// Badges 上游账号列表里的小标签：账号 → 「调度台：暂停 / 已调权重」
func (s *Service) Badges(ctx context.Context) (map[int64]string, error) {
	rules, err := s.Rules.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	cfg, _, err := s.Config.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	if !cfg.Apply {
		return out, nil
	}
	pause, share := map[int64]int{}, map[int64]int{}
	for _, r := range rules {
		if r.Share <= 0 {
			pause[r.AccountID]++
		} else {
			share[r.AccountID]++
		}
	}
	for id := range pause {
		out[id] = "已暂停部分线路"
	}
	for id := range share {
		if _, ok := out[id]; !ok {
			out[id] = "已调权重"
		}
	}
	return out, nil
}
