package dispatch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// CatAccount 账号（只读上游 accounts）。不保存明文 Key，只保存凭证指纹 CredKey。
type CatAccount struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Platform    string    `json:"platform"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Schedulable bool      `json:"schedulable"`
	BaseURL     string    `json:"base_url"`
	CredKey     string    `json:"-"`
	Rate        float64   `json:"rate_multiplier"`
	CreatedAt   time.Time `json:"created_at"`
	Groups      []int64   `json:"group_ids"`
	Priority    int       `json:"priority"`
	Direct      bool      `json:"direct"`     // 能否账号直连（API Key 类账号）
	DirectWhy   string    `json:"direct_why"` // 不能直连的原因
	Models      []string  `json:"models"`     // 模型映射里写了的模型（空 = 没配映射）
	creds       map[string]any
	extra       map[string]any
	memo        *sync.Map // model → bool（目录快照会被多个请求共用，所以用并发安全的缓存）
}

// Supports 账号支不支持这个模型（和网关选号同一个判断）
func (a *CatAccount) Supports(model string) bool {
	if a.memo == nil {
		return service.PlusAccountSupportsModel(a.Platform, a.Type, a.creds, a.extra, model)
	}
	if v, ok := a.memo.Load(model); ok {
		return v.(bool)
	}
	v := service.PlusAccountSupportsModel(a.Platform, a.Type, a.creds, a.extra, model)
	a.memo.Store(model, v)
	return v
}

// Serving 是否参与调度（启用且调度已开启）
func (a *CatAccount) Serving() bool { return a.Status == "active" && a.Schedulable }

// CatGroup 分组（只读上游 groups）
type CatGroup struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Platform string  `json:"platform"`
	Accounts []int64 `json:"account_ids"`
}

// Catalog 账号 / 分组目录快照
type Catalog struct {
	Accounts map[int64]*CatAccount
	Groups   map[int64]*CatGroup
	GroupIDs []int64 // 按 id 排序
	byCred   map[string][]int64
}

// Aliases 和这个账号凭证相同的所有账号（含自己），按 id 升序
func (c *Catalog) Aliases(accountID int64) []int64 {
	a := c.Accounts[accountID]
	if a == nil {
		return nil
	}
	return c.byCred[a.CredKey]
}

// GroupsOf 账号所在分组名
func (c *Catalog) GroupNames(ids []int64) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if g := c.Groups[id]; g != nil {
			out = append(out, g.Name)
		}
	}
	return out
}

// CatalogSource 读取目录
type CatalogSource interface {
	LoadCatalog(ctx context.Context) (*Catalog, error)
	// Credential 账号直连用的凭证（只在执行时读取）
	Credential(ctx context.Context, accountID int64) (Credential, error)
}

// SQLCatalog 用原生 SQL 只读上游表
type SQLCatalog struct{ DB *sql.DB }

var _ CatalogSource = SQLCatalog{}

// directTypes 能直连的账号类型
var directTypes = map[string]bool{"apikey": true, "upstream": true}

// defaultBaseURL 各平台 API Key 账号不填地址时的官方地址
func defaultBaseURL(platform string) string {
	switch platform {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai":
		return "https://api.openai.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com"
	}
	return ""
}

// credKey 凭证指纹：同一个地址 + Key 只测一次
func credKey(accountID int64, typ, baseURL, apiKey string) string {
	if !directTypes[typ] || apiKey == "" {
		return "acct:" + itoa(accountID)
	}
	sum := sha256.Sum256([]byte(normalizeBaseURL(baseURL) + "|" + apiKey))
	return hex.EncodeToString(sum[:8])
}

// normalizeBaseURL 去掉末尾的 / 和 /v1（请求路径由协议编解码器补全）
func normalizeBaseURL(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	for _, suf := range []string{"/v1beta", "/v1"} {
		if strings.HasSuffix(strings.ToLower(u), suf) {
			u = u[:len(u)-len(suf)]
		}
	}
	return strings.TrimRight(u, "/")
}

func (s SQLCatalog) LoadCatalog(ctx context.Context) (*Catalog, error) {
	cat := &Catalog{Accounts: map[int64]*CatAccount{}, Groups: map[int64]*CatGroup{}, byCred: map[string][]int64{}}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT a.id, a.name, a.platform, a.type, a.status, a.schedulable,
		       COALESCE(a.credentials->>'base_url', ''), COALESCE(a.credentials->>'api_key', ''),
		       COALESCE(a.rate_multiplier, 1), a.created_at,
		       COALESCE(ARRAY(SELECT ag.group_id FROM account_groups ag WHERE ag.account_id = a.id ORDER BY ag.group_id), '{}'),
		       a.priority,
		       COALESCE(a.credentials, '{}'::jsonb) - ARRAY['api_key', 'access_token', 'refresh_token', 'id_token', 'session_key', 'cookie'],
		       COALESCE(a.extra, '{}'::jsonb)
		FROM accounts a WHERE a.deleted_at IS NULL ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			a           CatAccount
			key         string
			groups      pq.Int64Array
			creds, extr []byte
		)
		if err := rows.Scan(&a.ID, &a.Name, &a.Platform, &a.Type, &a.Status, &a.Schedulable, &a.BaseURL, &key, &a.Rate, &a.CreatedAt, &groups,
			&a.Priority, &creds, &extr); err != nil {
			return nil, err
		}
		a.memo = &sync.Map{}
		_ = json.Unmarshal(creds, &a.creds)
		_ = json.Unmarshal(extr, &a.extra)
		if mm, ok := a.creds["model_mapping"].(map[string]any); ok {
			for k := range mm {
				a.Models = append(a.Models, k)
			}
			sort.Strings(a.Models)
		}
		a.Groups = []int64(groups)
		a.CredKey = credKey(a.ID, a.Type, a.BaseURL, key)
		switch {
		case !directTypes[a.Type]:
			a.DirectWhy = "不是 API Key 账号，暂不支持直连检测"
		case key == "":
			a.DirectWhy = "账号没有配置 API Key"
		case a.BaseURL == "" && defaultBaseURL(a.Platform) == "":
			a.DirectWhy = "账号没有配置地址"
		default:
			a.Direct = true
		}
		if a.BaseURL == "" {
			a.BaseURL = defaultBaseURL(a.Platform)
		}
		acc := a
		cat.Accounts[a.ID] = &acc
		cat.byCred[a.CredKey] = append(cat.byCred[a.CredKey], a.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	grows, err := s.DB.QueryContext(ctx, `SELECT id, name, platform FROM groups WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer grows.Close()
	for grows.Next() {
		var g CatGroup
		if err := grows.Scan(&g.ID, &g.Name, &g.Platform); err != nil {
			return nil, err
		}
		gg := g
		cat.Groups[g.ID] = &gg
		cat.GroupIDs = append(cat.GroupIDs, g.ID)
	}
	if err := grows.Err(); err != nil {
		return nil, err
	}
	for _, a := range cat.Accounts {
		for _, gid := range a.Groups {
			if g := cat.Groups[gid]; g != nil {
				g.Accounts = append(g.Accounts, a.ID)
			}
		}
	}
	for _, g := range cat.Groups {
		sort.Slice(g.Accounts, func(i, j int) bool { return g.Accounts[i] < g.Accounts[j] })
	}
	return cat, nil
}

