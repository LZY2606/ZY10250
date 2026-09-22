package store

import (
	"path/filepath"
	"testing"

	"resonance-workshop/internal/model"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSeedLoadRoundTrip(t *testing.T) {
	st := openTemp(t)
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	d, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Experiments) != 2 || len(d.Peaks) != 6 || len(d.Atoms) != 4 || len(d.Conns) != 2 {
		t.Fatalf("unexpected fixture size: %+v", d)
	}
	var ovl *model.Peak
	for i, p := range d.Peaks {
		if p.Overlap {
			ovl = &d.Peaks[i]
		}
	}
	if ovl == nil || ovl.Capacity != 2 {
		t.Fatalf("fixture must declare one overlapped peak with capacity 2")
	}
}

// The database can be wiped and the fixture re-imported for review.
func TestWipeAndReimport(t *testing.T) {
	st := openTemp(t)
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	if err := st.Wipe(); err != nil {
		t.Fatal(err)
	}
	d, _ := st.Load()
	if len(d.Peaks) != 0 {
		t.Fatalf("wipe should empty the database")
	}
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	d, _ = st.Load()
	if len(d.Peaks) != 6 {
		t.Fatalf("re-import after wipe failed")
	}
}

func TestRunLog(t *testing.T) {
	st := openTemp(t)
	res := model.SolveResult{Version: "v1"}
	if err := st.SaveRun("v1", model.Ops{}, res); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Version != "v1" {
		t.Fatalf("run log not persisted: %+v", runs)
	}
}
