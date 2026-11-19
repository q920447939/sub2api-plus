// plus_email_verified_hook.go — [plus] 邮箱已验证的回调桥。
// 上游在「用验证码证明了邮箱归属」的流程成功后调用 plusAfterEmailVerified（注册、第三方登录补邮箱、绑定 / 更换邮箱），
// plus 邮箱验证模块通过 SetPlusEmailVerifiedHook 记录验证。回调异常只记日志，绝不影响上游流程。
package service

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// 验证来源
const (
	PlusEmailVerifiedRegister = "register" // 邮箱注册时填了验证码
	PlusEmailVerifiedOAuth    = "oauth"    // 第三方登录注册时验证了本地邮箱，或第三方返回的已验证邮箱
	PlusEmailVerifiedBind     = "bind"     // 绑定 / 更换邮箱时填了验证码
)

// PlusEmailVerifiedEvent 一次已完成的邮箱验证
type PlusEmailVerifiedEvent struct {
	UserID int64
	Email  string
	Source string
}

type plusEmailVerifiedHookFn func(ctx context.Context, ev PlusEmailVerifiedEvent)

var plusEmailVerifiedHook atomic.Pointer[plusEmailVerifiedHookFn]

// SetPlusEmailVerifiedHook 注册回调；传 nil 取消
func SetPlusEmailVerifiedHook(fn func(ctx context.Context, ev PlusEmailVerifiedEvent)) {
	if fn == nil {
		plusEmailVerifiedHook.Store(nil)
		return
	}
	f := plusEmailVerifiedHookFn(fn)
	plusEmailVerifiedHook.Store(&f)
}

func plusAfterEmailVerified(ctx context.Context, userID int64, email, source string) {
	fn := plusEmailVerifiedHook.Load()
	if fn == nil || userID <= 0 || strings.TrimSpace(email) == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plus] email verified hook panic: %v", r)
		}
	}()
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	(*fn)(hctx, PlusEmailVerifiedEvent{UserID: userID, Email: strings.ToLower(strings.TrimSpace(email)), Source: source})
}
