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
