// Package outreach — 内容资产：外壳（Layout）与模板（Template）。
//
// 一封邮件 = 外壳 ⊕ 正文 ⊕ 合规区。
//   - 外壳：品牌层，由「块」组成（页头 / 版式 / 页脚品牌区），不开放自由 HTML，避免客户端兼容问题和误删合规内容
//   - 模板：某个目的的主题 + 正文，可复用；新建活动时复制到活动，之后互不影响
//   - 合规区：系统按目的追加（render.go），不在这里
package outreach

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 外壳页头与版式
const (
	HeaderNone = "none"
	HeaderText = "text"
	HeaderLogo = "logo"

	StyleCard  = "card"
	StylePlain = "plain"
)

// LayoutConfig 外壳的块配置
type LayoutConfig struct {
	Header     string `json:"header"`      // none | text（自定义名称或站点名）| logo
	HeaderName string `json:"header_name"` // header=text 时显示的文字；空 = 用站点名
	LogoURL    string `json:"logo_url"`    // header=logo 时必填
	HeaderLink string `json:"header_link"` // 点页头跳转，可空
	ThemeColor string `json:"theme_color"` // #RRGGBB：按钮、链接、卡片顶边
	Style      string `json:"style"`       // card（卡片）| plain（通栏纯文本）
	Width      int    `json:"width"`       // 内容宽度 px
	FooterMD   string `json:"footer_md"`   // 页脚品牌区 Markdown，只能用 {{site_name}}
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (c LayoutConfig) withDefaults() LayoutConfig {
	if c.Header == "" {
		c.Header = HeaderText
	}
	if !hexColorRe.MatchString(c.ThemeColor) {
		c.ThemeColor = "#2563eb"
	}
	if c.Style != StylePlain {
		c.Style = StyleCard
	}
	if c.Width < 480 || c.Width > 800 {
		c.Width = 580
	}
	return c
}

func httpURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (c *LayoutConfig) validate() error {
	c.HeaderName = strings.TrimSpace(c.HeaderName)
	c.LogoURL = strings.TrimSpace(c.LogoURL)
	c.HeaderLink = strings.TrimSpace(c.HeaderLink)
	c.FooterMD = strings.TrimSpace(c.FooterMD)
	switch {
	case c.Header != HeaderNone && c.Header != HeaderText && c.Header != HeaderLogo:
		return badInput("页头只能是 无 / 自定义名称 / Logo")
	case len([]rune(c.HeaderName)) > 60:
		return badInput("页头名称不超过 60 字")
	case c.Header == HeaderLogo && !httpURL(c.LogoURL):
		return badInput("Logo 需填写 http(s):// 开头的图片地址")
	case c.HeaderLink != "" && !httpURL(c.HeaderLink):
		return badInput("页头链接需是 http(s):// 开头的地址")
	case !hexColorRe.MatchString(c.ThemeColor):
		return badInput("主题色格式为 #RRGGBB")
	case c.Style != StyleCard && c.Style != StylePlain:
		return badInput("版式只能是 卡片 / 通栏")
	case c.Width < 480 || c.Width > 800:
		return badInput("内容宽度需在 480 到 800 之间")
	case len(c.FooterMD) > 2000:
		return badInput("页脚不超过 2000 字符")
	}
	if unk := unknownIn(map[string]bool{"site_name": true}, c.FooterMD); len(unk) > 0 {
		return badInput("页脚只能使用 {{site_name}}，不认识：" + wrapVars(unk))
	}
	return nil
}

// Layout 外壳
type Layout struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	IsDefault bool         `json:"is_default"`
	Config    LayoutConfig `json:"config"`
	UsedBy    int          `json:"used_by"` // 引用它的模板 + 活动数（列表用）
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// LayoutInput 新建 / 修改外壳
type LayoutInput struct {
	Name   string       `json:"name"`
	Config LayoutConfig `json:"config"`
}

func (in *LayoutInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 60 {
		return badInput("外壳名称不能为空，且不超过 60 字")
	}
	return in.Config.validate()
}

// Template 模板
type Template struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Purpose   string    `json:"purpose"`
	Subject   string    `json:"subject"`
	BodyMD    string    `json:"body_md"`
	LayoutID  *int64    `json:"layout_id"`
	Builtin   bool      `json:"builtin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateInput 新建 / 修改模板
type TemplateInput struct {
	Name     string `json:"name"`
	Purpose  string `json:"purpose"`
	Subject  string `json:"subject"`
	BodyMD   string `json:"body_md"`
	LayoutID *int64 `json:"layout_id"`
}

func (in *TemplateInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)
	in.BodyMD = strings.TrimSpace(in.BodyMD)
	switch {
	case in.Name == "" || len([]rune(in.Name)) > 60:
		return badInput("模板名称不能为空，且不超过 60 字")
	case in.Subject == "" || len([]rune(in.Subject)) > 200:
		return badInput("邮件主题不能为空，且不超过 200 字")
	case in.BodyMD == "" || len(in.BodyMD) > 50_000:
		return badInput("邮件正文不能为空，且不超过 5 万字符")
	}
	if in.LayoutID != nil && *in.LayoutID <= 0 {
		in.LayoutID = nil
	}
	return nil
}

var ErrInUse = errors.New("outreach: in use")

// ─── 外壳仓储 ────────────────────────────────────────────────────────────────

const layoutCols = `l.id, l.name, l.is_default, l.config, l.created_at, l.updated_at,
	(SELECT COUNT(*) FROM plus_mail_templates t WHERE t.layout_id = l.id)
	+ (SELECT COUNT(*) FROM plus_outreach_campaigns c WHERE c.layout_id = l.id AND c.deleted_at IS NULL)`

func scanLayout(r rowScanner) (*Layout, error) {
	var (
		l   Layout
		cfg []byte
	)
	if err := r.Scan(&l.ID, &l.Name, &l.IsDefault, &cfg, &l.CreatedAt, &l.UpdatedAt, &l.UsedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(cfg, &l.Config)
	return &l, nil
}

func (r *Repository) ListLayouts(ctx context.Context) ([]Layout, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+layoutCols+` FROM plus_mail_layouts l ORDER BY l.is_default DESC, l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Layout{}
	for rows.Next() {
		l, err := scanLayout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (r *Repository) GetLayout(ctx context.Context, id int64) (*Layout, error) {
	return scanLayout(r.db.QueryRowContext(ctx, `SELECT `+layoutCols+` FROM plus_mail_layouts l WHERE l.id = $1`, id))
}

// ResolveLayout id 为空或已不存在时用默认外壳；连默认都没有时用内置默认值
func (r *Repository) ResolveLayout(ctx context.Context, id *int64) (LayoutConfig, error) {
	var cfg []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT config FROM plus_mail_layouts
		WHERE id = $1 OR is_default ORDER BY (id = $1) DESC NULLS LAST LIMIT 1`, id).Scan(&cfg)
	var out LayoutConfig
	if errors.Is(err, sql.ErrNoRows) {
		return out.withDefaults(), nil
	}
	if err != nil {
		return out, err
	}
	_ = json.Unmarshal(cfg, &out)
	return out.withDefaults(), nil
}

func (r *Repository) CreateLayout(ctx context.Context, in LayoutInput) (*Layout, error) {
	cfg, _ := json.Marshal(in.Config)
	var id int64
	if err := r.db.QueryRowContext(ctx, `
		INSERT INTO plus_mail_layouts (name, config, is_default)
		VALUES ($1, $2, NOT EXISTS (SELECT 1 FROM plus_mail_layouts WHERE is_default)) RETURNING id`, in.Name, cfg).Scan(&id); err != nil {
		return nil, err
	}
	return r.GetLayout(ctx, id)
}

func (r *Repository) UpdateLayout(ctx context.Context, id int64, in LayoutInput) (*Layout, error) {
	cfg, _ := json.Marshal(in.Config)
	res, err := r.db.ExecContext(ctx, `UPDATE plus_mail_layouts SET name=$2, config=$3, updated_at=NOW() WHERE id=$1`, id, in.Name, cfg)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return r.GetLayout(ctx, id)
}

func (r *Repository) SetDefaultLayout(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM plus_mail_layouts WHERE id=$1 FOR UPDATE`, id).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE plus_mail_layouts SET is_default=FALSE WHERE is_default AND id<>$1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE plus_mail_layouts SET is_default=TRUE, updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLayout 默认外壳和仍被模板 / 活动引用的外壳不能删（已生成的批次持有快照，不受影响）
func (r *Repository) DeleteLayout(ctx context.Context, id int64) error {
	l, err := r.GetLayout(ctx, id)
	if err != nil {
		return err
	}
	if l.IsDefault {
		return badInput("默认外壳不能删除，请先把其他外壳设为默认")
	}
	if l.UsedBy > 0 {
		return badInput("还有模板或活动在使用这个外壳，请先改用其他外壳")
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM plus_mail_layouts WHERE id=$1`, id)
	return err
}

// ─── 模板仓储 ────────────────────────────────────────────────────────────────

const templateCols = `id, name, purpose, subject, body_md, layout_id, builtin, created_at, updated_at`

func scanTemplate(r rowScanner) (*Template, error) {
	var t Template
	if err := r.Scan(&t.ID, &t.Name, &t.Purpose, &t.Subject, &t.BodyMD, &t.LayoutID, &t.Builtin, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repository) ListTemplates(ctx context.Context, purpose string) ([]Template, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+templateCols+` FROM plus_mail_templates
		WHERE $1 = '' OR purpose = $1 ORDER BY purpose, builtin DESC, id`, purpose)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *Repository) GetTemplate(ctx context.Context, id int64) (*Template, error) {
	return scanTemplate(r.db.QueryRowContext(ctx, `SELECT `+templateCols+` FROM plus_mail_templates WHERE id=$1`, id))
}

// BuiltinTemplate 某目的的内置模板：直发（例如单发验证邀请）使用它
func (r *Repository) BuiltinTemplate(ctx context.Context, purpose string) (*Template, error) {
	return scanTemplate(r.db.QueryRowContext(ctx, `SELECT `+templateCols+` FROM plus_mail_templates WHERE purpose=$1 AND builtin`, purpose))
}

func (r *Repository) CreateTemplate(ctx context.Context, in TemplateInput) (*Template, error) {
	return scanTemplate(r.db.QueryRowContext(ctx, `
		INSERT INTO plus_mail_templates (name, purpose, subject, body_md, layout_id)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+templateCols, in.Name, in.Purpose, in.Subject, in.BodyMD, in.LayoutID))
}

// UpdateTemplate 目的不可改（变量集合由目的决定，改目的等于换一个模板）
func (r *Repository) UpdateTemplate(ctx context.Context, id int64, in TemplateInput) (*Template, error) {
	return scanTemplate(r.db.QueryRowContext(ctx, `
		UPDATE plus_mail_templates SET name=$2, subject=$3, body_md=$4, layout_id=$5, updated_at=NOW()
		WHERE id=$1 RETURNING `+templateCols, id, in.Name, in.Subject, in.BodyMD, in.LayoutID))
}

func (r *Repository) DeleteTemplate(ctx context.Context, id int64) error {
	t, err := r.GetTemplate(ctx, id)
	if err != nil {
		return err
	}
	if t.Builtin {
		return badInput("内置模板不能删除（单发验证邀请等直发邮件使用它），可以直接编辑")
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM plus_mail_templates WHERE id=$1`, id)
	return err
}

func (r *Repository) layoutExists(ctx context.Context, id *int64) (bool, error) {
	if id == nil {
		return true, nil
	}
	var ok bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM plus_mail_layouts WHERE id=$1)`, *id).Scan(&ok)
	return ok, err
}
