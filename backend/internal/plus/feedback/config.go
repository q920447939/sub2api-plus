// Package feedback — [plus] 配置
package feedback

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldType 场景补充字段类型
type FieldType string

const (
	FieldText     FieldType = "text"
	FieldTextarea FieldType = "textarea"
	FieldSelect   FieldType = "select"
	FieldDatetime FieldType = "datetime"
)

// Field 场景补充字段（选填）
type Field struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Type        FieldType `json:"type"`
	Options     []string  `json:"options,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
}

// Scenario 反馈场景，从属于某个类型。Key 固定不变，改名不影响历史统计
type Scenario struct {
	Key     string  `json:"key"`
	Kind    Kind    `json:"kind"`
	Label   string  `json:"label"`
	Icon    string  `json:"icon"`
	Hint    string  `json:"hint"`             // 描述框占位文字
	Fields  []Field `json:"fields,omitempty"` // 补充字段
	Enabled bool    `json:"enabled"`
}

// Notice 反馈中心公告（Markdown，前端渲染时禁用原始 HTML）
type Notice struct {
	Enabled  bool   `json:"enabled"`
	Markdown string `json:"markdown"`
}

// Limits 防刷
type Limits struct {
	DailyMax        int `json:"daily_max"`        // 每人每天最多条数，0=不限
	IntervalSeconds int `json:"interval_seconds"` // 两次提交最短间隔，0=不限
	OpenMax         int `json:"open_max"`         // 同时未结案最多条数，0=不限
}

// AttachmentRule 附件
type AttachmentRule struct {
	MaxCount int `json:"max_count"` // 每次提交/补充最多张数，0=不允许上传
	MaxMB    int `json:"max_mb"`
}

// Reward 积分（只用于已采纳的建议）
type Reward struct {
	Default int64 `json:"default"`
	Step    int64 `json:"step"`
}

// Config 反馈中心配置
type Config struct {
	Enabled        bool           `json:"enabled"`
	FloatButton    bool           `json:"float_button"`
	Notice         Notice         `json:"notice"`
	Scenarios      []Scenario     `json:"scenarios"`
	Limits         Limits         `json:"limits"`
	Attachments    AttachmentRule `json:"attachments"`
	AskTimeoutDays int            `json:"ask_timeout_days"` // 已回复后用户多少天不回应自动结案，0=关闭
	Reward         Reward         `json:"reward"`
}

const (
	ContentMin    = 10
	ContentMax    = 2000
	ReplyMax      = 2000
	maxTags       = 10
	maxTagLen     = 24
	maxNoticeLen  = 4000
	maxScenarios  = 40
	maxFieldValue = 500
)

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)

func cfgErr(format string, args ...any) error { return inputErr(fmt.Sprintf(format, args...)) }

// Validate 校验配置
func (c *Config) Validate() error {
	if len(c.Scenarios) > maxScenarios {
		return cfgErr("场景最多 %d 个", maxScenarios)
	}
	seen := map[string]bool{}
	enabled := map[Kind]int{}
	for i := range c.Scenarios {
		sc := &c.Scenarios[i]
		sc.Label = strings.TrimSpace(sc.Label)
		if !keyRe.MatchString(sc.Key) {
			return cfgErr("场景标识「%s」只能用小写字母、数字和下划线，并以字母开头", sc.Key)
		}
		if seen[sc.Key] {
			return cfgErr("场景标识「%s」重复", sc.Key)
		}
		seen[sc.Key] = true
		if !sc.Kind.Valid() {
			return cfgErr("场景「%s」的类型无效", sc.Key)
		}
		if sc.Label == "" {
			return cfgErr("场景「%s」缺少名称", sc.Key)
		}
		fieldSeen := map[string]bool{}
		for _, f := range sc.Fields {
			if !keyRe.MatchString(f.Key) || fieldSeen[f.Key] {
				return cfgErr("场景「%s」的字段标识「%s」无效或重复", sc.Label, f.Key)
			}
			fieldSeen[f.Key] = true
			if strings.TrimSpace(f.Label) == "" {
				return cfgErr("场景「%s」有字段缺少名称", sc.Label)
			}
			switch f.Type {
			case FieldText, FieldTextarea, FieldDatetime:
			case FieldSelect:
				if len(f.Options) == 0 {
					return cfgErr("场景「%s」的下拉字段「%s」至少要有一个选项", sc.Label, f.Label)
				}
			default:
				return cfgErr("场景「%s」的字段「%s」类型无效", sc.Label, f.Label)
			}
		}
		if sc.Enabled {
			enabled[sc.Kind]++
		}
	}
	if c.Enabled && (enabled[KindIssue] == 0 || enabled[KindSuggestion] == 0) {
		return cfgErr("问题和建议都至少要启用一个场景")
	}
	if len([]rune(c.Notice.Markdown)) > maxNoticeLen {
		return cfgErr("公告最多 %d 字", maxNoticeLen)
	}
	if c.Limits.DailyMax < 0 || c.Limits.IntervalSeconds < 0 || c.Limits.OpenMax < 0 {
		return cfgErr("防刷阈值不能为负")
	}
	if c.Attachments.MaxCount < 0 || c.Attachments.MaxCount > 9 {
		return cfgErr("附件数量需在 0-9 之间")
	}
	if c.Attachments.MaxMB < 1 || c.Attachments.MaxMB > 20 {
		return cfgErr("单张附件大小需在 1-20 MB 之间")
	}
	if c.AskTimeoutDays < 0 || c.AskTimeoutDays > 365 {
		return cfgErr("自动结案天数需在 0-365 之间")
	}
	if c.Reward.Default < 0 || c.Reward.Step <= 0 {
		return cfgErr("积分默认值不能为负，步长必须大于 0")
	}
	return nil
}

// Scenario 按 key 查找（含已停用的，历史数据要能显示名称）
func (c *Config) Scenario(key string) (Scenario, bool) {
	for _, s := range c.Scenarios {
		if s.Key == key {
			return s, true
		}
	}
	return Scenario{}, false
}

// UsableScenario 用户提交时可选的场景：存在、启用、类型匹配
func (c *Config) UsableScenario(kind Kind, key string) (Scenario, bool) {
	s, ok := c.Scenario(key)
	if !ok || !s.Enabled || s.Kind != kind {
		return Scenario{}, false
	}
	return s, true
}

// CleanExtra 只保留场景里定义过的字段，截断超长值，丢掉空值
func (s Scenario) CleanExtra(in map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range s.Fields {
		v := strings.TrimSpace(in[f.Key])
		if v == "" {
			continue
		}
		if r := []rune(v); len(r) > maxFieldValue {
			v = string(r[:maxFieldValue])
		}
		if f.Type == FieldSelect && !contains(f.Options, v) {
			continue
		}
		out[f.Key] = v
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// NormalizeTags 去空白、去重、限制数量和长度
func NormalizeTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if r := []rune(t); len(r) > maxTagLen {
			t = string(r[:maxTagLen])
		}
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == maxTags {
			break
		}
	}
	return out
}

// DefaultConfig 默认配置：问题 8 个场景，建议 6 个场景
func DefaultConfig() Config {
	issue := func(key, icon, label, hint string, fields ...Field) Scenario {
		return Scenario{Key: key, Kind: KindIssue, Icon: icon, Label: label, Hint: hint, Fields: fields, Enabled: true}
	}
	idea := func(key, icon, label, hint string, fields ...Field) Scenario {
		return Scenario{Key: key, Kind: KindSuggestion, Icon: icon, Label: label, Hint: hint, Fields: fields, Enabled: true}
	}
	model := Field{Key: "model", Label: "模型", Type: FieldText, Placeholder: "例如 claude-sonnet-4-5"}
	expect := Field{Key: "expect", Label: "期望的效果", Type: FieldTextarea}
	return Config{
		Enabled:     true,
		FloatButton: true,
		Notice:      Notice{Enabled: true, Markdown: "紧急问题请直接联系售后群，回复更及时。\n\n这里适合不急的问题和改进建议，我们会逐条查看。"},
		Scenarios: []Scenario{
			issue("access", "🔌", "接入和配置", "用的是哪个客户端？卡在哪一步？",
				Field{Key: "client", Label: "客户端", Type: FieldSelect, Options: []string{"Claude Code", "Codex CLI", "Cursor", "Cherry Studio", "其他"}}),
			issue("call_error", "⚠️", "调用报错", "什么时候、调用什么模型时出错？报错原文是什么？",
				Field{Key: "occurred_at", Label: "大概时间", Type: FieldDatetime}, model,
				Field{Key: "error", Label: "报错原文", Type: FieldTextarea}),
			issue("model_quality", "🧠", "模型效果不对", "哪里不对？最好附上输入和输出", model),
			issue("speed", "🐢", "速度慢或不稳定", "从什么时候开始？一直这样还是偶尔？",
				Field{Key: "target", Label: "模型或分组", Type: FieldText}),
			issue("billing", "💳", "充值和扣费", "金额、时间和遇到的情况",
				Field{Key: "order_no", Label: "订单号", Type: FieldText}),
			issue("activity", "🎁", "活动和权益", "哪个活动？发生了什么？"),
			issue("account", "👤", "账号和登录", "遇到了什么情况？"),
			issue("other_issue", "💬", "其他问题", "说说遇到的情况"),
			idea("new_model", "✨", "想要新模型", "想用哪个模型？主要拿来做什么？",
				Field{Key: "model", Label: "模型名称", Type: FieldText}),
			idea("new_feature", "🧩", "想要新功能", "现在做不到什么？想用它解决什么问题？", expect),
			idea("ux", "🎯", "体验可以更好", "哪里用着不顺手？", expect),
			idea("pricing", "🏷️", "价格和套餐", "希望怎么调整？"),
			idea("activity_idea", "🎉", "活动和福利", "想要什么玩法或奖励？"),
			idea("other_suggestion", "💬", "其他建议", "说说你的想法"),
		},
		Limits:         Limits{DailyMax: 10, IntervalSeconds: 60, OpenMax: 5},
		Attachments:    AttachmentRule{MaxCount: 3, MaxMB: 5},
		AskTimeoutDays: 7,
		Reward:         Reward{Default: 500, Step: 100},
	}
}
