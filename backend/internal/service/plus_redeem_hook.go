// plus_redeem_hook.go — [plus] 兑换成功后的回调桥。上游 redeem() 提交事务后调用 plusAfterRedeem，
// plus 模块通过 SetPlusRedeemHook 注册处理函数（如兑换送积分）。回调异常只记日志，绝不影响兑换结果。
package service

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// 兑换来源：用户自助兑换 / 在线支付履约 / 管理员代发
const (
	PlusRedeemSourceUser    = "user"
	PlusRedeemSourcePayment = "payment"
	PlusRedeemSourceAdmin   = "admin"
)

// PlusRedeemEvent 一次已提交的兑换
type PlusRedeemEvent struct {
	UserID int64
	CodeID int64
	Code   string
	Type   string
	Value  float64
	Source string
}

type plusRedeemHookFn func(ctx context.Context, ev PlusRedeemEvent)

var plusRedeemHook atomic.Pointer[plusRedeemHookFn]

// SetPlusRedeemHook 注册兑换回调；传 nil 取消
func SetPlusRedeemHook(fn func(ctx context.Context, ev PlusRedeemEvent)) {
	if fn == nil {
		plusRedeemHook.Store(nil)
		return
	}
	f := plusRedeemHookFn(fn)
	plusRedeemHook.Store(&f)
}

func plusAfterRedeem(ctx context.Context, userID int64, rc *RedeemCode, policy redeemRateLimitPolicy) {
	fn := plusRedeemHook.Load()
	if fn == nil || rc == nil {
		return
	}
	source := PlusRedeemSourceAdmin
	switch {
	case policy == enforceRedeemRateLimit:
		source = PlusRedeemSourceUser
	case ctx.Value(ctxKeySkipRedeemAffiliate{}) == true:
		source = PlusRedeemSourcePayment
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[plus] redeem hook panic: %v", r)
		}
	}()
	// 兑换已提交：回调不受请求取消影响，但限制耗时
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	(*fn)(hctx, PlusRedeemEvent{UserID: userID, CodeID: rc.ID, Code: rc.Code, Type: rc.Type, Value: rc.Value, Source: source})
}
