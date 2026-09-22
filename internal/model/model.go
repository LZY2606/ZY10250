// Package model defines the domain types for the resonance assignment workshop.
package model

// Experiment describes one NMR experiment (one peak list).
type Experiment struct {
	ID        int64
	Name      string
	Dims      []DimSpec // one entry per spectral dimension
	ShiftBias []float64 // system-level shift offset per dimension, adjustable
}

// DimSpec describes one spectral dimension.
type DimSpec struct {
	Nucleus string  // e.g. "1H", "15N", "13C"
	Tol     float64 // tolerance window (ppm) for this nucleus+experiment group
}

// Peak is one entry of a peak list. Overlapped peaks declare their
// capacity explicitly; capacity 1 means exclusive.
type Peak struct {
	ID        int64
	ExpID     int64
	Label     string
	Shifts    []float64 // one per experiment dimension
	Intensity float64
	Overlap   bool
	Capacity  int // declared by input evidence; never assumed infinite
}

// AtomSite is an assignable resonance site.
type AtomSite struct {
	ID       int64
	Label    string
	Expected []float64 // expected shifts aligned with experiment dims (per experiment)
}

// AtomExpected stores expected shifts per (atom, experiment).
type AtomExpected struct {
	AtomID int64
	ExpID  int64
	Shifts []float64
}

// Connectivity is a piece of through-bond/through-space evidence
// linking two atom sites (used for cross-experiment correspondence).
type Connectivity struct {
	ID     int64
	FromID int64
	ToID   int64
	Kind   string // e.g. "bond", "NOE"
	Weight float64
}

// Lock pins a peak-atom pair chosen by the user.
type Lock struct {
	PeakID int64
	AtomID int64
}

// Ops are the user operations that modify a solve run.
type Ops struct {
	Locks        []Lock
	RejectedConn []int64             // connectivity ids rejected by the user
	BiasOverride map[int64][]float64 // experiment id -> per-dim bias
}

// Assignment is one peak -> atom mapping inside a solution.
type Assignment struct {
	PeakID int64
	AtomID int64
	ExpID  int64
	Cost   float64
}

// Solution is one near-optimal assignment scheme.
type Solution struct {
	Score       float64
	Assignments []Assignment
	Unassigned  []int64 // peak ids left unassigned
}

// SolveResult is the full output of a solver run.
type SolveResult struct {
	Version    string     // pinned peak-list version hash
	Solutions  []Solution // multiple near-tied schemes, best first
	Unassigned []int64    // peaks unassigned in the best solution
	Conflicts  []string   // minimal conflict set (human readable)
}
