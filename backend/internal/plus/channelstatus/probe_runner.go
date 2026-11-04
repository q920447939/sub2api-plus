// Package channelstatus — [plus] 探测执行器（Adapter）：把探测格式映射到上游 V1 的 provider / api_mode，
// 复用上游的算术 challenge 与请求/响应适配（小 max_tokens，单次只耗十几个 token）。
//
// 与 V1 不同，这里不使用上游的 SSRF 拦截客户端：探测目标固定是管理员配置的本站网关，
// 常见部署是内网 IP（如 http://192.168.1.141:8090）或解析到内网的域名，拦截内网会让探测永远失败。
// 为了不把它变成任意请求代理，本客户端：只发往配置的网关地址、只允许 http/https、不跟随重定向、
// 不读环境代理、响应体限长。
// 以后换实现（如走内部调用、不经 HTTP）只需新增一个 ProbeRunner。
package channelstatus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ProbeRequest 一次探测
type ProbeRequest struct {
	Endpoint string
	APIKey   string
	Model    string
	Format   ProbeFormat // 已解析，不会是 FormatAuto
}

// ProbeResult 探测结果
type ProbeResult struct {
	OK        bool
	LatencyMs int
	Message   string
}

// ProbeRunner 执行一次探测
type ProbeRunner interface {
	Run(ctx context.Context, req ProbeRequest) ProbeResult
}

// errRedirect 网关不应重定向探测请求；拒绝跟随，避免被引到配置以外的地址
var errRedirect = errors.New("gateway redirected the probe request (redirects are not followed)")

// probeHTTPClient 允许内网地址的专用客户端；超时由调用方 ctx 控制
var probeHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil, // 不走环境代理，内网网关直连
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirect },
}

// GatewayRunner 通过本站网关发探测请求，路径与用户真实调用一致
type GatewayRunner struct {
	Client *http.Client // 为 nil 时用 probeHTTPClient（单测可注入）
}

func (r GatewayRunner) Run(ctx context.Context, req ProbeRequest) ProbeResult {
	provider, apiMode := providerFor(req.Format)
	if u, err := url.Parse(req.Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ProbeResult{Message: "网关地址格式不正确，需以 http:// 或 https:// 开头"}
	}
	pr, err := service.PlusMonitorBuildProbe(provider, apiMode, req.Endpoint, req.APIKey, req.Model)
	if err != nil {
		return ProbeResult{Message: "build request: " + err.Error()}
	}

	start := time.Now()
	status, body, reqErr := r.post(ctx, pr)
	lat := int(time.Since(start) / time.Millisecond)

	ok, msg := service.PlusMonitorJudgeProbe(provider, apiMode, pr, status, body, reqErr)
	return ProbeResult{OK: ok, LatencyMs: lat, Message: msg}
}

func (r GatewayRunner) post(ctx context.Context, pr *service.PlusMonitorProbeRequest) (int, []byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, pr.URL, bytes.NewReader(pr.Body))
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for k, v := range pr.Headers {
		httpReq.Header.Set(k, v)
	}
	client := r.Client
	if client == nil {
		client = probeHTTPClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return 0, nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, pr.MaxBytes))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, body, nil
}

func providerFor(f ProbeFormat) (provider, apiMode string) {
	provider, apiMode = service.MonitorProviderOpenAI, service.MonitorAPIModeChatCompletions
	switch f {
	case FormatAnthropic:
		provider = service.MonitorProviderAnthropic
	case FormatGemini:
		provider = service.MonitorProviderGemini
	case FormatOpenAIResponses:
		apiMode = service.MonitorAPIModeResponses
	}
	return provider, apiMode
}
