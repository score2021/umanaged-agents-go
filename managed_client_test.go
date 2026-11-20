package umanagedagents

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestManagedClientRetriesOnlyReplayableRequests(t *testing.T) {
	for _, scenario := range []struct {
		name, method, key string
		expected          int32
	}{
		{name: "read", method: http.MethodGet, expected: 2},
		{name: "keyed write", method: http.MethodPost, key: "original-key", expected: 2},
		{name: "unkeyed write", method: http.MethodPost, expected: 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Header.Get("Idempotency-Key") != scenario.key || string(body) != "original-body" {
					t.Error("request key or body changed")
				}
				if count.Add(1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "fixture-key")
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(scenario.method, server.URL, strings.NewReader("original-body"))
			if err != nil {
				t.Fatal(err)
			}
			if scenario.key != "" {
				req.Header.Set("Idempotency-Key", scenario.key)
			}
			resp, err := client.API.GetConfig().HTTPClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if count.Load() != scenario.expected {
				t.Fatalf("got %d attempts, want %d", count.Load(), scenario.expected)
			}
		})
	}
}

func TestManagedClientConfigurationPrecedence(t *testing.T) {
	t.Setenv("MANAGED_AGENTS_API_URL", "https://new.example/")
	t.Setenv("MANAGED_AGENTS_API_KEY", "new-key")
	t.Setenv("MVPP_API_URL", "https://legacy.example")
	t.Setenv("MVPP_API_KEY", "legacy-key")
	for _, scenario := range []struct {
		name, baseURL, apiKey, wantURL, wantKey string
		unsetNew                                bool
	}{
		{name: "new variables win", wantURL: "https://new.example", wantKey: "new-key"},
		{name: "arguments win", baseURL: "https://arg.example", apiKey: "arg-key", wantURL: "https://arg.example", wantKey: "arg-key"},
		{name: "legacy fallback", unsetNew: true, wantURL: "https://legacy.example", wantKey: "legacy-key"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.unsetNew {
				t.Setenv("MANAGED_AGENTS_API_URL", "")
				t.Setenv("MANAGED_AGENTS_API_KEY", "")
			}
			client, err := NewClient(scenario.baseURL, scenario.apiKey)
			if err != nil {
				t.Fatal(err)
			}
			if got := client.API.GetConfig().Servers[0].URL; got != scenario.wantURL {
				t.Fatalf("got URL %q, want %q", got, scenario.wantURL)
			}
			if client.key != scenario.wantKey {
				t.Fatalf("got key %q, want %q", client.key, scenario.wantKey)
			}
		})
	}
}

func TestManagedClientHasNoBuiltInAddress(t *testing.T) {
	for _, name := range []string{"MANAGED_AGENTS_API_URL", "MVPP_API_URL"} {
		t.Setenv(name, "")
	}
	t.Setenv("MANAGED_AGENTS_API_KEY", "secret-key-value")
	if _, err := NewClient("", ""); err == nil || err.Error() != "base URL is required (or MANAGED_AGENTS_API_URL)" {
		t.Fatalf("got %v", err)
	}
	if _, err := NewClient("ftp://bad.example", ""); err == nil || strings.Contains(err.Error(), "secret-key-value") {
		t.Fatalf("got %v", err)
	}
	t.Setenv("MANAGED_AGENTS_API_KEY", "")
	t.Setenv("MVPP_API_KEY", "")
	if _, err := NewClient("https://x.example", ""); err == nil || err.Error() != "api key is required (or MANAGED_AGENTS_API_KEY)" {
		t.Fatalf("got %v", err)
	}
}
