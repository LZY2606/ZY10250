// Package solver builds the peak<->atom bipartite graph and enumerates
// the K best-scoring assignment solutions under capacity, exclusivity,
// lock and correspondence constraints.
package solver

import (
	"math"
	"sort"

	"resonance-workshop/internal/model"
)

const (
	// ScoreEpsilon defines "near-equal": solutions within this absolute
	// score distance of the best one are all returned.
	ScoreEpsilon = 1e-6
	maxSolutions = 8
	maxEnumerate = 400000
)

// Candidate is one bipartite edge between a peak and an atom site.
type Candidate struct {
	PeakID   int64   `json:"peak_id"`
	AtomID   int64   `json:"atom_id"`
	Distance float64 `json:"distance"` // scale-aware normalised distance
	Score    float64 `json:"score"`
}

// Assignment is one decided pair inside a solution.
type Assignment struct {
	PeakID    int64   `json:"peak_id"`
	AtomID    int64   `json:"atom_id"`
	EdgeScore float64 `json:"edge_score"`
}

// Solution is one complete assignment alternative.
type Solution struct {
	Assignments []Assignment `json:"assignments"`
	Score       float64      `json:"score"`
	// CorrViolations lists correspondence edge IDs violated by this
	// solution. Checked separately from single-experiment exclusivity.
	CorrViolations []int64 `json:"correspondence_violations"`
}

// Result is the solver output, always pinned to a peak-list version.
type Result struct {
	VersionID   int64       `json:"version_id"`
	Candidates  []Candidate `json:"candidates"`
	Solutions   []Solution  `json:"solutions"`
	Unassigned  []int64     `json:"unassigned_peak_ids"`
	ConflictSet []int64     `json:"conflict_lock_ids"` // minimal infeasible lock subset
	Truncated   bool        `json:"truncated"`
}

// NormalizedDistance combines per-dimension offsets with per-(experiment,
// nucleus) tolerances. Each dimension is scaled by its own tolerance and
// combined as a Euclidean norm; raw ppm distances are never summed across
// dimensions with different scales.
func NormalizedDistance(exp model.Experiment, peak model.Peak, atom model.Atom) float64 {
	var sumSq float64
	for i, nuc := range exp.Nuclei {
		tol := exp.Tolerances[nuc]
		if tol <= 0 {
			tol = 1
		}
		z := (peak.Shifts[i] - exp.Bias[nuc] - atom.Shifts[nuc]) / tol
		sumSq += z * z
	}
	return math.Sqrt(sumSq)
}

