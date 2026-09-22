// Package model defines the domain types for the resonance assignment workshop.
package model

// Experiment describes one NMR experiment: its dimension nuclei, the
// per-(experiment, nucleus) tolerance windows and a user-adjustable
// systematic shift bias per dimension.
type Experiment struct {
	ID         int64              `json:"id"`
	VersionID  int64              `json:"version_id"`
	Name       string             `json:"name"`
	Nuclei     []string           `json:"nuclei"`     // one per dimension, e.g. ["H","N"]
	Tolerances map[string]float64 `json:"tolerances"` // nucleus -> tolerance window (ppm)
	Bias       map[string]float64 `json:"bias"`       // nucleus -> systematic offset (ppm)
	Window     float64            `json:"window"`     // candidate cut-off in tolerance units
}

// Peak is one observed signal in a peak-list version.
type Peak struct {
	ID           int64     `json:"id"`
	VersionID    int64     `json:"version_id"`
	ExperimentID int64     `json:"experiment_id"`
	Label        string    `json:"label"`
	Shifts       []float64 `json:"shifts"` // one per experiment dimension
	Intensity    float64   `json:"intensity"`
	Overlap      bool      `json:"overlap"`
	// Capacity is declared by the input evidence: an overlapped peak may
	// support at most Capacity atom sites. Never treated as unlimited.
	Capacity int `json:"capacity"`
}

// Atom is an assignable site with reference (expected) shifts per nucleus.
type Atom struct {
	ID        int64              `json:"id"`
	VersionID int64              `json:"version_id"`
	Name      string             `json:"name"`
	Residue   string             `json:"residue"`
	Shifts    map[string]float64 `json:"shifts"` // nucleus -> reference shift
}

// Connectivity is a piece of evidence linking two atom sites
// (e.g. sequential neighbours). It contributes a bonus when both ends
// are assigned. Users may reject an edge to remove its contribution.
type Connectivity struct {
	ID        int64   `json:"id"`
	VersionID int64   `json:"version_id"`
	AtomA     int64   `json:"atom_a"`
	AtomB     int64   `json:"atom_b"`
	Kind      string  `json:"kind"`
	Weight    float64 `json:"weight"`
	Rejected  bool    `json:"rejected"`
}

// Correspondence links two peaks from different experiments that are
// believed to report on the same atom. Validated separately from the
// single-experiment exclusivity constraint.
type Correspondence struct {
	ID        int64 `json:"id"`
	VersionID int64 `json:"version_id"`
	PeakA     int64 `json:"peak_a"`
	PeakB     int64 `json:"peak_b"`
}

// Lock pins a peak-atom pair chosen by the user.
type Lock struct {
	ID        int64 `json:"id"`
	VersionID int64 `json:"version_id"`
	PeakID    int64 `json:"peak_id"`
	AtomID    int64 `json:"atom_id"`
}

// Version is an immutable peak-list version; every conclusion pins one.
type Version struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// State is the full working set for one peak-list version.
type State struct {
	Version        Version          `json:"version"`
	Experiments    []Experiment     `json:"experiments"`
	Peaks          []Peak           `json:"peaks"`
	Atoms          []Atom           `json:"atoms"`
	Connectivities []Connectivity   `json:"connectivities"`
	Correspondence []Correspondence `json:"correspondence"`
	Locks          []Lock           `json:"locks"`
}
