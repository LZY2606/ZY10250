package solver

import (
	"math"
	"testing"

	"resonance-workshop/internal/model"
)

// fixtureState mirrors internal/fixture but is self-contained for unit tests.
func fixtureState() model.State {
	exp1 := model.Experiment{ID: 1, VersionID: 1, Name: "HSQC",
		Nuclei: []string{"H", "N"}, Tolerances: map[string]float64{"H": 0.05, "N": 0.5},
		Bias: map[string]float64{"H": 0, "N": 0}, Window: 1.5}
	exp2 := model.Experiment{ID: 2, VersionID: 1, Name: "HNCA",
		Nuclei: []string{"H", "C"}, Tolerances: map[string]float64{"H": 0.05, "C": 0.6},
		Bias: map[string]float64{"H": 0.02, "C": 0}, Window: 1.5}
	return model.State{
		Version:     model.Version{ID: 1, Name: "fixture-v1"},
		Experiments: []model.Experiment{exp1, exp2},
		Peaks: []model.Peak{
			{ID: 1, VersionID: 1, ExperimentID: 1, Label: "hsqc-1", Shifts: []float64{8.01, 120.1}, Intensity: 1.0, Capacity: 1},
			{ID: 2, VersionID: 1, ExperimentID: 1, Label: "hsqc-2", Shifts: []float64{8.02, 119.9}, Intensity: 0.9, Capacity: 1},
			{ID: 3, VersionID: 1, ExperimentID: 1, Label: "hsqc-3", Shifts: []float64{7.50, 115.1}, Intensity: 0.8, Capacity: 1},
			{ID: 4, VersionID: 1, ExperimentID: 1, Label: "hsqc-ovl", Shifts: []float64{7.25, 112.2}, Intensity: 2.4, Overlap: true, Capacity: 2},
			{ID: 5, VersionID: 1, ExperimentID: 1, Label: "hsqc-lone", Shifts: []float64{9.50, 130.0}, Intensity: 0.2, Capacity: 1},
			{ID: 6, VersionID: 1, ExperimentID: 2, Label: "hnca-1", Shifts: []float64{8.03, 55.1}, Intensity: 0.7, Capacity: 1},
			{ID: 7, VersionID: 1, ExperimentID: 2, Label: "hnca-2", Shifts: []float64{7.52, 58.1}, Intensity: 0.6, Capacity: 1},
		},
		Atoms: []model.Atom{
			{ID: 1, VersionID: 1, Name: "Gly1-HN", Residue: "G1", Shifts: map[string]float64{"H": 8.00, "N": 120.0, "C": 55.0}},
			{ID: 2, VersionID: 1, Name: "Ala2-HN", Residue: "A2", Shifts: map[string]float64{"H": 8.00, "N": 120.0, "C": 55.4}},
			{ID: 3, VersionID: 1, Name: "Ser3-HN", Residue: "S3", Shifts: map[string]float64{"H": 7.50, "N": 115.0, "C": 58.0}},
			{ID: 4, VersionID: 1, Name: "Thr4-HN", Residue: "T4", Shifts: map[string]float64{"H": 7.20, "N": 112.0, "C": 62.0}},
			{ID: 5, VersionID: 1, Name: "Val5-HN", Residue: "V5", Shifts: map[string]float64{"H": 7.28, "N": 112.5, "C": 62.5}},
			{ID: 6, VersionID: 1, Name: "Leu6-HN", Residue: "L6", Shifts: map[string]float64{"H": 7.21, "N": 111.8, "C": 60.0}},
		},
		Connectivities: []model.Connectivity{
			{ID: 1, VersionID: 1, AtomA: 2, AtomB: 3, Kind: "sequential", Weight: 0.3},
			{ID: 2, VersionID: 1, AtomA: 3, AtomB: 4, Kind: "sequential", Weight: 0.3},
			{ID: 3, VersionID: 1, AtomA: 4, AtomB: 5, Kind: "sequential", Weight: 0.3},
		},
		Correspondence: []model.Correspondence{
			{ID: 1, VersionID: 1, PeakA: 1, PeakB: 6},
			{ID: 2, VersionID: 1, PeakA: 3, PeakB: 7},
		},
	}
}

func assignmentsOf(s Solution) map[int64]int64 {
	m := map[int64]int64{}
	for _, a := range s.Assignments {
		m[a.PeakID] = a.AtomID
	}
	return m
}

