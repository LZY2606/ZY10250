package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"resonance-workshop/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st).Handler())
	t.Cleanup(func() { ts.Close(); st.Close() })
	return ts
}

func TestIndexShowsTitle(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 1<<20)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "共振归属工坊") {
		t.Fatalf("index page must show 共振归属工坊")
	}
}

func TestSolveAndBiasEffect(t *testing.T) {
	ts := newTestServer(t)
	// default solve: Q2 (peak 5) unassigned
	resp, _ := http.Get(ts.URL + "/api/state")
	buf := make([]byte, 1<<20)
	n, _ := resp.Body.Read(buf)
	resp.Body.Close()
	if !strings.Contains(string(buf[:n]), `"Version"`) {
		t.Fatalf("state must return a versioned result")
	}
	// solve with +0.2 13C bias on experiment 2: Q2 becomes assigned
	body := strings.NewReader(`{"BiasOverride":{"2":[0,0.2]}}`)
	resp2, err := http.Post(ts.URL+"/api/solve", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	n, _ = resp2.Body.Read(buf)
	resp2.Body.Close()
	out := string(buf[:n])
	if !strings.Contains(out, `"PeakID":5`) {
		t.Fatalf("Q2 should be assigned after bias adjustment: %s", out)
	}
	// run log exported
	resp3, _ := http.Get(ts.URL + "/api/runs")
	n, _ = resp3.Body.Read(buf)
	resp3.Body.Close()
	if !strings.Contains(string(buf[:n]), `"runs"`) {
		t.Fatalf("run log export failed")
	}
}

func TestResetReseeds(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/reset", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp2, _ := http.Get(ts.URL + "/api/state")
	buf := make([]byte, 1<<20)
	n, _ := resp2.Body.Read(buf)
	resp2.Body.Close()
	if !strings.Contains(string(buf[:n]), "HSQC-15N") {
		t.Fatalf("reset should re-import the fixture")
	}
}
