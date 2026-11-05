//go:build unit

package iq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type captured struct {
	path, auth string
	body       map[string]any
}

func sseServer(t *testing.T, cap *captured, events string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.path = r.URL.RequestURI()
		cap.auth = r.Header.Get("Authorization") + r.Header.Get("x-api-key") + r.Header.Get("x-goog-api-key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &cap.body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(clientRequestIDHeader, "req-123")
		_, _ = io.WriteString(w, events)
	}))
}

func TestStreamClientFormats(t *testing.T) {
	cases := []struct {
		format    Format
		events    string
		wantPath  string
		wantText  string
		wantOut   int
		checkBody func(map[string]any) bool
	}{
		{
			FormatAnthropic,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"<html>\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"</html>\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":34}}\n\n",
			"/v1/messages", "<html></html>", 34,
			func(b map[string]any) bool { return b["thinking"] != nil && b["max_tokens"].(float64) == 1000+2048 },
		},
		{
			FormatOpenAIChat,
			"data: {\"choices\":[{\"delta\":{\"content\":\"<html>\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"</html>\"}}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n",
			"/v1/chat/completions", "<html></html>", 7,
			func(b map[string]any) bool { return b["reasoning_effort"] == "low" && b["max_tokens"] != nil },
		},
		{
			FormatOpenAIResponses,
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"<html>\"}\n\n" +
				"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"</html>\"}\n\n" +
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":9}}}\n\n",
			"/v1/responses", "<html></html>", 9,
			func(b map[string]any) bool { return b["reasoning"] != nil && b["max_output_tokens"] != nil },
		},
		{
			FormatGemini,
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"think\",\"thought\":true},{\"text\":\"<html>\"}]}}]}\n\n" +
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"</html>\"}]}}],\"usageMetadata\":{\"promptTokenCount\":4,\"candidatesTokenCount\":11}}\n\n",
			"/v1beta/models/gemini-x:streamGenerateContent?alt=sse", "<html></html>", 11,
			func(b map[string]any) bool { return b["generationConfig"] != nil },
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.format), func(t *testing.T) {
			var cap captured
			srv := sseServer(t, &cap, tc.events)
			defer srv.Close()
			model := "m"
			if tc.format == FormatGemini {
				model = "gemini-x"
			}
			comp, err := StreamClient{}.Complete(context.Background(), CompletionRequest{
				Target: Target{Endpoint: srv.URL + "/", APIKey: "sk-test", Model: model, Format: tc.format},
				Prompt: "draw", MaxTokens: 1000, Thinking: ThinkingLow,
			})
			if err != nil {
				t.Fatal(err)
			}
			if cap.path != tc.wantPath {
				t.Fatalf("path = %s", cap.path)
			}
			if !strings.Contains(cap.auth, "sk-test") {
				t.Fatalf("auth header missing: %q", cap.auth)
			}
			if comp.Text != tc.wantText || comp.OutputTokens == nil || *comp.OutputTokens != tc.wantOut {
				t.Fatalf("comp = %+v", comp)
			}
			if comp.ClientRequestID != "req-123" {
				t.Fatalf("client request id = %q", comp.ClientRequestID)
			}
			if !tc.checkBody(cap.body) {
				t.Fatalf("unexpected body %v", cap.body)
			}
		})
	}
}

func TestStreamClientErrorKeepsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(clientRequestIDHeader, "req-err")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream down"}}`)
	}))
	defer srv.Close()
	comp, err := StreamClient{}.Complete(context.Background(), CompletionRequest{
		Target: Target{Endpoint: srv.URL, APIKey: "k", Model: "m", Format: FormatAnthropic}, Prompt: "x", MaxTokens: 10,
	})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 502 || he.Message != "upstream down" {
		t.Fatalf("err = %v", err)
	}
	if comp.ClientRequestID != "req-err" {
		t.Fatal("request id must be kept on error for attribution")
	}
}

