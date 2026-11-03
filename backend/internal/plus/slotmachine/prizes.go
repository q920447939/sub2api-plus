// Package slotmachine — [plus] 奖品类型注册表
//
// 每种奖品类型负责：参数规整与校验、价值折算（用于扣奖金罐和返奖率）、发奖。
// 新增奖品类型只需实现 PrizeKind 并在 init() 里 RegisterPrizeKind，抽奖主流程不用改。
package slotmachine

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/plus/points"
)

const (
	PrizeTypeNone    = "none"    // 谢谢惠顾
	PrizeTypePoints  = "points"  // 积分
	PrizeTypeBalance = "balance" // 余额（生成 SLOT-<16 位随机字符> 凭证码并直接到账）
)

// AwardInput 发奖上下文；在摇奖事务内调用
type AwardInput struct {
	Tx     *sql.Tx
	UserID int64
	SpinID int64
	Tier   SlotTier
}

// AwardOutput 发奖结果
type AwardOutput struct {
	WinLedgerID    sql.NullInt64 // 积分奖的入账流水
	PointsBalance  *int64        // 发奖后积分余额（仅积分奖会变）
	RedeemCode     string        // 余额奖的凭证
	BalanceChanged bool          // 是否改动了 users.balance，需失效缓存
}

// PrizeKind 一种奖品类型
type PrizeKind interface {
	Type() string
	Name() string
	// Normalize 校验奖品参数并清掉与本类型无关的字段，错误信息直接展示给管理员
	Normalize(t *SlotTier) error
	// ValuePoints 奖品价值（积分口径）
	ValuePoints(cfg *Config, t SlotTier) int64
	Award(ctx context.Context, in AwardInput) (AwardOutput, error)
}

var prizeKinds = map[string]PrizeKind{}

// RegisterPrizeKind 注册奖品类型；重复注册 panic
func RegisterPrizeKind(k PrizeKind) {
	if _, dup := prizeKinds[k.Type()]; dup {
		panic("slotmachine: duplicate prize kind " + k.Type())
	}
	prizeKinds[k.Type()] = k
}

func PrizeKindOf(typ string) (PrizeKind, bool) {
	k, ok := prizeKinds[typ]
	return k, ok
}

// PrizeKindMeta 管理端下拉选项
type PrizeKindMeta struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func PrizeKindCatalog() []PrizeKindMeta {
	list := make([]PrizeKindMeta, 0, len(prizeKinds))
	for _, k := range prizeKinds {
		list = append(list, PrizeKindMeta{Type: k.Type(), Name: k.Name()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Type < list[j].Type })
	return list
}

// ─── 谢谢惠顾 ──────────────────────────────────────────────────────────

type noneKind struct{}

func (noneKind) Type() string { return PrizeTypeNone }
func (noneKind) Name() string { return "谢谢惠顾" }
func (noneKind) Normalize(t *SlotTier) error {
	t.PrizePoints, t.PrizeBalance = 0, 0
	return nil
}
func (noneKind) ValuePoints(*Config, SlotTier) int64 { return 0 }
func (noneKind) Award(context.Context, AwardInput) (AwardOutput, error) {
	return AwardOutput{}, nil
}

// ─── 积分 ──────────────────────────────────────────────────────────────

type pointsKind struct{}

func (pointsKind) Type() string { return PrizeTypePoints }
func (pointsKind) Name() string { return "积分" }
func (pointsKind) Normalize(t *SlotTier) error {
	if t.PrizePoints <= 0 || t.PrizePoints > 10_000_000 {
		return errors.New("积分奖励需在 1-10000000 之间")
	}
	t.PrizeBalance = 0
	return nil
}
func (pointsKind) ValuePoints(_ *Config, t SlotTier) int64 { return t.PrizePoints }
func (pointsKind) Award(ctx context.Context, in AwardInput) (AwardOutput, error) {
	e, err := points.Credit(ctx, in.Tx, in.UserID, in.Tier.PrizePoints, points.ReasonSlotWin, in.SpinID,
		fmt.Sprintf("幸运九宫格中奖 +%d 积分", in.Tier.PrizePoints))
	if err != nil {
		return AwardOutput{}, fmt.Errorf("credit win points: %w", err)
	}
	return AwardOutput{WinLedgerID: sql.NullInt64{Int64: e.ID, Valid: true}, PointsBalance: &e.Balance}, nil
}

// ─── 余额 ──────────────────────────────────────────────────────────────

type balanceKind struct{}

func (balanceKind) Type() string { return PrizeTypeBalance }
func (balanceKind) Name() string { return "余额" }
func (balanceKind) Normalize(t *SlotTier) error {
	if t.PrizeBalance <= 0 || t.PrizeBalance > 10000 {
		return errors.New("余额奖励需在 (0, 10000] 之间")
	}
	t.PrizeBalance = math.Round(t.PrizeBalance*100) / 100
	t.PrizePoints = 0
	return nil
}
func (balanceKind) ValuePoints(cfg *Config, t SlotTier) int64 {
	return int64(math.Round(t.PrizeBalance * float64(cfg.PointsPerYuan)))
}
func (balanceKind) Award(ctx context.Context, in AwardInput) (AwardOutput, error) {
	code, err := newRedeemCode()
	if err != nil {
		return AwardOutput{}, err
	}
	if _, err := in.Tx.ExecContext(ctx,
		`INSERT INTO redeem_codes (code, type, value, status, used_by, used_at, notes, created_at)
		 VALUES ($1, 'balance', $2, 'used', $3, NOW(), $4, NOW())`,
		code, in.Tier.PrizeBalance, in.UserID, "幸运九宫格余额奖励"); err != nil {
		return AwardOutput{}, fmt.Errorf("insert redeem code: %w", err)
	}
	if _, err := in.Tx.ExecContext(ctx,
		`UPDATE users SET balance = balance + $1, updated_at = NOW() WHERE id = $2`,
		in.Tier.PrizeBalance, in.UserID); err != nil {
		return AwardOutput{}, fmt.Errorf("credit balance: %w", err)
	}
	if _, err := in.Tx.ExecContext(ctx,
		`UPDATE plus_slot_spins SET redeem_code = $1 WHERE id = $2`, code, in.SpinID); err != nil {
		return AwardOutput{}, fmt.Errorf("save redeem code: %w", err)
	}
	return AwardOutput{RedeemCode: code, BalanceChanged: true}, nil
}

// redeemCodeAlphabet Crockford Base32（不含易混淆的 I/L/O/U），32 个字符整除 256，取模无偏；16 位 = 80 bit 熵
const redeemCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const redeemCodeLen = 16

// newRedeemCode 生成不可预测的凭证码：SLOT- + 16 位随机字符（crypto/rand）
func newRedeemCode() (string, error) {
	buf := make([]byte, redeemCodeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate redeem code: %w", err)
	}
	out := make([]byte, redeemCodeLen)
	for i, b := range buf {
		out[i] = redeemCodeAlphabet[int(b)%len(redeemCodeAlphabet)]
	}
	return "SLOT-" + string(out), nil
}

// spinRedeemCode 摇奖记录对应的凭证码；redeem_code 为空的历史记录回落到旧规则 SLOT-{spin_id}
func spinRedeemCode(rec SpinRecord) string {
	if rec.PrizeType != PrizeTypeBalance {
		return ""
	}
	if rec.RedeemCode != "" {
		return rec.RedeemCode
	}
	return "SLOT-" + strconv.FormatInt(rec.ID, 10)
}

func init() {
	RegisterPrizeKind(noneKind{})
	RegisterPrizeKind(pointsKind{})
	RegisterPrizeKind(balanceKind{})
}