// Candidates builds all bipartite edges inside the tolerance window.
func Candidates(st model.State) []Candidate {
	exps := map[int64]model.Experiment{}
	for _, e := range st.Experiments {
		exps[e.ID] = e
	}
	var out []Candidate
	for _, p := range st.Peaks {
		exp := exps[p.ExperimentID]
		for _, a := range st.Atoms {
			d := NormalizedDistance(exp, p, a)
			if d <= exp.Window {
				out = append(out, Candidate{
					PeakID:   p.ID,
					AtomID:   a.ID,
					Distance: d,
					Score:    1 - d/exp.Window,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PeakID != out[j].PeakID {
			return out[i].PeakID < out[j].PeakID
		}
		return out[i].Distance < out[j].Distance
	})
	return out
}

// locksFeasible reports whether the lock set alone is satisfiable under
// per-experiment exclusivity and declared peak capacities.
func locksFeasible(st model.State, locks []model.Lock) bool {
	peakByID := map[int64]model.Peak{}
	for _, p := range st.Peaks {
		peakByID[p.ID] = p
	}
	atomExp := map[[2]int64]bool{} // (experiment, atom) exclusivity
	peakCount := map[int64]int{}
	for _, l := range locks {
		p := peakByID[l.PeakID]
		key := [2]int64{p.ExperimentID, l.AtomID}
		if atomExp[key] {
			return false
		}
		atomExp[key] = true
		peakCount[l.PeakID]++
		cap := p.Capacity
		if cap < 1 {
			cap = 1
		}
		if peakCount[l.PeakID] > cap {
			return false
		}
	}
	return true
}

// minimalConflictSet reduces an infeasible lock set to an irreducible
// subset by greedy deletion: removing any remaining lock makes it feasible.
func minimalConflictSet(st model.State, locks []model.Lock) []model.Lock {
	bad := append([]model.Lock(nil), locks...)
	changed := true
	for changed {
		changed = false
		for i := 0; i < len(bad); i++ {
			trial := append(append([]model.Lock(nil), bad[:i]...), bad[i+1:]...)
			if !locksFeasible(st, trial) {
				bad = trial
				changed = true
				break
			}
		}
	}
	return bad
}

// Solve enumerates the K best solutions for the given state.
func Solve(st model.State) Result {
	cands := Candidates(st)
	res := Result{VersionID: st.Version.ID, Candidates: cands}

	if !locksFeasible(st, st.Locks) {
		bad := minimalConflictSet(st, st.Locks)
		for _, l := range bad {
			res.ConflictSet = append(res.ConflictSet, l.ID)
		}
		return res
	}

	byPeak := map[int64][]Candidate{}
	for _, c := range cands {
		byPeak[c.PeakID] = append(byPeak[c.PeakID], c)
	}
	lockedBy := map[int64][]int64{} // peak -> locked atoms
	for _, l := range st.Locks {
		lockedBy[l.PeakID] = append(lockedBy[l.PeakID], l.AtomID)
	}

	peaks := append([]model.Peak(nil), st.Peaks...)
	sort.Slice(peaks, func(i, j int) bool { return peaks[i].ID < peaks[j].ID })

	type partial struct {
		assign map[int64][]int64 // peak -> atoms (len <= declared capacity)
		score  float64
	}
	atomExpUsed := map[[2]int64]bool{}
	connBonusOf := func(assigned map[int64]bool) float64 {
		var b float64
		for _, cn := range st.Connectivities {
			if cn.Rejected {
				continue
			}
			if assigned[cn.AtomA] && assigned[cn.AtomB] {
				b += cn.Weight
			}
		}
		return b
	}

	var sols []partial
	steps := 0
	truncated := false
	// optionsFor enumerates every allowed atom subset for one peak:
	// the empty choice plus all subsets up to the declared capacity that
	// include every locked atom.
	optionsFor := func(p model.Peak) [][]Candidate {
		cs := byPeak[p.ID]
		cap := p.Capacity
		if cap < 1 {
			cap = 1
		}
		locked := map[int64]bool{}
		for _, a := range lockedBy[p.ID] {
			locked[a] = true
		}
		var out [][]Candidate
		var cur []Candidate
		var rec func(start int)
		rec = func(start int) {
			if len(cur) > cap {
				return
			}
			ok := true
			for a := range locked {
				found := false
				for _, c := range cur {
					if c.AtomID == a {
						found = true
					}
				}
				if !found {
					ok = false
					break
				}
			}
			if ok {
				cp := append([]Candidate(nil), cur...)
				out = append(out, cp)
			}
			for i := start; i < len(cs); i++ {
				cur = append(cur, cs[i])
				rec(i + 1)
				cur = cur[:len(cur)-1]
			}
		}
		rec(0)
		return out
	}

	var walk func(i int, assign map[int64][]int64, score float64)
	walk = func(i int, assign map[int64][]int64, score float64) {
		if truncated {
			return
		}
		steps++
		if steps > maxEnumerate {
			truncated = true
			return
		}
		if i == len(peaks) {
			cp := map[int64][]int64{}
			for k, v := range assign {
				cp[k] = append([]int64(nil), v...)
			}
			sols = append(sols, partial{assign: cp, score: score})
			return
		}
		p := peaks[i]
		for _, opt := range optionsFor(p) {
			var keys [][2]int64
			ok := true
			sum := 0.0
			for _, c := range opt {
				key := [2]int64{p.ExperimentID, c.AtomID}
				if atomExpUsed[key] { // single-experiment exclusivity
					ok = false
					break
				}
				keys = append(keys, key)
				sum += c.Score
			}
			if !ok {
				continue
			}
			for _, k := range keys {
				atomExpUsed[k] = true
			}
			if len(opt) > 0 {
				atoms := make([]int64, 0, len(opt))
				for _, c := range opt {
					atoms = append(atoms, c.AtomID)
				}
				assign[p.ID] = atoms
			}
			walk(i+1, assign, score+sum)
			delete(assign, p.ID)
			for _, k := range keys {
				delete(atomExpUsed, k)
			}
		}
	}
	walk(0, map[int64][]int64{}, 0)

	// Add connectivity bonus, rank, and keep near-best distinct solutions.
	for i := range sols {
		assigned := map[int64]bool{}
		for _, atoms := range sols[i].assign {
			for _, a := range atoms {
				assigned[a] = true
			}
		}
		sols[i].score += connBonusOf(assigned)
	}
	sort.Slice(sols, func(i, j int) bool { return sols[i].score > sols[j].score })
	seen := map[string]bool{}
	corrByPeak := map[int64][]model.Correspondence{}
	for _, c := range st.Correspondence {
		corrByPeak[c.PeakA] = append(corrByPeak[c.PeakA], c)
		corrByPeak[c.PeakB] = append(corrByPeak[c.PeakB], c)
	}
	atomsOf := func(s partial, peakID int64) []int64 {
		return s.assign[peakID]
	}
	best := math.Inf(-1)
	if len(sols) > 0 {
		best = sols[0].score
	}
	for _, s := range sols {
		if len(res.Solutions) >= maxSolutions {
			break
		}
		if s.score < best-ScoreEpsilon && len(res.Solutions) > 0 {
			break
		}
		key := solutionKey(s.assign)
		if seen[key] {
			continue
		}
		seen[key] = true
		sol := Solution{Score: s.score}
		ids := make([]int64, 0, len(s.assign))
		for pid := range s.assign {
			ids = append(ids, pid)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, pid := range ids {
			for _, atomID := range s.assign[pid] {
				es := 0.0
				for _, c := range byPeak[pid] {
					if c.AtomID == atomID {
						es = c.Score
					}
				}
				sol.Assignments = append(sol.Assignments, Assignment{PeakID: pid, AtomID: atomID, EdgeScore: es})
			}
		}
		// Cross-experiment correspondence, validated independently.
		seenCorr := map[int64]bool{}
		for pid := range s.assign {
			for _, c := range corrByPeak[pid] {
				if seenCorr[c.ID] {
					continue
				}
				seenCorr[c.ID] = true
				a1 := atomsOf(s, c.PeakA)
				a2 := atomsOf(s, c.PeakB)
				if len(a1) > 0 && len(a2) > 0 && !shareAtom(a1, a2) {
					sol.CorrViolations = append(sol.CorrViolations, c.ID)
				}
			}
		}
		res.Solutions = append(res.Solutions, sol)
	}
	res.Truncated = truncated

	// Unassigned peaks of the best solution.
	if len(res.Solutions) > 0 {
		used := map[int64]bool{}
		for _, a := range res.Solutions[0].Assignments {
			used[a.PeakID] = true
		}
		for _, p := range st.Peaks {
			if !used[p.ID] {
				res.Unassigned = append(res.Unassigned, p.ID)
			}
		}
	} else {
		for _, p := range st.Peaks {
			res.Unassigned = append(res.Unassigned, p.ID)
		}
	}
	return res
}

func shareAtom(a, b []int64) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func solutionKey(assign map[int64][]int64) string {
	ids := make([]int64, 0, len(assign))
	for pid := range assign {
		ids = append(ids, pid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	b := make([]byte, 0, len(ids)*16)
	for _, pid := range ids {
		atoms := append([]int64(nil), assign[pid]...)
		sort.Slice(atoms, func(i, j int) bool { return atoms[i] < atoms[j] })
		b = append(b, []byte(itoa(pid))...)
		b = append(b, ':')
		for _, a := range atoms {
			b = append(b, []byte(itoa(a))...)
			b = append(b, ',')
		}
		b = append(b, ';')
	}
	return string(b)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
