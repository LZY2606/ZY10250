package solver

import "resonance-workshop/internal/model"

// CandidateEdge is one displayed bipartite edge between a peak and an atom.
type CandidateEdge struct {
	PeakID int64   `json:"peak_id"`
	AtomID int64   `json:"atom_id"`
	ExpID  int64   `json:"exp_id"`
	Cost   float64 `json:"cost"`
}

// Candidates lists every candidate edge under the given bias overrides,
// using per-dimension tolerance windows (never raw-summed distances).
func Candidates(exps []model.Experiment, peaks []model.Peak,
	expected []model.AtomExpected, bias map[int64][]float64) []CandidateEdge {
	expectedMap := map[int64]map[int64][]float64{}
	for _, ex := range expected {
		if expectedMap[ex.ExpID] == nil {
			expectedMap[ex.ExpID] = map[int64][]float64{}
		}
		expectedMap[ex.ExpID][ex.AtomID] = ex.Shifts
	}
	var out []CandidateEdge
	for _, e := range exps {
		b := e.ShiftBias
		if ov, ok := bias[e.ID]; ok {
			b = ov
		}
		cand := candidateEdges(e, peaks, expectedMap[e.ID], b)
		for atomID, edges := range cand {
			for _, ed := range edges {
				out = append(out, CandidateEdge{
					PeakID: ed.peakID, AtomID: atomID, ExpID: e.ID, Cost: ed.cost,
				})
			}
		}
	}
	return out
}
