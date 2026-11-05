//go:build unit

package iq

import (
	"strings"
	"testing"
	"time"
)

func TestExtract(t *testing.T) {
	c := PelicanChallenge{}
	cases := []struct {
		name, raw, wantPrefix string
		wantErr               bool
	}{
		{"plain html", "<!DOCTYPE html><html><body>x</body></html>", "<!DOCTYPE html>", false},
		{"fenced", "好的：\n```html\n<!DOCTYPE html><html><head></head></html>\n```\n说明", "<!DOCTYPE html>", false},
		{"leading text", "下面是代码\n<html lang=\"zh\"><body></body></html>", "<html", false},
		{"bare svg wrapped", "<svg viewBox=\"0 0 1 1\"></svg>", "<!DOCTYPE html>", false},
		{"bom", "\uFEFF<html></html>", "<html>", false},
		{"no html", "我来描述一下这只鹈鹕……", "", true},
		{"empty", "  ", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			art, err := c.Extract(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(art.HTML, tc.wantPrefix) || strings.Contains(art.HTML, "```") {
				t.Fatalf("html = %q", art.HTML)
			}
		})
	}
}

func TestWithPreviewCSPAndRefs(t *testing.T) {
	out := WithPreviewCSP(`<html><head><title>a</title></head><body><img src="https://evil.example/x.png"></body></html>`)
	if !strings.Contains(out, `<head><meta http-equiv="Content-Security-Policy"`) {
		t.Fatalf("csp must be the first thing in head: %s", out)
	}
	if out := WithPreviewCSP("<svg></svg>"); !strings.Contains(out, "Content-Security-Policy") {
		t.Fatal("csp missing when there is no head")
	}
	refs := FindExternalRefs(`<script src="https://cdn.x/a.js"></script><div style="background:url('https://img.x/b.png')"></div>`)
	if len(refs) != 2 {
		t.Fatalf("refs = %v", refs)
	}
}

func TestSigner(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 30, 0, 0, time.UTC)
	s := Signer{Secret: []byte(strings.Repeat("k", 40)), Now: func() time.Time { return now }}
	u := s.URL(42, ScopeUser)
	if u != s.URL(42, ScopeUser) {
		t.Fatal("url should be stable within the hour")
	}
	q := parseQuery(t, u)
	if sc, ok := s.Verify(42, q["sc"], q["e"], q["s"]); !ok || sc != ScopeUser {
		t.Fatal("valid signature rejected")
	}
	if _, ok := s.Verify(43, q["sc"], q["e"], q["s"]); ok {
		t.Fatal("signature must bind the run id")
	}
	if _, ok := s.Verify(42, "a", q["e"], q["s"]); ok {
		t.Fatal("user signature must not grant admin scope")
	}
	later := Signer{Secret: s.Secret, Now: func() time.Time { return now.Add(3 * time.Hour) }}
	if _, ok := later.Verify(42, q["sc"], q["e"], q["s"]); ok {
		t.Fatal("expired link accepted")
	}
	other := Signer{Secret: []byte(strings.Repeat("x", 40)), Now: s.Now}
	if _, ok := other.Verify(42, q["sc"], q["e"], q["s"]); ok {
		t.Fatal("signature from another secret accepted")
	}
}

func parseQuery(t *testing.T, u string) map[string]string {
	t.Helper()
	i := strings.Index(u, "?")
	out := map[string]string{}
	for _, kv := range strings.Split(u[i+1:], "&") {
		p := strings.SplitN(kv, "=", 2)
		out[p[0]] = p[1]
	}
	return out
}