func TestStreamClientRetriesTokenCapField(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &b)
		bodies = append(bodies, b)
		if _, ok := b["max_tokens"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Unsupported parameter: 'max_tokens'. Use 'max_completion_tokens' instead."}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()
	comp, err := StreamClient{}.Complete(context.Background(), CompletionRequest{
		Target: Target{Endpoint: srv.URL, APIKey: "k", Model: "m", Format: FormatOpenAIChat}, Prompt: "x", MaxTokens: 10,
	})
	if err != nil || comp.Text != "ok" {
		t.Fatalf("comp=%+v err=%v", comp, err)
	}
	if len(bodies) != 2 || bodies[1]["max_completion_tokens"] == nil {
		t.Fatalf("bodies = %v", bodies)
	}
}

func TestStreamClientStreamError(t *testing.T) {
	var cap captured
	srv := sseServer(t, &cap, "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"overloaded\"}}\n\n")
	defer srv.Close()
	_, err := StreamClient{}.Complete(context.Background(), CompletionRequest{
		Target: Target{Endpoint: srv.URL, APIKey: "k", Model: "m", Format: FormatAnthropic}, Prompt: "x", MaxTokens: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("err = %v", err)
	}
}

// 新模型要求 adaptive 思考：首选写法就是 adaptive + output_config.effort；
// 老模型拒绝 adaptive（400 提到 thinking）时退回 enabled + budget_tokens。
func TestAnthropicAdaptiveThinkingFallback(t *testing.T) {
	ok := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"<html></html>\"}}\n\n"
	for _, legacyOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacyOnly=%v", legacyOnly), func(t *testing.T) {
			var bodies []map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var b map[string]any
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &b)
				bodies = append(bodies, b)
				th, _ := b["thinking"].(map[string]any)
				if legacyOnly && th["type"] == "adaptive" {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":{"message":"thinking.type: Input should be 'enabled' or 'disabled'"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, ok)
			}))
			defer srv.Close()
			comp, err := StreamClient{}.Complete(context.Background(), CompletionRequest{
				Target: Target{Endpoint: srv.URL, APIKey: "k", Model: "claude-x", Format: FormatAnthropic},
				Prompt: "p", MaxTokens: 1000, Thinking: ThinkingHigh,
			})
			if err != nil || comp.Text != "<html></html>" {
				t.Fatalf("err=%v text=%q", err, comp.Text)
			}
			first, _ := bodies[0]["thinking"].(map[string]any)
			oc, _ := bodies[0]["output_config"].(map[string]any)
			if first["type"] != "adaptive" || oc["effort"] != "high" {
				t.Fatalf("first body = %v", bodies[0])
			}
			if !legacyOnly {
				if len(bodies) != 1 {
					t.Fatalf("expected 1 request, got %d", len(bodies))
				}
				return
			}
			if len(bodies) != 2 {
				t.Fatalf("expected retry, got %d requests", len(bodies))
			}
			second, _ := bodies[1]["thinking"].(map[string]any)
			if second["type"] != "enabled" || second["budget_tokens"].(float64) != 16384 || bodies[1]["output_config"] != nil {
				t.Fatalf("second body = %v", bodies[1])
			}
		})
	}
}

// 思考关闭时不带任何思考字段，也不会因为 400 去重试
func TestAnthropicNoThinkingNoRetry(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		var b map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &b)
		if b["thinking"] != nil || b["output_config"] != nil {
			t.Errorf("unexpected thinking fields: %v", b)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"requires adaptive thinking"}}`)
	}))
	defer srv.Close()
	_, err := StreamClient{}.Complete(context.Background(), CompletionRequest{
		Target: Target{Endpoint: srv.URL, APIKey: "k", Model: "claude-x", Format: FormatAnthropic},
		Prompt: "p", MaxTokens: 1000, Thinking: ThinkingOff,
	})
	if err == nil || n != 1 {
		t.Fatalf("err=%v requests=%d", err, n)
	}
}
