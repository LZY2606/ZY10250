package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"resonance-workshop/internal/solver"
	"resonance-workshop/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return httptest.NewServer(New(st).Handler())
}

func TestPageServed(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "共振归属工坊") {
		t.Fatal("page must contain the workshop title")
	}
}

type stateResponse struct {
	State  json.RawMessage `json:"state"`
	Result solver.Result   `json:"result"`
}

func TestAPIFlow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	get := func() stateResponse {
		resp, err := http.Get(ts.URL + "/api/state")
		if err != nil {
			t.Fatalf("state: %v", err)
		}
		defer resp.Body.Close()
		var sr stateResponse
		if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return sr
	}

	first := get()
	if len(first.Result.Solutions) != 2 {
		t.Fatalf("want 2 tied solutions, got %d", len(first.Result.Solutions))
	}
	if first.Result.VersionID == 0 {
		t.Fatal("result must be pinned to a peak-list version")
	}

	// Adjust the experiment-level bias; candidates must change.
	resp, err := http.Post(ts.URL+"/api/bias", "application/json",
		strings.NewReader(`{"experiment_id":2,"nucleus":"H","value":0.3}`))
	if err != nil {
		t.Fatalf("bias post: %v", err)
	}
	resp.Body.Close()
	second := get()
	count := 0
	for _, c := range second.Result.Candidates {
		if c.PeakID == 6 {
			count++
		}
	}
	if count != 0 {
		t.Fatal("hnca-1 candidates should vanish after bias adjustment")
	}

	// Lock a pair; tie collapses.
	resp, err = http.Post(ts.URL+"/api/lock", "application/json",
		strings.NewReader(`{"peak_id":1,"atom_id":1,"locked":true}`))
	if err != nil {
		t.Fatalf("lock post: %v", err)
	}
	resp.Body.Close()
	third := get()
	if len(third.Result.Solutions) != 1 {
		t.Fatalf("want 1 solution after lock, got %d", len(third.Result.Solutions))
	}

	// Reset clears and re-imports the fixture.
	resp, err = http.Post(ts.URL+"/api/reset", "application/json", nil)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	resp.Body.Close()
	fourth := get()
	if len(fourth.Result.Solutions) != 2 {
		t.Fatalf("fixture should restore 2 tied solutions, got %d", len(fourth.Result.Solutions))
	}

	// Run log is exportable.
	resp, err = http.Get(ts.URL + "/api/export")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	defer resp.Body.Close()
	var log []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&log); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if len(log) == 0 {
		t.Fatal("run log must not be empty after operations")
	}
}