// Credential 账号直连用的凭证
type Credential struct {
	BaseURL     string
	APIKey      string
	Platform    string
	Type        string
	Credentials map[string]any // 原始凭证（用来按账号的模型映射换算模型名）
}

func (s SQLCatalog) Credential(ctx context.Context, accountID int64) (Credential, error) {
	var (
		c   Credential
		raw []byte
	)
	err := s.DB.QueryRowContext(ctx, `
		SELECT platform, type, COALESCE(credentials, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL`, accountID).
		Scan(&c.Platform, &c.Type, &raw)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c.Credentials); err != nil {
		return c, err
	}
	c.BaseURL, _ = c.Credentials["base_url"].(string)
	c.APIKey, _ = c.Credentials["api_key"].(string)
	if strings.TrimSpace(c.BaseURL) == "" {
		c.BaseURL = defaultBaseURL(c.Platform)
	}
	return c, nil
}

// IsNew 账号是否在创建后 days 天内（新接入）
func (a *CatAccount) IsNew(days int, now time.Time) bool {
	return days > 0 && !a.CreatedAt.IsZero() && now.Sub(a.CreatedAt) < time.Duration(days)*24*time.Hour
}

// Account 按 id 取账号（目录为空时返回 nil）
func (c *Catalog) Account(id int64) *CatAccount {
	if c == nil {
		return nil
	}
	return c.Accounts[id]
}
