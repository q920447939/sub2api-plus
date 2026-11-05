// Package iq — [plus] 读取分组网关的模型列表，给管理端新建任务时做候选
package iq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ListModels GET {endpoint}/v1/models（gemini 为 /v1beta/models）
func (c StreamClient) ListModels(ctx context.Context, t Target) ([]string, error) {
	path, header := "/v1/models", map[string]string{"Authorization": "Bearer " + t.APIKey, "x-api-key": t.APIKey}
	if t.Format == FormatGemini {
		path, header = "/v1beta/models", map[string]string{"x-goog-api-key": t.APIKey}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.Endpoint, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if t.Format == FormatAnthropic {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	hc := c.HTTP
	if hc == nil {
		hc = defaultHTTPClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, errorMessage(raw))
	}
	var v struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("解析模型列表失败：%w", err)
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, m := range v.Data {
		add(m.ID)
	}
	for _, m := range v.Models {
		add(m.Name)
	}
	sort.Strings(out)
	return out, nil
}
