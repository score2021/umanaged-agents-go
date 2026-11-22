package umanagedagents

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagedClientTrustsTheConfiguredCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(ca, block, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MANAGED_AGENTS_CA_FILE", "")
	t.Setenv("MVPP_CA_FILE", "")
	plain, err := NewClient(server.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := plain.API.ModelsAPI.ListModels(plain.Context(context.Background())).Execute(); err == nil {
		t.Fatal("a private CA must not be trusted without configuration")
	}
	for name, client := range map[string]func() (*Client, error){
		"option": func() (*Client, error) { return NewClient(server.URL, "k", WithCAFile(ca)) },
		"env": func() (*Client, error) {
			t.Setenv("MANAGED_AGENTS_CA_FILE", ca)
			return NewClient(server.URL, "k")
		},
	} {
		c, err := client()
		if err != nil {
			t.Fatal(name, err)
		}
		if _, _, err := c.API.ModelsAPI.ListModels(c.Context(context.Background())).Execute(); err != nil {
			t.Fatal(name, err)
		}
	}
	if _, err := NewClient(server.URL, "k", WithCAFile(filepath.Join(t.TempDir(), "missing.pem"))); err == nil {
		t.Fatal("a missing CA file must fail")
	}
}

func TestManagedClientStreamYieldsEventsAndSkipsHeartbeats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/sesn_1/events/stream" || r.Header.Get("x-api-key") != "k" || r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, ": heartbeat\n\n")
		fmt.Fprint(w, "event: agent.message\ndata: {\"type\":\"agent.message\",\"id\":\"sevt_a\",\"processed_at\":\"2026-01-01T00:00:00Z\",\"input_event_id\":\"sevt_in\",\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"type\":\"session.status_idle\",\"id\":\"sevt_b\",\"processed_at\":\"2026-01-01T00:00:01Z\",\n")
		fmt.Fprint(w, "data: \"input_event_id\":\"sevt_in\",\"stop_reason\":{\"type\":\"end_turn\"}}\n\n")
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for event, err := range c.Stream(context.Background(), "sesn_1") {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, EventType(event)+"/"+EventID(event)+"/"+EventInputID(event))
	}
	if strings.Join(got, ",") != "agent.message/sevt_a/sevt_in,session.status_idle/sevt_b/sevt_in" {
		t.Fatalf("got %v", got)
	}
}

func TestManagedClientStreamReportsHTTPErrorsAndContextEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "missing") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"RESOURCE_NOT_FOUND"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, "k")
	for _, err := range c.Stream(context.Background(), "missing") {
		if err == nil || !strings.Contains(string(err.(*GenericOpenAPIError).Body()), "RESOURCE_NOT_FOUND") {
			t.Fatalf("got %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	for _, err := range c.Stream(ctx, "sesn_1") {
		if err != context.DeadlineExceeded {
			t.Fatalf("got %v", err)
		}
	}
}

func TestManagedClientUploadFileSendsEveryPart(t *testing.T) {
	data := bytes.Repeat([]byte("abc"), 1000) // 3000 bytes in parts of 1024
	var mu sync.Mutex
	parts := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["filename"] != "data.bin" || body["size_bytes"] != float64(len(data)) || len(fmt.Sprint(body["sha256"])) != 64 {
				http.Error(w, "bad create", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"id":"upl_1","type":"upload","filename":"data.bin","mime_type":"application/octet-stream","size_bytes":3000,"part_size":1024,"part_count":3,"state":"open","parts":[],"file_id":null,"created_at":"x","expires_at":"y"}`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/uploads/upl_1/parts/"):
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			parts[strings.TrimPrefix(r.URL.Path, "/v1/uploads/upl_1/parts/")] = body
			mu.Unlock()
			fmt.Fprint(w, `{"part_number":1,"size_bytes":1,"sha256":"x"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/uploads/upl_1/complete":
			fmt.Fprint(w, `{"id":"file_1","type":"file","filename":"data.bin","mime_type":"application/octet-stream","size_bytes":3000,"sha256":"x","own_key":true,"session_id":null,"origin":"upload","input_event_id":null,"downloadable":true,"created_by":"u","created_at":"x","expires_at":null}`)
		default:
			http.Error(w, r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "data.bin")
	_ = os.WriteFile(path, data, 0o600)
	c, _ := NewClient(server.URL, "k")
	file, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	joined := append(append(append([]byte{}, parts["1"]...), parts["2"]...), parts["3"]...)
	if file.Id != "file_1" || len(parts) != 3 || !bytes.Equal(joined, data) {
		t.Fatalf("file %s, %d parts, joined equal %v", file.Id, len(parts), bytes.Equal(joined, data))
	}
}
