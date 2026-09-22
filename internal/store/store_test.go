package store_test

import (
	"testing"

	"resonance-workshop/internal/fixture"
	"resonance-workshop/internal/solver"
	"resonance-workshop/internal/store"
)

func openMem(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSeedLoadRoundTrip(t *testing.T) {
	s := openMem(t)
	vid, err := fixture.Seed(s)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	st, err := s.LoadState(vid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.Peaks) != 7 || len(st.Atoms) != 6 || len(st.Experiments) != 2 {
		t.Fatalf("unexpected counts: %d peaks, %d atoms, %d experiments",
			len(st.Peaks), len(st.Atoms), len(st.Experiments))
	}
	if len(st.Correspondence) != 2 || len(st.Connectivities) != 3 {
		t.Fatalf("unexpected evidence counts")
	}
}

// Reset must clear the database so the fixture can be re-imported and
// re-checked from scratch.
func TestResetAndReimport(t *testing.T) {
	s := openMem(t)
	vid1, err := fixture.Seed(s)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	st1, err := s.LoadState(vid1)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r1 := solver.Solve(*st1)

	if err := s.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n, _ := s.LatestVersion(); n != 0 {
		t.Fatalf("database not empty after reset, latest version %d", n)
	}
	vid2, err := fixture.Seed(s)
	if err != nil {
		t.Fatalf("reseed: %v", err)
	}
	st2, err := s.LoadState(vid2)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	r2 := solver.Solve(*st2)
	if len(r1.Solutions) != len(r2.Solutions) {
		t.Fatalf("solution count changed after re-import: %d vs %d",
			len(r1.Solutions), len(r2.Solutions))
	}
	if len(r2.Solutions) != 2 {
		t.Fatalf("want 2 equal solutions after re-import, got %d", len(r2.Solutions))
	}
}

func TestRunLogExport(t *testing.T) {
	s := openMem(t)
	vid, err := fixture.Seed(s)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.LogRun(vid, "solve", map[string]any{"solutions": 2}); err != nil {
		t.Fatalf("log: %v", err)
	}
	if err := s.SetBias(2, "H", 0.3); err != nil {
		t.Fatalf("bias: %v", err)
	}
	if err := s.LogRun(vid, "bias", map[string]any{"experiment_id": 2, "nucleus": "H", "value": 0.3}); err != nil {
		t.Fatalf("log: %v", err)
	}
	log, err := s.RunLog()
	if err != nil {
		t.Fatalf("runlog: %v", err)
	}
	if len(log) != 2 || log[0].Action != "solve" || log[1].Action != "bias" {
		t.Fatalf("unexpected run log: %+v", log)
	}
	// Bias persisted.
	st, err := s.LoadState(vid)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if st.Experiments[1].Bias["H"] != 0.3 {
		t.Fatalf("bias not persisted: %v", st.Experiments[1].Bias)
	}
}
