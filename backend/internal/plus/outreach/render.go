// Package outreach — 渲染管线（模板方法）：变量替换 → Markdown → 外壳（页头 / 版式 / 页脚品牌区）→ 合规区。
//
// 编辑器预览、批次逐人预览、测试邮件、实际发送全部走 renderMail，保证看到的就是发出的。
// 合规区由目的的 Policy 决定、系统强制追加，外壳无法去掉：营销邮件一定带退订链接。
// Markdown 先整体转义再生成标签，不会注入 HTML。
package outreach

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`{{\s*([a-zA-Z][a-zA-Z0-9_]*)\s*}}`)

// substitute 替换变量；未知变量原样保留（保存时已校验，这里只是兜底）
func substitute(s string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		if val, ok := vars[placeholderRe.FindStringSubmatch(m)[1]]; ok {
			return val
		}
		return m
	})
}

var (
	boldRe   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRe = regexp.MustCompile(`(^|[^*])\*([^*\s][^*]*?)\*`)
	codeRe   = regexp.MustCompile("`([^`]+)`")
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	buttonRe = regexp.MustCompile(`(?m)^\[\[([^\]]+)\]\]\((\S+?)\)\s*$`)
)

func safeURL(u string) bool {
	low := strings.ToLower(u)
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "mailto:")
}

func inline(escaped, color string) string {
	s := codeRe.ReplaceAllString(escaped, `<code style="background:#f3f4f6;padding:1px 5px;border-radius:3px;">$1</code>`)
	s = boldRe.ReplaceAllString(s, `<strong>$1</strong>`)
	s = italicRe.ReplaceAllString(s, `$1<em>$2</em>`)
	return linkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := linkRe.FindStringSubmatch(m)
		u := html.UnescapeString(parts[2]) // 先还原再校验协议，输出时再转义
		if !safeURL(u) {
			return parts[1]
		}
		return fmt.Sprintf(`<a href="%s" style="color:%s;">%s</a>`, html.EscapeString(u), color, parts[1])
	})
}

// markdownToHTML 支持：标题(#~###)、无序列表、引用、分隔线、段落、**粗体**、*斜体*、`代码`、[链接](url)、独占一行的 [[按钮文字]](url)
func markdownToHTML(md, color string) string {
	// 按钮：先抽出占位，转换后再放回，避免被段落逻辑拆开
	var buttons []string
	md = buttonRe.ReplaceAllStringFunc(md, func(m string) string {
		p := buttonRe.FindStringSubmatch(m)
		low := strings.ToLower(p[2])
		if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
			return p[1]
		}
		buttons = append(buttons, fmt.Sprintf(
			`<p style="margin:22px 0;text-align:center;"><a href="%s" style="display:inline-block;padding:12px 32px;background:%s;color:#ffffff;text-decoration:none;border-radius:6px;font-size:15px;">%s</a></p>`,
			html.EscapeString(p[2]), color, html.EscapeString(p[1])))
		return fmt.Sprintf("BTNPLACEHOLDER%d", len(buttons)-1)
	})

	var out strings.Builder
	var para, list []string
	flushPara := func() {
		if len(para) > 0 {
			out.WriteString(`<p style="margin:0 0 14px;color:#374151;line-height:1.7;font-size:15px;">` + strings.Join(para, "<br>") + "</p>")
			para = nil
		}
	}
	flushList := func() {
		if len(list) > 0 {
			out.WriteString(`<ul style="margin:0 0 14px;padding-left:22px;color:#374151;line-height:1.7;font-size:15px;">`)
			for _, it := range list {
				out.WriteString("<li>" + it + "</li>")
			}
			out.WriteString("</ul>")
			list = nil
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		trim := strings.TrimSpace(raw)
		switch {
		case trim == "":
			flushPara()
			flushList()
		case strings.HasPrefix(trim, "BTNPLACEHOLDER"):
			flushPara()
			flushList()
			out.WriteString(trim)
		case trim == "---":
			flushPara()
			flushList()
			out.WriteString(`<hr style="border:none;border-top:1px solid #e5e7eb;margin:20px 0;">`)
		case strings.HasPrefix(trim, "### "), strings.HasPrefix(trim, "## "), strings.HasPrefix(trim, "# "):
			flushPara()
			flushList()
			level := strings.IndexByte(trim, ' ')
			size := map[int]string{1: "22px", 2: "18px", 3: "16px"}[level]
			fmt.Fprintf(&out, `<h%d style="margin:0 0 12px;font-size:%s;color:#111827;">%s</h%d>`,
				level, size, inline(html.EscapeString(strings.TrimSpace(trim[level:])), color), level)
		case strings.HasPrefix(trim, "- "), strings.HasPrefix(trim, "* "):
			flushPara()
			list = append(list, inline(html.EscapeString(strings.TrimSpace(trim[2:])), color))
		case strings.HasPrefix(trim, "> "):
			flushPara()
			flushList()
			out.WriteString(`<blockquote style="margin:0 0 14px;padding:10px 14px;background:#f9fafb;border-left:3px solid #d1d5db;color:#4b5563;font-size:14px;">` +
				inline(html.EscapeString(strings.TrimSpace(trim[2:])), color) + "</blockquote>")
		default:
			flushList()
			para = append(para, inline(html.EscapeString(trim), color))
		}
	}
	flushPara()
	flushList()
	s := out.String()
	for i, b := range buttons {
		s = strings.Replace(s, fmt.Sprintf("BTNPLACEHOLDER%d", i), b, 1)
	}
	return s
}

