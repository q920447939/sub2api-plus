// Package iq — [plus] 各协议的请求体与流事件解析（Strategy）
package iq

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type codec interface {
	path(model string) string
	headers(apiKey string) map[string]string
	// body variant 用于输出上限字段的兼容重试（0 为首选写法）
	body(req CompletionRequest, variant int) map[string]any
	capVariants() int
	onEvent(event string, data []byte, acc *accum) error
	parseJSON(raw []byte, acc *accum) error
}

func codecFor(f Format) (codec, error) {
	switch f {
	case FormatAnthropic:
		return anthropicCodec{}, nil
	case FormatOpenAIChat:
		return chatCodec{}, nil
	case FormatOpenAIResponses:
		return responsesCodec{}, nil
	case FormatGemini:
		return geminiCodec{}, nil
	}
	return nil, fmt.Errorf("不支持的请求格式 %q", f)
}

func userText(prompt string) []map[string]any {
	return []map[string]any{{"role": "user", "content": prompt}}
}

// ---------- Anthropic Messages ----------

type anthropicCodec struct{}

func (anthropicCodec) path(string) string { return "/v1/messages" }
func (anthropicCodec) headers(k string) map[string]string {
	return map[string]string{"x-api-key": k, "anthropic-version": "2023-06-01"}
}
func (anthropicCodec) capVariants() int { return 1 }
func (anthropicCodec) body(req CompletionRequest, _ int) map[string]any {
	b := map[string]any{"model": req.Target.Model, "max_tokens": req.MaxTokens, "stream": true, "messages": userText(req.Prompt)}
	if budget := req.Thinking.budget(); budget > 0 {
		b["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
		b["max_tokens"] = req.MaxTokens + budget
	}
	return b
}

type anthropicEvent struct {
	Type    string `json:"type"`
	Message struct {
		Usage struct {
			InputTokens  *int `json:"input_tokens"`
			OutputTokens *int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"delta"`
	Usage struct {
		InputTokens  *int `json:"input_tokens"`
		OutputTokens *int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"content"`
}

func (anthropicCodec) onEvent(_ string, data []byte, acc *accum) error {
	var ev anthropicEvent
	if json.Unmarshal(data, &ev) != nil {
		return nil
	}
	switch ev.Type {
	case "message_start":
		if ev.Message.Usage.InputTokens != nil {
			acc.in = ev.Message.Usage.InputTokens
		}
	case "content_block_delta":
		acc.text.WriteString(ev.Delta.Text)
		acc.thinking.WriteString(ev.Delta.Thinking)
	case "message_delta":
		if ev.Usage.OutputTokens != nil {
			acc.out = ev.Usage.OutputTokens
		}
		if ev.Usage.InputTokens != nil && acc.in == nil {
			acc.in = ev.Usage.InputTokens
		}
	case "error":
		acc.errMsg = ev.Error.Message
	}
	return nil
}

func (anthropicCodec) parseJSON(raw []byte, acc *accum) error {
	var ev anthropicEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("解析响应失败：%w", err)
	}
	for _, c := range ev.Content {
		acc.text.WriteString(c.Text)
		acc.thinking.WriteString(c.Thinking)
	}
	acc.in, acc.out = ev.Usage.InputTokens, ev.Usage.OutputTokens
	return nil
}

// ---------- OpenAI Chat Completions ----------

type chatCodec struct{}

func (chatCodec) path(string) string { return "/v1/chat/completions" }
func (chatCodec) headers(k string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + k}
}
func (chatCodec) capVariants() int { return 3 }
func (chatCodec) body(req CompletionRequest, variant int) map[string]any {
	b := map[string]any{
		"model": req.Target.Model, "stream": true, "messages": userText(req.Prompt),
		"stream_options": map[string]any{"include_usage": true},
	}
	switch variant {
	case 0:
		b["max_tokens"] = req.MaxTokens
	case 1:
		b["max_completion_tokens"] = req.MaxTokens
	}
	if req.Thinking != ThinkingOff && req.Thinking != "" {
		b["reasoning_effort"] = string(req.Thinking)
	}
	return b
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     *int `json:"prompt_tokens"`
		CompletionTokens *int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c chatCodec) onEvent(_ string, data []byte, acc *accum) error {
	var ch chatChunk
	if json.Unmarshal(data, &ch) != nil {
		return nil
	}
	for _, x := range ch.Choices {
		acc.text.WriteString(x.Delta.Content)
		acc.thinking.WriteString(x.Delta.ReasoningContent)
	}
	c.common(ch, acc)
	return nil
}

func (c chatCodec) parseJSON(raw []byte, acc *accum) error {
	var ch chatChunk
	if err := json.Unmarshal(raw, &ch); err != nil {
		return fmt.Errorf("解析响应失败：%w", err)
	}
	for _, x := range ch.Choices {
		acc.text.WriteString(x.Message.Content)
		acc.thinking.WriteString(x.Message.ReasoningContent)
	}
	c.common(ch, acc)
	return nil
}

func (chatCodec) common(ch chatChunk, acc *accum) {
	if ch.Usage != nil {
		acc.in, acc.out = ch.Usage.PromptTokens, ch.Usage.CompletionTokens
	}
	if ch.Error != nil && ch.Error.Message != "" {
		acc.errMsg = ch.Error.Message
	}
}

// ---------- OpenAI Responses ----------

type responsesCodec struct{}

func (responsesCodec) path(string) string { return "/v1/responses" }
func (responsesCodec) headers(k string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + k}
}
func (responsesCodec) capVariants() int { return 2 }
func (responsesCodec) body(req CompletionRequest, variant int) map[string]any {
	b := map[string]any{
		"model":  req.Target.Model,
		"stream": true,
		"input": []map[string]any{{
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": req.Prompt}},
		}},
	}
	if variant == 0 {
		b["max_output_tokens"] = req.MaxTokens
	}
	if req.Thinking != ThinkingOff && req.Thinking != "" {
		b["reasoning"] = map[string]any{"effort": string(req.Thinking)}
	}
	return b
}

