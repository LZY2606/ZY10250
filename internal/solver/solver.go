// Package solver builds the peak-atom bipartite graph and enumerates
// near-optimal assignment schemes.
package solver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"resonance-workshop/internal/model"
)

const (
	// unassignedPenalty is the cost of leaving a peak unassigned.
	unassignedPenalty = 4.0
	// assignReward prefers schemes that assign more peaks/atoms.
	assignReward = 1.0
	// tieEps defines "near-equal" scores kept as alternative schemes.
	tieEps = 0.05
	// maxSolutions caps the number of returned schemes.
	maxSolutions = 5
	// perExpKeep caps matchings kept per experiment before combining.
	perExpKeep = 24
)

type edge struct {
	peakID int64
	atomID int64
	cost   float64
}

// expMatching is one assignment scheme inside a single experiment.
type expMatching struct {
	expID  int64
	assign map[int64]int64 // atomID -> peakID
	cost   float64
}

// VersionHash pins a solve result to the exact peak-list / bias / tolerance state.
func VersionHash(exps []model.Experiment, peaks []model.Peak) string {
	h := sha256.New()
	expsSorted := append([]model.Experiment(nil), exps...)
	sort.Slice(expsSorted, func(i, j int) bool { return expsSorted[i].ID < expsSorted[j].ID })
	for _, e := range expsSorted {
		fmt.Fprintf(h, "E%d:%s:", e.ID, e.Name)
		for d, dim := range e.Dims {
			fmt.Fprintf(h, "%s/%.4f/b%.4f;", dim.Nucleus, dim.Tol, e.ShiftBias[d])
		}
	}
	peaksSorted := append([]model.Peak(nil), peaks...)
	sort.Slice(peaksSorted, func(i, j int) bool { return peaksSorted[i].ID < peaksSorted[j].ID })
	for _, p := range peaksSorted {
		fmt.Fprintf(h, "P%d:%d:%d:", p.ID, p.ExpID, p.Capacity)
		for _, s := range p.Shifts {
			fmt.Fprintf(h, "%.4f;", s)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// candidateEdges builds bipartite edges for one experiment. A peak matches an
// atom only when EVERY dimension is inside its own tolerance window; distances
// are scaled by the per-(nucleus, experiment) tolerance, never raw-summed.
func candidateEdges(exp model.Experiment, peaks []model.Peak, expected map[int64][]float64, bias []float64) map[int64][]edge {
	out := map[int64][]edge{}
	for _, p := range peaks {
		if p.ExpID != exp.ID {
			continue
		}
		for atomID, expShifts := range expected {
			ok := true
			cost := 0.0
			for d := range exp.Dims {
				delta := p.Shifts[d] - (expShifts[d] + bias[d])
				tol := exp.Dims[d].Tol
				if delta < 0 {
					delta = -delta
				}
				if delta > tol {
					ok = false
					break
				}
				scaled := delta / tol
				cost += scaled * scaled
			}
			if ok {
				out[atomID] = append(out[atomID], edge{peakID: p.ID, atomID: atomID, cost: cost})
			}
		}
	}
	return out
}

// enumerateMatchings enumerates per-experiment matchings. An atom is used at
// most once per experiment (single-experiment exclusion); an overlapped peak
// accepts at most its declared capacity of atoms.
func enumerateMatchings(exp model.Experiment, peaks []model.Peak, cand map[int64][]edge, locks []model.Lock) []expMatching {
	cap := map[int64]int{}
	for _, p := range peaks {
		if p.ExpID == exp.ID {
			c := p.Capacity
			if c < 1 {
				c = 1
			}
			cap[p.ID] = c
		}
	}
	lockByAtom := map[int64]int64{}
	for _, l := range locks {
		// locks apply to the experiment of their peak
		for _, p := range peaks {
			if p.ID == l.PeakID && p.ExpID == exp.ID {
				lockByAtom[l.AtomID] = l.PeakID
			}
		}
	}
	atoms := make([]int64, 0, len(cand)+len(lockByAtom))
	seen := map[int64]bool{}
	for a := range cand {
		atoms = append(atoms, a)
		seen[a] = true
	}
	for a := range lockByAtom {
		if !seen[a] {
			atoms = append(atoms, a)
		}
	}
	sort.Slice(atoms, func(i, j int) bool { return atoms[i] < atoms[j] })

	used := map[int64]int{} // peakID -> atoms assigned
	var results []expMatching
	var rec func(idx int, assign map[int64]int64, cost float64)
	rec = func(idx int, assign map[int64]int64, cost float64) {
		if len(results) >= perExpKeep*4 {
			return
		}
		if idx == len(atoms) {
			cp := map[int64]int64{}
			for k, v := range assign {
				cp[k] = v
			}
			results = append(results, expMatching{expID: exp.ID, assign: cp, cost: cost})
			return
		}
		atom := atoms[idx]
		// option 1: atom unassigned in this experiment
		rec(idx+1, assign, cost)
		// option 2: atom assigned to one of its candidate peaks
		if forced, ok := lockByAtom[atom]; ok {
			if used[forced] < cap[forced] {
				c := edgeCost(cand[atom], forced)
				used[forced]++
				assign[atom] = forced
				rec(idx+1, assign, cost+c)
				delete(assign, atom)
				used[forced]--
			}
			return
		}
		for _, e := range cand[atom] {
			if used[e.peakID] >= cap[e.peakID] {
				continue
			}
			used[e.peakID]++
			assign[atom] = e.peakID
			rec(idx+1, assign, cost+e.cost)
			delete(assign, atom)
			used[e.peakID]--
		}
	}
	rec(0, map[int64]int64{}, 0)
	sort.Slice(results, func(i, j int) bool { return results[i].cost < results[j].cost })
	if len(results) > perExpKeep {
		results = results[:perExpKeep]
	}
	return results
}

func edgeCost(edges []edge, peakID int64) float64 {
	for _, e := range edges {
		if e.peakID == peakID {
			return e.cost
		}
	}
	return unassignedPenalty
}

// Solve runs the full assignment and returns near-tied schemes.
func Solve(exps []model.Experiment, peaks []model.Peak, atoms []model.AtomSite,
	expected []model.AtomExpected, conns []model.Connectivity, ops model.Ops) model.SolveResult {

	expMap := map[int64]model.Experiment{}
	for _, e := range exps {
		if b, ok := ops.BiasOverride[e.ID]; ok {
			e.ShiftBias = append([]float64(nil), b...)
		}
		expMap[e.ID] = e
	}
	expByID := map[int64][]model.AtomExpected{}
	_ = expByID
	expectedMap := map[int64]map[int64][]float64{} // expID -> atomID -> shifts
	for _, ex := range expected {
		if expectedMap[ex.ExpID] == nil {
			expectedMap[ex.ExpID] = map[int64][]float64{}
		}
		expectedMap[ex.ExpID][ex.AtomID] = ex.Shifts
	}

	version := VersionHash(func() []model.Experiment {
		out := make([]model.Experiment, 0, len(expMap))
		for _, e := range expMap {
			out = append(out, e)
		}
		return out
	}(), peaks)

	// Minimal conflict set: find the smallest subset of locks whose removal
	// restores feasibility, if the full lock set is infeasible.
	conflicts := []string{}
	activeLocks := ops.Locks
	if !locksFeasible(exps, peaks, expectedMap, expMap, ops.Locks) {
		best := minimalConflictLocks(ops.Locks, func(ls []model.Lock) bool {
			return locksFeasible(exps, peaks, expectedMap, expMap, ls)
		})
		drop := map[int]bool{}
		for _, i := range best {
			drop[i] = true
			conflicts = append(conflicts, fmt.Sprintf("lock peak=%d atom=%d", ops.Locks[i].PeakID, ops.Locks[i].AtomID))
		}
		kept := []model.Lock{}
		for i, l := range ops.Locks {
			if !drop[i] {
				kept = append(kept, l)
			}
		}
		activeLocks = kept
	}

	// Per-experiment matchings.
	expIDs := make([]int64, 0, len(expMap))
	for id := range expMap {
		expIDs = append(expIDs, id)
	}
	sort.Slice(expIDs, func(i, j int) bool { return expIDs[i] < expIDs[j] })
	perExp := [][]expMatching{}
	for _, id := range expIDs {
		e := expMap[id]
		cand := candidateEdges(e, peaks, expectedMap[id], e.ShiftBias)
		perExp = append(perExp, enumerateMatchings(e, peaks, cand, activeLocks))
	}

	// Active connectivity evidence (rejected edges removed).
	rejected := map[int64]bool{}
	for _, id := range ops.RejectedConn {
		rejected[id] = true
	}
	activeConns := []model.Connectivity{}
	for _, c := range conns {
		if !rejected[c.ID] {
			activeConns = append(activeConns, c)
		}
	}

	// Combine per-experiment matchings; connectivity bonuses couple
	// experiments (cross-experiment correspondence), scored separately
	// from the single-experiment exclusion enforced above.
	type combined struct {
		score float64
		parts []expMatching
	}
	var combos []combined
	var rec func(i int, score float64, parts []expMatching)
	rec = func(i int, score float64, parts []expMatching) {
		if i == len(perExp) {
			// connectivity bonus: both endpoints assigned in a shared experiment
			bonus := 0.0
			for _, c := range activeConns {
				for _, m := range parts {
					_, okA := m.assign[c.FromID]
					_, okB := m.assign[c.ToID]
					if okA && okB {
						bonus += c.Weight
						break
					}
				}
			}
			cp := append([]expMatching(nil), parts...)
			combos = append(combos, combined{score: score - bonus, parts: cp})
			return
		}
		for _, m := range perExp[i] {
			rec(i+1, score+m.cost, append(parts, m))
		}
	}
	rec(0, 0, nil)

	// Unassigned penalty per peak, applied on the combined assignment.
	peakExp := map[int64]int64{}
	for _, p := range peaks {
		peakExp[p.ID] = p.ExpID
	}
	finalize := func(c combined) model.Solution {
		assigned := map[int64]bool{}
		sol := model.Solution{Score: c.score}
		for _, m := range c.parts {
			for atomID, peakID := range m.assign {
				assigned[peakID] = true
				sol.Score -= assignReward
				sol.Assignments = append(sol.Assignments, model.Assignment{
					PeakID: peakID, AtomID: atomID, ExpID: m.expID,
				})
			}
		}
		for _, p := range peaks {
			if !assigned[p.ID] {
				sol.Unassigned = append(sol.Unassigned, p.ID)
				sol.Score += unassignedPenalty
			}
		}
		sort.Slice(sol.Unassigned, func(i, j int) bool { return sol.Unassigned[i] < sol.Unassigned[j] })
		sort.Slice(sol.Assignments, func(i, j int) bool {
			if sol.Assignments[i].ExpID != sol.Assignments[j].ExpID {
				return sol.Assignments[i].ExpID < sol.Assignments[j].ExpID
			}
			return sol.Assignments[i].PeakID < sol.Assignments[j].PeakID
		})
		return sol
	}

	sols := []model.Solution{}
	seenKey := map[string]bool{}
	for _, c := range combos {
		s := finalize(c)
		key := fmt.Sprintf("%v", s.Assignments)
		if seenKey[key] {
			continue
		}
		seenKey[key] = true
		sols = append(sols, s)
	}
	sort.Slice(sols, func(i, j int) bool { return sols[i].Score < sols[j].Score })
	if len(sols) == 0 {
		return model.SolveResult{Version: version, Conflicts: conflicts}
	}
	best := sols[0].Score
	kept := []model.Solution{}
	for _, s := range sols {
		if s.Score-best <= tieEps && len(kept) < maxSolutions {
			kept = append(kept, s)
		}
	}
	return model.SolveResult{
		Version:    version,
		Solutions:  kept,
		Unassigned: kept[0].Unassigned,
		Conflicts:  conflicts,
	}
}

// locksFeasible reports whether the lock set can be satisfied at all:
// no two locks force the same atom onto different peaks of one experiment,
// and no peak is locked beyond its declared capacity.
func locksFeasible(exps []model.Experiment, peaks []model.Peak,
	expectedMap map[int64]map[int64][]float64, expMap map[int64]model.Experiment,
	locks []model.Lock) bool {
	peakByID := map[int64]model.Peak{}
	for _, p := range peaks {
		peakByID[p.ID] = p
	}
	atomExp := map[string]int64{} // atom+exp -> peak
	peakCount := map[int64]int{}  // peak -> locked atoms
	for _, l := range locks {
		p, ok := peakByID[l.PeakID]
		if !ok {
			return false
		}
		key := fmt.Sprintf("%d/%d", l.AtomID, p.ExpID)
		if prev, ok := atomExp[key]; ok && prev != l.PeakID {
			return false
		}
		atomExp[key] = l.PeakID
		peakCount[l.PeakID]++
		c := p.Capacity
		if c < 1 {
			c = 1
		}
		if peakCount[l.PeakID] > c {
			return false
		}
		// locked pair must satisfy the tolerance window itself
		e := expMap[p.ExpID]
		expShifts, ok := expectedMap[p.ExpID][l.AtomID]
		if !ok {
			return false
		}
		for d := range e.Dims {
			delta := p.Shifts[d] - (expShifts[d] + e.ShiftBias[d])
			if delta < 0 {
				delta = -delta
			}
			if delta > e.Dims[d].Tol {
				return false
			}
		}
	}
	return true
}

// minimalConflictLocks finds the smallest subset of lock indices whose
// removal makes the set feasible.
func minimalConflictLocks(locks []model.Lock, feasible func([]model.Lock) bool) []int {
	n := len(locks)
	for size := 1; size <= n; size++ {
		var best []int
		var rec func(start, left int, chosen []int)
		rec = func(start, left int, chosen []int) {
			if best != nil {
				return
			}
			if left == 0 {
				kept := []model.Lock{}
				drop := map[int]bool{}
				for _, i := range chosen {
					drop[i] = true
				}
				for i, l := range locks {
					if !drop[i] {
						kept = append(kept, l)
					}
				}
				if feasible(kept) {
					best = append([]int(nil), chosen...)
				}
				return
			}
			for i := start; i <= n-left; i++ {
				rec(i+1, left-1, append(chosen, i))
			}
		}
		rec(0, size, nil)
		if best != nil {
			return best
		}
	}
	return nil
}
