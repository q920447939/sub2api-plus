//go:build unit

package slotmachine

import (
	"strings"
	"testing"
)

func TestNewRedeemCodeRandom(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		c, err := newRedeemCode()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(c, "SLOT-") || len(c) != 21 || len(c) > 32 {
			t.Fatalf("bad code %q", c)
		}
		for _, ch := range c[5:] {
			if !strings.ContainsRune(redeemCodeAlphabet, ch) {
				t.Fatalf("unexpected char %q in %q", ch, c)
			}
		}
		if seen[c] {
			t.Fatalf("duplicate code %q", c)
		}
		seen[c] = true
	}
}

func TestSpinRedeemCodeLegacyFallback(t *testing.T) {
	if got := spinRedeemCode(SpinRecord{ID: 47, PrizeType: PrizeTypeBalance}); got != "SLOT-47" {
		t.Fatalf("legacy fallback got %q", got)
	}
	if got := spinRedeemCode(SpinRecord{ID: 47, PrizeType: PrizeTypeBalance, RedeemCode: "SLOT-ABC"}); got != "SLOT-ABC" {
		t.Fatalf("stored code got %q", got)
	}
	if got := spinRedeemCode(SpinRecord{ID: 47, PrizeType: PrizeTypePoints}); got != "" {
		t.Fatalf("non-balance should have no code, got %q", got)
	}
}