// MailInput 渲染一封邮件的全部输入
type MailInput struct {
	Policy   Policy
	Subject  string
	BodyMD   string
	Layout   LayoutConfig
	Vars     map[string]string
	SiteName string
	UnsubURL string
}

// renderMail 渲染管线入口
func renderMail(in MailInput) (subject, htmlBody string) {
	lay := in.Layout.withDefaults()
	subject = strings.TrimSpace(substitute(in.Subject, in.Vars))
	content := markdownToHTML(substitute(in.BodyMD, in.Vars), lay.ThemeColor)

	header := renderHeader(lay, in.SiteName)
	brand := ""
	if f := strings.TrimSpace(lay.FooterMD); f != "" {
		brand = markdownToHTML(substitute(f, in.Vars), "#9ca3af")
		// 页脚用小号灰字
		brand = strings.NewReplacer("color:#374151;line-height:1.7;font-size:15px;", "color:#9ca3af;line-height:1.6;font-size:12px;").Replace(brand)
	}
	compliance := complianceBlock(in.Policy, in.SiteName, in.UnsubURL)

	var mainBlock string
	if lay.Style == StylePlain {
		mainBlock = fmt.Sprintf(`<div style="padding:8px 4px;">%s</div>`, content)
	} else {
		mainBlock = fmt.Sprintf(`<div style="background:#ffffff;border-radius:8px;padding:32px 28px;border-top:4px solid %s;">%s</div>`, lay.ThemeColor, content)
	}
	bg := "#f5f5f5"
	if lay.Style == StylePlain {
		bg = "#ffffff"
	}
	htmlBody = fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="UTF-8"><title>%s</title></head>
<body style="margin:0;padding:0;background:%s;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'PingFang SC','Microsoft YaHei',sans-serif;">
<div style="max-width:%dpx;margin:0 auto;padding:24px;">
%s%s
<div style="margin:16px 0 0;text-align:center;">%s</div>
<p style="margin:8px 0 0;text-align:center;color:#9ca3af;font-size:12px;line-height:1.6;">%s</p>
</div></body></html>`, html.EscapeString(subject), bg, lay.Width, header, mainBlock, brand, compliance)
	return subject, htmlBody
}

func renderHeader(lay LayoutConfig, siteName string) string {
	var inner string
	switch lay.Header {
	case HeaderLogo:
		alt := lay.HeaderName
		if alt == "" {
			alt = siteName
		}
		inner = fmt.Sprintf(`<img src="%s" alt="%s" style="max-height:40px;border:0;">`, html.EscapeString(lay.LogoURL), html.EscapeString(alt))
	case HeaderText:
		label := lay.HeaderName
		if label == "" {
			label = siteName
		}
		inner = fmt.Sprintf(`<span style="font-size:18px;font-weight:600;color:%s;">%s</span>`, lay.ThemeColor, html.EscapeString(label))
	default:
		return ""
	}
	if lay.HeaderLink != "" {
		inner = fmt.Sprintf(`<a href="%s" style="text-decoration:none;">%s</a>`, html.EscapeString(lay.HeaderLink), inner)
	}
	return `<div style="margin:0 0 16px;text-align:center;">` + inner + `</div>`
}

// complianceBlock 合规区：由目的的合规类别决定，外壳无法修改
func complianceBlock(p Policy, siteName, unsubURL string) string {
	if p.Marketing() {
		return fmt.Sprintf(`您收到这封邮件，是因为您在 %s 注册了账号。不想再收到此类邮件？<a href="%s" style="color:#9ca3af;">点此退订</a>`,
			html.EscapeString(siteName), html.EscapeString(unsubURL))
	}
	return `这是一封与您账号相关的服务邮件。`
}
