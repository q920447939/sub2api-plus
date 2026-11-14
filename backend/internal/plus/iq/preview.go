// Package iq — [plus] 预览链接签名。
//
// 模型生成的 HTML 是不可信代码，用 <iframe sandbox="allow-scripts" src=...> 加载：
// iframe 不能带 Authorization 头，所以由已鉴权的接口签发短期链接（HMAC），预览接口只认签名。
// 不用 srcdoc 是因为 srcdoc 会继承站点的 CSP（禁止内联脚本），JS 动画无法运行。
package iq

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// PreviewScope 签发方：用户端只能看成功样本；管理端可以看任何有 HTML 的记录
type PreviewScope string

const (
	ScopeUser  PreviewScope = "u"
	ScopeAdmin PreviewScope = "a"
)

// PreviewBasePath 预览接口路径（不挂 JWT，靠签名鉴权）
const PreviewBasePath = "/api/v1/plus/iq-preview"

// Signer 签发 / 校验预览链接
type Signer struct {
	Secret []byte
	Now    func() time.Time
	Base   string // 预览接口路径，空 = PreviewBasePath（上游调度台用自己的路径和密钥）
}

func (s Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Signer) mac(id int64, scope PreviewScope, exp int64) string {
	h := hmac.New(sha256.New, s.Secret)
	_, _ = fmt.Fprintf(h, "%d.%s.%d", id, scope, exp)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// URL 签发链接。过期时间取整到小时（1~2 小时有效），同一小时内链接不变，浏览器可以缓存。
func (s Signer) URL(id int64, scope PreviewScope) string {
	exp := s.now().Truncate(time.Hour).Add(2 * time.Hour).Unix()
	base := s.Base
	if base == "" {
		base = PreviewBasePath
	}
	return fmt.Sprintf("%s/%d?sc=%s&e=%d&s=%s", base, id, scope, exp, s.mac(id, scope, exp))
}

// Verify 校验链接参数
func (s Signer) Verify(id int64, scope, expStr, sig string) (PreviewScope, bool) {
	sc := PreviewScope(scope)
	if sc != ScopeUser && sc != ScopeAdmin {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || s.now().Unix() > exp {
		return "", false
	}
	return sc, hmac.Equal([]byte(sig), []byte(s.mac(id, sc, exp)))
}
