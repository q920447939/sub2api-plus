//go:build unit

package channelstatus

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var lastQ = regexp.MustCompile(`Q: (\d+) ([+-]) (\d+) = \?\s*A:\s*$`)

// fakeGateway 监听 127.0.0.1（内网/回环地址），按 Chat Completions 格式回答算术题
func fakeGateway(t *testing.T, wrong bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"bad auth"}}`)
			return
		}
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m := lastQ.FindStringSubmatch(body.Messages[0].Content)
		if m == nil {
			t.Errorf("unexpected prompt: %q", body.Messages[0].Content)
			return
		}
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[3])
		ans := a + b
		if m[2] == "-" {
			ans = a - b
		}
		if wrong {
			ans += 1000
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"`+strconv.Itoa(ans)+`"}}]}`)
	}))
}

func TestGatewayRunner_AllowsIntranetAndPasses(t *testing.T) {
	srv := fakeGateway(t, false)
	defer srv.Close()
	res := GatewayRunner{}.Run(context.Background(), ProbeRequest{Endpoint: srv.URL, APIKey: "sk-test", Model: "m", Format: FormatOpenAIChat})
	if !res.OK {
		t.Fatalf("回环/内网网关应能探测成功, got %+v", res)
	}
}

func TestGatewayRunner_WrongAnswerFails(t *testing.T) {
	srv := fakeGateway(t, true)
	defer srv.Close()
	res := GatewayRunner{}.Run(context.Background(), ProbeRequest{Endpoint: srv.URL, APIKey: "sk-test", Model: "m", Format: FormatOpenAIChat})
	if res.OK || !strings.Contains(res.Message, "challenge mismatch") {
		t.Fatalf("答错应失败, got %+v", res)
	}
}

func TestGatewayRunner_HTTPErrorFails(t *testing.T) {
	srv := fakeGateway(t, false)
	defer srv.Close()
	res := GatewayRunner{}.Run(context.Background(), ProbeRequest{Endpoint: srv.URL, APIKey: "sk-bad", Model: "m", Format: FormatOpenAIChat})
	if res.OK || !strings.Contains(res.Message, "HTTP 401") {
		t.Fatalf("401 应失败并带状态码, got %+v", res)
	}
}

func TestGatewayRunner_DoesNotFollowRedirect(t *testing.T) {
	var hit bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redir.Close()
	res := GatewayRunner{}.Run(context.Background(), ProbeRequest{Endpoint: redir.URL, APIKey: "sk-test", Model: "m", Format: FormatOpenAIChat})
	if res.OK || hit {
		t.Fatalf("不应跟随重定向, res=%+v hit=%v", res, hit)
	}
}

func TestGatewayRunner_RejectsNonHTTPScheme(t *testing.T) {
	res := GatewayRunner{}.Run(context.Background(), ProbeRequest{Endpoint: "file:///etc/passwd", APIKey: "k", Model: "m", Format: FormatOpenAIChat})
	if res.OK || !strings.Contains(res.Message, "网关地址格式不正确") {
		t.Fatalf("非 http(s) 应拒绝, got %+v", res)
	}
}
