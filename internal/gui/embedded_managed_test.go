package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

func TestEmbeddedWebGuardHeader(t *testing.T) {
	t.Setenv("MAGPIE_EMBEDDED", "1")
	key := "0123456789abcdef0123456789abcdef"
	inner := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("ok"))
	})
	h := webGuard("magpie_test", key, 0, nil, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/managed/health", nil)
	req.Header.Set("X-Magpie-Web-Key", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid X-Magpie-Web-Key, got %d", rec.Code)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/managed/health", nil)
	badReq.Header.Set("X-Magpie-Web-Key", "wrong-key")
	badRec := httptest.NewRecorder()
	h.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with bad X-Magpie-Web-Key, got %d", badRec.Code)
	}
}

func TestEmbeddedManagedProviderSync(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MAGPIE_EMBEDDED", "1")

	if err := provider.Save(provider.Provider{
		ID:   "user-own-provider",
		Name: "User Own",
		Key:  "sk-user",
		Chat: "https://api.example.com/v1",
	}); err != nil {
		t.Fatalf("seed user provider: %v", err)
	}

	mux := http.NewServeMux()
	embeddedManagedRoutes(mux)

	body, _ := json.Marshal(map[string]any{
		"prefix": "lov-env-",
		"providers": []map[string]any{
			{
				"id":        "lov-env-101-codex",
				"name":      "Lov Env 101 Codex",
				"key":       "lov-slot-abc",
				"responses": "http://127.0.0.1:49999/v1",
				"baseAPI":   "responses",
				"models":    []string{"gpt-5.3-codex"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/managed/providers/sync", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first sync failed: %d %s", rec.Code, rec.Body.String())
	}

	p, err := provider.Find("lov-env-101-codex")
	if err != nil {
		t.Fatalf("expected managed provider to be saved: %v", err)
	}
	if p.Proxy != "direct" {
		t.Fatalf("expected default proxy=direct, got %q", p.Proxy)
	}
	if _, err := provider.Find("user-own-provider"); err != nil {
		t.Fatalf("user provider should not be touched: %v", err)
	}

	// Second sync with empty list should prune lov-env-* but keep user-own-provider
	body2, _ := json.Marshal(map[string]any{
		"prefix":    "lov-env-",
		"providers": []any{},
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/managed/providers/sync", bytes.NewReader(body2))
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second sync failed: %d %s", rec2.Code, rec2.Body.String())
	}
	if _, err := provider.Find("lov-env-101-codex"); err == nil {
		t.Fatalf("expected stale managed provider to be pruned")
	}
	if _, err := provider.Find("user-own-provider"); err != nil {
		t.Fatalf("user provider must remain after managed prune: %v", err)
	}
}
