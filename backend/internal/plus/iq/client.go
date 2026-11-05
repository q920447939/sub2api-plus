// Package iq — [plus] 多协议流式生成客户端（Adapter）。
//
// 鹈鹕题一次要输出上万 token、耗时几分钟，所以统一用流式请求：首包很快返回，
// 网关的 X-Client-Request-ID 响应头也能马上拿到。每种协议的请求体和事件解析是一个 codec（Strategy）。
//
// 与 channelstatus 一样，目标固定是管理员配置的本站网关（常见是内网地址），因此不用上游的 SSRF 拦截客户端；
// 为了不变成任意请求代理：只允许 http/https、不跟随重定向、不读环境代理、响应体限长。
package iq

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientRequestIDHeader = "X-Client-Request-ID"
	maxTextBytes          = 2 << 20  // 正文上限 2MB，超出视为异常输出
	maxErrorBodyBytes     = 64 << 10 // 错误响应只读 64KB
	maxSSELineBytes       = 4 << 20
)

var errRedirect = errors.New("网关重定向了请求（不跟随重定向）")

var defaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 5 * time.Minute, // 思考型模型首包可能很慢；总超时由 ctx 控制
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirect },
}

// StreamClient 实现 LLMClient
type StreamClient struct {
	HTTP *http.Client // 为 nil 时用默认客户端（单测可注入）
}

var _ LLMClient = StreamClient{}

// HTTPError 网关返回非 2xx
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("网关返回 HTTP %d：%s", e.Status, e.Message)
}

func (c StreamClient) Complete(ctx context.Context, req CompletionRequest) (Completion, error) {
	codec, err := codecFor(req.Target.Format)
	if err != nil {
		return Completion{}, err
	}
	if u, err := url.Parse(req.Target.Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Completion{}, errors.New("网关地址格式不正确，需以 http:// 或 https:// 开头")
	}
	// 部分上游不认识输出上限字段：遇到相关 400 时换一种写法重试一次
	for variant := 0; ; variant++ {
		comp, err := c.do(ctx, codec, req, variant)
		var he *HTTPError
		if errors.As(err, &he) && he.Status == http.StatusBadRequest && variant < codec.capVariants()-1 && mentionsTokenCap(he.Message) {
			continue
		}
		return comp, err
	}
}

func mentionsTokenCap(msg string) bool {
	m := strings.ToLower(msg)
	for _, s := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens", "maxoutputtokens", "unsupported parameter"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

func (c StreamClient) do(ctx context.Context, codec codec, req CompletionRequest, variant int) (Completion, error) {
	body, err := json.Marshal(codec.body(req, variant))
	if err != nil {
		return Completion{}, err
	}
	endpoint := strings.TrimRight(req.Target.Endpoint, "/") + codec.path(req.Target.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Completion{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range codec.headers(req.Target.APIKey) {
		httpReq.Header.Set(k, v)
	}
	hc := c.HTTP
	if hc == nil {
		hc = defaultHTTPClient
	}
	resp, err := hc.Do(httpReq)
	if err != nil {
		return Completion{}, friendlyNetErr(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	comp := Completion{ClientRequestID: strings.TrimSpace(resp.Header.Get(clientRequestIDHeader)), StatusCode: resp.StatusCode}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return comp, &HTTPError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}

	acc := &accum{}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		err = readSSE(resp.Body, func(event string, data []byte) error { return codec.onEvent(event, data, acc) })
	} else {
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxTextBytes+1))
		if rerr == nil {
			err = codec.parseJSON(raw, acc)
		} else {
			err = rerr
		}
	}
	comp.Text, comp.Thinking = acc.text.String(), acc.thinking.String()
	comp.InputTokens, comp.OutputTokens = acc.in, acc.out
	if err != nil {
		return comp, friendlyNetErr(ctx, err)
	}
	if acc.errMsg != "" {
		return comp, errors.New("上游流中返回错误：" + acc.errMsg)
	}
	if acc.text.Len() > maxTextBytes {
		return comp, errors.New("输出超过 2MB，已中止")
	}
	return comp, nil
}

func friendlyNetErr(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("生成超时")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return errors.New("已取消")
	}
	return err
}

// errorMessage 从错误响应里取出可读的信息
func errorMessage(raw []byte) string {
	var v struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &v) == nil {
		var obj struct {
			Message string `json:"message"`
		}
		if len(v.Error) > 0 && json.Unmarshal(v.Error, &obj) == nil && obj.Message != "" {
			return truncate(obj.Message, 500)
		}
		var s string
		if len(v.Error) > 0 && json.Unmarshal(v.Error, &s) == nil && s != "" {
			return truncate(s, 500)
		}
		if v.Message != "" {
			return truncate(v.Message, 500)
		}
	}
	return truncate(strings.TrimSpace(string(raw)), 500)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// readSSE 逐个事件回调；兼容只有 data: 行的流（OpenAI Chat）
func readSSE(r io.Reader, fn func(event string, data []byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxSSELineBytes)
	var (
		event string
		data  bytes.Buffer
		total int
	)
	flush := func() error {
		defer func() { event = ""; data.Reset() }()
		if data.Len() == 0 {
			return nil
		}
		d := bytes.TrimSpace(data.Bytes())
		if bytes.Equal(d, []byte("[DONE]")) {
			return nil
		}
		return fn(event, d)
	}
	for sc.Scan() {
		line := sc.Bytes()
		total += len(line)
		if total > maxTextBytes*4 {
			return errors.New("流式响应过大，已中止")
		}
		switch {
		case len(line) == 0:
			if err := flush(); err != nil {
				return err
			}
		case bytes.HasPrefix(line, []byte("event:")):
			event = strings.TrimSpace(string(line[6:]))
		case bytes.HasPrefix(line, []byte("data:")):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.Write(bytes.TrimPrefix(line[5:], []byte(" ")))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return flush()
}

// accum 流式累积
type accum struct {
	text     strings.Builder
	thinking strings.Builder
	in, out  *int
	errMsg   string
}

func intp(v int) *int { return &v }
