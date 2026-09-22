package solver

import (
	"testing"

	"resonance-workshop/internal/model"
	"resonance-workshop/internal/store"
)

func fixture() ([]model.Experiment, []model.Peak, []model.AtomSite, []model.AtomExpected, []model.Connectivity) {
	d := store.Fixture()
	return d.Experiments, d.Peaks, d.Atoms, d.Expected, d.Conns
}

func assigned(sol model.Solution, expID, peakID int64) (int64, bool) {
	for _, a := range sol.Assignments {
		if a.ExpID == expID && a.PeakID == peakID {
			return a.AtomID, true
		}
	}
	return 0, false
}

// Two equal-length shortest schemes: A1/A2 swap must yield tied scores.
func TestTiedShortestSolutions(t *testing.T) {
	exps, peaks, atoms, exp, conns := fixture()
	res := Solve(exps, peaks, atoms, exp, conns, model.Ops{})
	if len(res.Solutions) < 2 {
		t.Fatalf("expected >=2 tied solutions, got %d", len(res.Solutions))
	}
	d := res.Solutions[1].Score - res.Solutions[0].Score
	if d < 0 || d > 1e-9 {
		t.Fatalf("expected tied scores, diff=%v", d)
	}
	a1, _ := assigned(res.Solutions[0], 1, 1)
	a2, _ := assigned(res.Solutions[1], 1, 1)
	if a1 == a2 {
		t.Fatalf("tied solutions should swap A1/A2, both map P1->A%d", a1)
	}
}

