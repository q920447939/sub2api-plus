package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/plus/iq"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Target 一次请求打到哪里（账号直连：账号自己的地址和 Key，不经过本站调度）
type Target struct {
	AccountID int64
	BaseURL   string
	APIKey    string
	Model     string
	Platform  string
}

// Completion 一次生成的结果
type Completion struct {
	Text         string
	InputTokens  *int
	OutputTokens *int
}

// Completer 多协议生成客户端（端口）。检测器只依赖它，不关心协议细节。
type Completer interface {
	Complete(ctx context.Context, t Target, prompt string, maxTokens int) (Completion, error)
}

// iqCompleter 复用 plus/iq 的多协议流式客户端（Adapter）：
// anthropic → /v1/messages，openai → /v1/responses，gemini → 原生 streamGenerateContent
type iqCompleter struct{ client iq.LLMClient }

func newIQCompleter() Completer { return iqCompleter{client: iq.StreamClient{}} }

func (c iqCompleter) Complete(ctx context.Context, t Target, prompt string, maxTokens int) (Completion, error) {
	comp, err := c.client.Complete(ctx, iq.CompletionRequest{
		Target: iq.Target{
			Endpoint: normalizeBaseURL(t.BaseURL),
			APIKey:   t.APIKey,
			Model:    t.Model,
			Format:   iq.ResolveFormat(iq.FormatAuto, t.Platform),
		},
		Prompt:    prompt,
		MaxTokens: maxTokens,
		Thinking:  iq.ThinkingOff,
	})
	if err != nil {
		// iq 的报错是从「网关」视角写的；这里是账号直连，说清楚是账号的上游返回的
		var he *iq.HTTPError
		if errors.As(err, &he) {
			err = fmt.Errorf("账号上游返回 HTTP %d：%s", he.Status, he.Message)
		}
		return Completion{Text: comp.Text}, err
	}
	return Completion{Text: comp.Text, InputTokens: comp.InputTokens, OutputTokens: comp.OutputTokens}, nil
}

// DirectChannel 账号直连：读取账号的地址和 Key，按账号的模型映射换算模型名（和网关实际发出去的一致）
type DirectChannel struct{ Catalog CatalogSource }

func (d DirectChannel) Resolve(ctx context.Context, accountID int64, model string) (Target, error) {
	c, err := d.Catalog.Credential(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return Target{}, errors.New("账号已删除")
	}
	if err != nil {
		return Target{}, fmt.Errorf("读取账号凭证：%w", err)
	}
	if !directTypes[c.Type] {
		return Target{}, errors.New("不是 API Key 账号，暂不支持直连检测")
	}
	if c.APIKey == "" || c.BaseURL == "" {
		return Target{}, errors.New("账号没有配置地址或 API Key")
	}
	upstream := service.PlusAccountUpstreamModel(c.Platform, c.Type, c.Credentials, model)
	if upstream == "" {
		upstream = model
	}
	return Target{AccountID: accountID, BaseURL: c.BaseURL, APIKey: c.APIKey, Model: upstream, Platform: c.Platform}, nil
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