// The fixture must yield exactly two equal-scoring shortest solutions:
// hsqc-1/hsqc-2 swap between Gly1-HN and Ala2-HN.
func TestTwoEqualBestSolutions(t *testing.T) {
	res := Solve(fixtureState())
	if len(res.Solutions) != 2 {
		t.Fatalf("want 2 near-equal solutions, got %d", len(res.Solutions))
	}
	if math.Abs(res.Solutions[0].Score-res.Solutions[1].Score) > ScoreEpsilon {
		t.Fatalf("scores differ: %v vs %v", res.Solutions[0].Score, res.Solutions[1].Score)
	}
	a0, a1 := assignmentsOf(res.Solutions[0]), assignmentsOf(res.Solutions[1])
	if a0[1] == a1[1] || a0[2] == a1[2] {
		t.Fatalf("tied solutions should swap atoms 1/2: %v vs %v", a0, a1)
	}
	// Overlap peak takes exactly two atoms in both solutions.
	for i, s := range res.Solutions {
		load := 0
		for _, a := range s.Assignments {
			if a.PeakID == 4 {
				load++
			}
		}
		if load != 2 {
			t.Fatalf("solution %d: overlap peak load = %d, want 2", i, load)
		}
	}
	// hsqc-lone has no candidates and stays unassigned.
	if len(res.Unassigned) != 1 || res.Unassigned[0] != 5 {
		t.Fatalf("unassigned = %v, want [5]", res.Unassigned)
	}
}

// Overlap capacity is declared by input evidence: three atoms sit inside
// the window of hsqc-ovl, but no solution may load it beyond capacity 2.
func TestOverlapCapacityDeclaredNotInfinite(t *testing.T) {
	st := fixtureState()
	res := Solve(st)
	inWindow := 0
	for _, c := range res.Candidates {
		if c.PeakID == 4 {
			inWindow++
		}
	}
	if inWindow != 3 {
		t.Fatalf("overlap peak should have 3 candidate atoms, got %d", inWindow)
	}
	for _, s := range res.Solutions {
		load := 0
		for _, a := range s.Assignments {
			if a.PeakID == 4 {
				load++
			}
		}
		if load > 2 {
			t.Fatalf("capacity violated: load %d > 2", load)
		}
	}
}

// Adjusting the experiment-level bias must change the candidate set.
func TestBiasShiftChangesCandidates(t *testing.T) {
	st := fixtureState()
	before := Candidates(st)
	countFor := func(cs []Candidate, peakID int64) int {
		n := 0
		for _, c := range cs {
			if c.PeakID == peakID {
				n++
			}
		}
		return n
	}
	if countFor(before, 6) == 0 {
		t.Fatal("hnca-1 should have candidates under the initial bias")
	}
	// Acceptance move: raise the HNCA H bias to 0.3 ppm.
	st.Experiments[1].Bias["H"] = 0.3
	after := Candidates(st)
	if countFor(after, 6) != 0 {
		t.Fatal("hnca-1 candidates should disappear after bias adjustment")
	}
	res := Solve(st)
	found := false
	for _, id := range res.Unassigned {
		if id == 6 {
			found = true
		}
	}
	if !found {
		t.Fatal("hnca-1 should become unassigned after bias adjustment")
	}
}

// Single-experiment exclusivity and cross-experiment correspondence are
// validated separately: sharing an atom across experiments is allowed and
// satisfies correspondence; sharing within one experiment is forbidden.
func TestExclusivityAndCorrespondenceSeparated(t *testing.T) {
	res := Solve(fixtureState())
	if len(res.Solutions) == 0 {
		t.Fatal("no solutions")
	}
	for _, s := range res.Solutions {
		seen := map[[2]int64]int64{}
		for _, a := range s.Assignments {
			exp := int64(1)
			if a.PeakID >= 6 {
				exp = 2
			}
			key := [2]int64{exp, a.AtomID}
			if prev, dup := seen[key]; dup {
				t.Fatalf("exclusivity violated: peaks %d and %d share atom %d in experiment %d",
					prev, a.PeakID, a.AtomID, exp)
			}
			seen[key] = a.PeakID
		}
	}
	// One of the two tied solutions maps hsqc-1 -> Ala2-HN while
	// hnca-1 -> Gly1-HN: correspondence #1 must be flagged there only.
	viol := []bool{}
	for _, s := range res.Solutions {
		v := false
		for _, id := range s.CorrViolations {
			if id == 1 {
				v = true
			}
		}
		viol = append(viol, v)
	}
	if len(viol) != 2 || viol[0] == viol[1] {
		t.Fatalf("correspondence violations should differ across tied solutions: %v", viol)
	}
	// Cross-experiment atom sharing itself is legal: find a solution where
	// hsqc-1 and hnca-1 share an atom without an exclusivity conflict.
	shared := false
	for _, s := range res.Solutions {
		m := assignmentsOf(s)
		if m[1] != 0 && m[1] == m[6] {
			shared = true
		}
	}
	if !shared {
		t.Fatal("expected a solution sharing one atom across experiments")
	}
}