type responsesUsage struct {
	InputTokens  *int `json:"input_tokens"`
	OutputTokens *int `json:"output_tokens"`
}

type responsesObj struct {
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage *responsesUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r responsesCodec) onEvent(event string, data []byte, acc *accum) error {
	var ev struct {
		Type     string        `json:"type"`
		Delta    string        `json:"delta"`
		Response *responsesObj `json:"response"`
		Message  string        `json:"message"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return nil
	}
	if ev.Type == "" {
		ev.Type = event
	}
	switch ev.Type {
	case "response.output_text.delta":
		acc.text.WriteString(ev.Delta)
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		acc.thinking.WriteString(ev.Delta)
	case "response.completed", "response.incomplete":
		if ev.Response != nil && ev.Response.Usage != nil {
			acc.in, acc.out = ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens
		}
		// 某些实现不发 delta，只在 completed 里带完整输出
		if acc.text.Len() == 0 && ev.Response != nil {
			r.collect(ev.Response, acc)
		}
	case "response.failed":
		if ev.Response != nil && ev.Response.Error != nil {
			acc.errMsg = ev.Response.Error.Message
		} else {
			acc.errMsg = "response.failed"
		}
	case "error":
		acc.errMsg = ev.Message
		if acc.errMsg == "" {
			acc.errMsg = string(data)
		}
	}
	return nil
}

func (responsesCodec) collect(o *responsesObj, acc *accum) {
	for _, item := range o.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "output_text" {
				acc.text.WriteString(c.Text)
			}
		}
	}
}

func (r responsesCodec) parseJSON(raw []byte, acc *accum) error {
	var o responsesObj
	if err := json.Unmarshal(raw, &o); err != nil {
		return fmt.Errorf("解析响应失败：%w", err)
	}
	r.collect(&o, acc)
	if o.Usage != nil {
		acc.in, acc.out = o.Usage.InputTokens, o.Usage.OutputTokens
	}
	if o.Error != nil {
		acc.errMsg = o.Error.Message
	}
	return nil
}

// ---------- Gemini ----------

type geminiCodec struct{}

func (geminiCodec) path(model string) string {
	return "/v1beta/models/" + url.PathEscape(model) + ":streamGenerateContent?alt=sse"
}
func (geminiCodec) headers(k string) map[string]string { return map[string]string{"x-goog-api-key": k} }
func (geminiCodec) capVariants() int                   { return 1 }
func (geminiCodec) body(req CompletionRequest, _ int) map[string]any {
	gen := map[string]any{"maxOutputTokens": req.MaxTokens}
	if budget := req.Thinking.budget(); budget > 0 {
		gen["thinkingConfig"] = map[string]any{"thinkingBudget": budget, "includeThoughts": true}
		gen["maxOutputTokens"] = req.MaxTokens + budget
	}
	return map[string]any{
		"contents":         []map[string]any{{"role": "user", "parts": []map[string]any{{"text": req.Prompt}}}},
		"generationConfig": gen,
	}
}

type geminiChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     *int `json:"promptTokenCount"`
		CandidatesTokenCount *int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (geminiCodec) apply(ch geminiChunk, acc *accum) {
	for _, c := range ch.Candidates {
		for _, p := range c.Content.Parts {
			if p.Thought {
				acc.thinking.WriteString(p.Text)
			} else {
				acc.text.WriteString(p.Text)
			}
		}
	}
	if ch.UsageMetadata != nil {
		acc.in, acc.out = ch.UsageMetadata.PromptTokenCount, ch.UsageMetadata.CandidatesTokenCount
	}
	if ch.Error != nil {
		acc.errMsg = ch.Error.Message
	}
}

func (g geminiCodec) onEvent(_ string, data []byte, acc *accum) error {
	var ch geminiChunk
	if json.Unmarshal(data, &ch) == nil {
		g.apply(ch, acc)
	}
	return nil
}

func (g geminiCodec) parseJSON(raw []byte, acc *accum) error {
	// 非流式时可能返回数组
	var arr []geminiChunk
	if json.Unmarshal(raw, &arr) == nil {
		for _, ch := range arr {
			g.apply(ch, acc)
		}
		return nil
	}
	var ch geminiChunk
	if err := json.Unmarshal(raw, &ch); err != nil {
		return fmt.Errorf("解析响应失败：%w", err)
	}
	g.apply(ch, acc)
	return nil
}
