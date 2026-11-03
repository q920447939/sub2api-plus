//go:build unit

package checkin

import (
	"encoding/json"
	"testing"
)

// 旧版配置 JSON 没有 popup_enabled 字段，加载时应沿用默认值（开启）
func TestConfigPopupDefaultsOnLegacyJSON(t *testing.T) {
	cfg := DefaultConfig()
	legacy := `{"enabled":true,"rewards":[1,2,3,4,5,6,7],"week_bonus":0,"ip_daily_max":0}`
	if err := json.Unmarshal([]byte(legacy), &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.PopupEnabled {
		t.Fatal("popup_enabled should default to true when absent")
	}
}

func TestConfigPopupExplicitOff(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(`{"popup_enabled":false}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.PopupEnabled {
		t.Fatal("popup_enabled=false should be respected")
	}
}