// Tolerance handling must be scale-aware: per-dimension offsets are
// normalised by their own (experiment, nucleus) tolerance and combined
// as a Euclidean norm, never summed as raw ppm.
func TestNormalizedDistanceIsScaleAware(t *testing.T) {
	exp := model.Experiment{Nuclei: []string{"H", "N"},
		Tolerances: map[string]float64{"H": 0.05, "N": 0.5},
		Bias:       map[string]float64{"H": 0, "N": 0}}
	peak := model.Peak{Shifts: []float64{8.04, 120.4}}
	atom := model.Atom{Shifts: map[string]float64{"H": 8.00, "N": 120.0}}
	got := NormalizedDistance(exp, peak, atom)
	want := math.Sqrt(0.8*0.8 + 0.8*0.8) // 0.8 tolerance units per dimension
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("distance = %v, want %v", got, want)
	}
	// Raw ppm sum would be 0.44 and raw max 0.4; both misread the scale.
	if got < 1.0 {
		t.Fatal("normalised distance must exceed 1 tolerance unit here")
	}
	// Bias is subtracted before normalisation.
	exp.Bias["H"] = 0.04
	got = NormalizedDistance(exp, peak, atom)
	if math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("distance after bias = %v, want 0.8", got)
	}
}

// Rejecting connectivity evidence lowers the score of every solution
// that used it, without removing candidate edges.
func TestRejectConnectivity(t *testing.T) {
	st := fixtureState()
	before := Solve(st)
	st.Connectivities[2].Rejected = true
	after := Solve(st)
	if len(before.Solutions) == 0 || len(after.Solutions) == 0 {
		t.Fatal("solutions missing")
	}
	diff := before.Solutions[0].Score - after.Solutions[0].Score
	if math.Abs(diff-0.3) > 1e-9 {
		t.Fatalf("score drop = %v, want 0.3", diff)
	}
	if len(after.Candidates) != len(before.Candidates) {
		t.Fatal("rejecting connectivity must not prune candidate edges")
	}
}

// Contradictory locks produce a minimal conflict set instead of pruning
// everything silently.
func TestMinimalConflictSet(t *testing.T) {
	st := fixtureState()
	// Two locks in the same experiment pointing at the same atom.
	st.Locks = []model.Lock{
		{ID: 10, VersionID: 1, PeakID: 1, AtomID: 1},
		{ID: 11, VersionID: 1, PeakID: 2, AtomID: 1},
		{ID: 12, VersionID: 1, PeakID: 3, AtomID: 3}, // consistent, must survive
	}
	res := Solve(st)
	if len(res.ConflictSet) != 2 {
		t.Fatalf("conflict set = %v, want the two contradictory locks", res.ConflictSet)
	}
	for _, id := range res.ConflictSet {
		if id == 12 {
			t.Fatal("consistent lock must not be in the minimal conflict set")
		}
	}
	if len(res.Solutions) != 0 {
		t.Fatal("no solutions should be returned while locks are infeasible")
	}
}

// Locking one of the tied pairs collapses the tie to a single solution.
func TestLockBreaksTie(t *testing.T) {
	st := fixtureState()
	st.Locks = []model.Lock{{ID: 10, VersionID: 1, PeakID: 1, AtomID: 1}}
	res := Solve(st)
	if len(res.Solutions) != 1 {
		t.Fatalf("want 1 solution after locking, got %d", len(res.Solutions))
	}
	m := assignmentsOf(res.Solutions[0])
	if m[1] != 1 || m[2] != 2 {
		t.Fatalf("locked pair not honoured: %v", m)
	}
}
