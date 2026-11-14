package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ---------- 存档题：发一道开放题，只保存作品给人看，不判对错 ----------

// ArchiveParams 存档题参数：默认题面 + 按平台的题面
type ArchiveParams struct {
	Prompt    string            `json:"prompt"`
	Prompts   map[string]string `json:"prompts,omitempty"`
	MaxTokens int               `json:"max_tokens"`
}

func (p *ArchiveParams) promptFor(platform string) string {
	if v := strings.TrimSpace(p.Prompts[platform]); v != "" {
		return v
	}
	return p.Prompt
}

// ArchiveDetector 存档题
type ArchiveDetector struct{ LLM Completer }

var _ Detector = ArchiveDetector{}

func (ArchiveDetector) Spec() TypeSpec {
	return TypeSpec{
		Key: "archive", Name: "存档题", Gallery: true, HasPrompt: true, DefaultEvery: 30,
		Description: "发一道开放题（比如画一段 HTML 动画），只保存作品给人看，不判对错。",
		RuleFields:  []RuleField{},
		Outcomes: []OutcomeSpec{
			{Key: OutcomeSaved, Label: "已存档", Tone: "ok"},
			{Key: OutcomeError, Label: "生成失败", Tone: "err"},
		},
	}
}

func (ArchiveDetector) NormalizeItem(params, _ json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var p ArchiveParams
	if err := decodeInto(params, &p); err != nil {
		return nil, nil, err
	}
	p.Prompt = strings.TrimSpace(p.Prompt)
	if p.Prompt == "" {
		return nil, nil, fmt.Errorf("%w：题面不能为空", ErrInvalid)
	}
	if p.MaxTokens <= 0 {
		p.MaxTokens = 32000
	}
	return mustJSON(p), json.RawMessage(`{}`), nil
}

func (ArchiveDetector) Supports(context.Context, string, string) (bool, string) { return true, "" }

var (
	htmlFence = regexp.MustCompile("(?s)```(?:html|HTML)?\\s*\\n(.*?)```")
	htmlStart = regexp.MustCompile(`(?is)(<!doctype html.*|<html.*|<svg.*)`)
)

// extractHTML 从回答里提取作品
func extractHTML(text string) (string, error) {
	if m := htmlFence.FindAllStringSubmatch(text, -1); len(m) > 0 {
		best := ""
		for _, x := range m {
			if len(x[1]) > len(best) {
				best = x[1]
			}
		}
		if strings.Contains(strings.ToLower(best), "<") {
			return strings.TrimSpace(best), nil
		}
	}
	if m := htmlStart.FindString(text); m != "" {
		return strings.TrimSpace(m), nil
	}
	return "", errors.New("回答里没有找到 HTML / SVG 作品")
}

func (d ArchiveDetector) Execute(ctx context.Context, in ExecInput) ExecResult {
	var p ArchiveParams
	_ = json.Unmarshal(in.Item.Params, &p)
	prompt := p.promptFor(in.Platform)
	if p.MaxTokens <= 0 {
		p.MaxTokens = 32000
	}
	comp, err := d.LLM.Complete(ctx, in.Target, prompt, p.MaxTokens)
	if err != nil {
		r := errResult(prompt, err)
		r.Reply = truncateRunes(comp.Text, 4000)
		return r
	}
	html, err := extractHTML(comp.Text)
	res := ExecResult{Prompt: prompt, InputTokens: comp.InputTokens, OutputTokens: comp.OutputTokens}
	if err != nil {
		res.Outcome, res.Summary, res.Error = OutcomeError, "生成失败", err.Error()
		res.Reply = truncateRunes(comp.Text, 4000)
		return res
	}
	if len(html) > 512<<10 {
		res.Outcome, res.Summary, res.Error = OutcomeError, "生成失败", "作品超过 512KB"
		return res
	}
	res.Outcome, res.Summary, res.Reply = OutcomeSaved, fmt.Sprintf("作品已保存（%.1f KB）", float64(len(html))/1024), html
	res.Detail = map[string]any{"bytes": len(html)}
	return res
}

func (ArchiveDetector) Classify(_ Item, r Run) string { return r.Outcome }

func (ArchiveDetector) Judge(_ Item, rs []Run) *Verdict {
	n := 0
	for _, r := range rs {
		if r.Outcome == OutcomeSaved {
			n++
		}
	}
	return &Verdict{Level: LevelInfo, Short: fmt.Sprintf("%d 份作品", n), Long: fmt.Sprintf("最近存档 %d 份作品（不判对错）", n), Valid: n}
}
