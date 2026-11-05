// Package iq — [plus] 鹈鹕画图题：出题 + 从模型输出里提取可运行的 HTML（思路来自 pelican-iq 的 extract.js）
package iq

import (
	"errors"
	"regexp"
	"strings"
)

// PreviewCSP 注入到预览 HTML 的 CSP：只允许内联脚本/样式和 data/blob 资源，禁止一切外连。
// 预览接口的响应头里还有一份带 sandbox 的 CSP，两者同时生效（取交集），模型输出无法放宽。
const PreviewCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'"

const previewCSPMeta = `<meta http-equiv="Content-Security-Policy" content="` + PreviewCSP + `">`

// PelicanChallenge 本期唯一题型：只存档预览，不判分
type PelicanChallenge struct{}

func (PelicanChallenge) Prompt(cfg Config) string { return cfg.Prompt }

var (
	reFence      = regexp.MustCompile("(?is)```(?:html|svg)?\\s*(.*?)```")
	reDocStart   = regexp.MustCompile(`(?i)<!DOCTYPE\s+html|<html[\s>]|<svg[\s>]`)
	reHTMLTag    = regexp.MustCompile(`(?i)<html[\s>]`)
	reSVGStart   = regexp.MustCompile(`(?i)^<svg[\s>]`)
	reTrailFence = regexp.MustCompile("```\\s*$")
	reHead       = regexp.MustCompile(`(?i)<head[^>]*>`)
	reAttrURL    = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*["'](https?://[^"']+)["']`)
	reCSSURL     = regexp.MustCompile(`(?i)url\(\s*["']?(https?://[^"')]+)["']?\s*\)`)
)

// ErrNoHTML 输出里找不到可运行的 HTML / SVG
var ErrNoHTML = errors.New("响应中没有可运行的 HTML 或 SVG")

func (PelicanChallenge) Extract(raw string) (Artifact, error) {
	text := strings.TrimSpace(strings.TrimPrefix(raw, "\uFEFF"))
	if text == "" {
		return Artifact{}, errors.New("响应为空")
	}
	if m := reFence.FindStringSubmatch(text); m != nil && reDocStart.MatchString(m[1]) {
		text = strings.TrimSpace(m[1])
	}
	loc := reDocStart.FindStringIndex(text)
	if loc == nil {
		return Artifact{}, ErrNoHTML
	}
	html := strings.TrimSpace(reTrailFence.ReplaceAllString(strings.TrimSpace(text[loc[0]:]), ""))
	if reSVGStart.MatchString(html) && !reHTMLTag.MatchString(html) {
		html = `<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><style>html,body{margin:0;height:100%;background:#f4fbf8}svg{width:100%;height:100%;display:block}</style></head><body>` + html + `</body></html>`
	}
	return Artifact{HTML: html, ExternalRefs: FindExternalRefs(html)}, nil
}

// WithPreviewCSP 在 <head> 开头注入 CSP（必须在任何内容之前生效）
func WithPreviewCSP(html string) string {
	if loc := reHead.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + previewCSPMeta + html[loc[1]:]
	}
	return `<!DOCTYPE html><html><head><meta charset="utf-8">` + previewCSPMeta + `</head><body>` + html + `</body></html>`
}

// FindExternalRefs 找出引用的外部资源（预览时会被 CSP 拦截，管理端提示一下）
func FindExternalRefs(html string) []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range []*regexp.Regexp{reAttrURL, reCSSURL} {
		for _, m := range re.FindAllStringSubmatch(html, -1) {
			if !seen[m[1]] && len(out) < 8 {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}