// Overlapped peak capacity comes from input evidence (cap=2), never unlimited.
func TestOverlapCapacityDeclared(t *testing.T) {
	exps := []model.Experiment{{ID: 1, Name: "E",
		Dims: []model.DimSpec{{Nucleus: "1H", Tol: 0.1}}, ShiftBias: []float64{0}}}
	peaks := []model.Peak{{ID: 1, ExpID: 1, Label: "ovl", Shifts: []float64{8.0}, Overlap: true, Capacity: 2}}
	atoms := []model.AtomSite{{ID: 1, Label: "A1"}, {ID: 2, Label: "A2"}, {ID: 3, Label: "A3"}}
	exp := []model.AtomExpected{
		{AtomID: 1, ExpID: 1, Shifts: []float64{8.0}},
		{AtomID: 2, ExpID: 1, Shifts: []float64{8.0}},
		{AtomID: 3, ExpID: 1, Shifts: []float64{8.0}},
	}
	res := Solve(exps, peaks, atoms, exp, nil, model.Ops{})
	n := 0
	for _, a := range res.Solutions[0].Assignments {
		if a.PeakID == 1 {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("overlapped peak must serve exactly its declared capacity 2, got %d", n)
	}
}

// Experiment-level bias adjustment changes the candidate set.
func TestBiasAdjustmentChangesCandidates(t *testing.T) {
	exps, peaks, atoms, exp, conns := fixture()
	base := Solve(exps, peaks, atoms, exp, conns, model.Ops{})
	found := false
	for _, id := range base.Unassigned {
		if id == 5 { // Q2 unassigned without bias
			found = true
		}
	}
	if !found {
		t.Fatalf("Q2 should be unassigned without bias, unassigned=%v", base.Unassigned)
	}
	adj := Solve(exps, peaks, atoms, exp, conns, model.Ops{BiasOverride: map[int64][]float64{2: {0, 0.2}}})
	if _, ok := assigned(adj.Solutions[0], 2, 5); !ok {
		t.Fatalf("Q2 should be assigned after +0.2 13C bias")
	}
	for _, id := range adj.Unassigned {
		if id == 5 {
			t.Fatalf("Q2 still unassigned after bias adjustment")
		}
	}
	if adj.Version == base.Version {
		t.Fatalf("version must pin the bias state")
	}
}

// Tolerances are per (nucleus, experiment); distances are never raw-summed.
func TestPerDimensionToleranceNotSummed(t *testing.T) {
	exps := []model.Experiment{{ID: 1, Name: "E",
		Dims:      []model.DimSpec{{Nucleus: "1H", Tol: 0.03}, {Nucleus: "15N", Tol: 0.30}},
		ShiftBias: []float64{0, 0}}}
	exp := []model.AtomExpected{{AtomID: 1, ExpID: 1, Shifts: []float64{8.00, 120.0}}}
	// H delta 0.05 exceeds its own window even though the raw sum (0.05) is tiny.
	bad := []model.Peak{{ID: 1, ExpID: 1, Label: "bad", Shifts: []float64{8.05, 120.0}, Capacity: 1}}
	c := Candidates(exps, bad, exp, nil)
	if len(c) != 0 {
		t.Fatalf("peak outside 1H window must not be a candidate, got %v", c)
	}
	// Both dims inside their own windows: raw sum 0.27 would fail a naive summed gate.
	good := []model.Peak{{ID: 2, ExpID: 1, Label: "good", Shifts: []float64{8.02, 120.25}, Capacity: 1}}
	c = Candidates(exps, good, exp, nil)
	if len(c) != 1 {
		t.Fatalf("peak inside per-dim windows must be a candidate, got %v", c)
	}
}

// Single-experiment exclusion and cross-experiment correspondence are checked
// separately: an atom may hold one peak per experiment, never two in one.
func TestExclusionVsCrossExperiment(t *testing.T) {
	exps, peaks, atoms, exp, conns := fixture()
	res := Solve(exps, peaks, atoms, exp, conns, model.Ops{BiasOverride: map[int64][]float64{2: {0, 0.2}}})
	best := res.Solutions[0]
	perExpAtom := map[[2]int64]int{}
	atomExps := map[int64]map[int64]bool{}
	for _, a := range best.Assignments {
		k := [2]int64{a.ExpID, a.AtomID}
		perExpAtom[k]++
		if perExpAtom[k] > 1 {
			t.Fatalf("atom %d assigned twice in experiment %d", a.AtomID, a.ExpID)
		}
		if atomExps[a.AtomID] == nil {
			atomExps[a.AtomID] = map[int64]bool{}
		}
		atomExps[a.AtomID][a.ExpID] = true
	}
	cross := false
	for _, m := range atomExps {
		if len(m) > 1 {
			cross = true
		}
	}
	if !cross {
		t.Fatalf("expected at least one atom assigned across experiments")
	}
}

// Conflicting locks produce a minimal conflict set and the solver recovers.
func TestMinimalConflictSet(t *testing.T) {
	exps, peaks, atoms, exp, conns := fixture()
	ops := model.Ops{Locks: []model.Lock{
		{PeakID: 1, AtomID: 1},
		{PeakID: 2, AtomID: 1}, // same atom, same experiment -> exclusion conflict
	}}
	res := Solve(exps, peaks, atoms, exp, conns, ops)
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected minimal conflict set of size 1, got %v", res.Conflicts)
	}
	if len(res.Solutions) == 0 {
		t.Fatalf("solver should recover after dropping the minimal conflict")
	}
}

// Locks within declared capacity are feasible (overlap peak takes two atoms).
func TestLockWithinOverlapCapacity(t *testing.T) {
	exps, peaks, atoms, exp, conns := fixture()
	ops := model.Ops{Locks: []model.Lock{
		{PeakID: 2, AtomID: 1},
		{PeakID: 2, AtomID: 2},
	}}
	res := Solve(exps, peaks, atoms, exp, conns, ops)
	if len(res.Conflicts) != 0 {
		t.Fatalf("locks within declared capacity must be feasible, got %v", res.Conflicts)
	}
	n := 0
	for _, a := range res.Solutions[0].Assignments {
		if a.PeakID == 2 {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected both locked atoms on overlapped peak, got %d", n)
	}
}

// Rejecting connectivity evidence changes scores but not feasibility.
func TestRejectConnectivity(t *testing.T) {
	exps, peaks, atoms, exp, conns := fixture()
	with := Solve(exps, peaks, atoms, exp, conns, model.Ops{})
	without := Solve(exps, peaks, atoms, exp, conns, model.Ops{RejectedConn: []int64{2}})
	if len(without.Solutions) == 0 {
		t.Fatalf("rejecting evidence must not break feasibility")
	}
	if with.Solutions[0].Score == without.Solutions[0].Score {
		t.Fatalf("rejecting connectivity evidence should change the score")
	}
}
