// Package fixture seeds the fixed demonstration data set: one overlapped
// peak with declared capacity, exactly two equal-scoring shortest
// solutions, and one experiment-level systematic shift bias.
package fixture

import (
	"resonance-workshop/internal/model"
	"resonance-workshop/internal/store"
)

// Seed wipes nothing; it appends a fresh fixture version and returns its ID.
func Seed(s *store.Store) (int64, error) {
	vid, err := s.CreateVersion("fixture-v1")
	if err != nil {
		return 0, err
	}

	hsqc := model.Experiment{
		VersionID:  vid,
		Name:       "HSQC",
		Nuclei:     []string{"H", "N"},
		Tolerances: map[string]float64{"H": 0.05, "N": 0.5},
		Bias:       map[string]float64{"H": 0, "N": 0},
		Window:     1.5,
	}
	if err := s.AddExperiment(&hsqc); err != nil {
		return 0, err
	}
	hnca := model.Experiment{
		VersionID:  vid,
		Name:       "HNCA",
		Nuclei:     []string{"H", "C"},
		Tolerances: map[string]float64{"H": 0.05, "C": 0.6},
		// Experiment-level systematic bias: acceptance adjusts this value
		// and observes the candidate set change.
		Bias:   map[string]float64{"H": 0.02, "C": 0},
		Window: 1.5,
	}
	if err := s.AddExperiment(&hnca); err != nil {
		return 0, err
	}

	peaks := []model.Peak{
		{VersionID: vid, ExperimentID: hsqc.ID, Label: "hsqc-1", Shifts: []float64{8.01, 120.1}, Intensity: 1.0, Capacity: 1},
		{VersionID: vid, ExperimentID: hsqc.ID, Label: "hsqc-2", Shifts: []float64{8.02, 119.9}, Intensity: 0.9, Capacity: 1},
		{VersionID: vid, ExperimentID: hsqc.ID, Label: "hsqc-3", Shifts: []float64{7.50, 115.1}, Intensity: 0.8, Capacity: 1},
		// Overlapped peak: capacity 2 is declared by the input evidence.
		{VersionID: vid, ExperimentID: hsqc.ID, Label: "hsqc-ovl", Shifts: []float64{7.25, 112.2}, Intensity: 2.4, Overlap: true, Capacity: 2},
		// No atom inside the tolerance window: stays unassigned.
		{VersionID: vid, ExperimentID: hsqc.ID, Label: "hsqc-lone", Shifts: []float64{9.50, 130.0}, Intensity: 0.2, Capacity: 1},
		{VersionID: vid, ExperimentID: hnca.ID, Label: "hnca-1", Shifts: []float64{8.03, 55.1}, Intensity: 0.7, Capacity: 1},
		{VersionID: vid, ExperimentID: hnca.ID, Label: "hnca-2", Shifts: []float64{7.52, 58.1}, Intensity: 0.6, Capacity: 1},
	}
	for i := range peaks {
		if err := s.AddPeak(&peaks[i]); err != nil {
			return 0, err
		}
	}

	atoms := []model.Atom{
		{VersionID: vid, Name: "Gly1-HN", Residue: "G1", Shifts: map[string]float64{"H": 8.00, "N": 120.0, "C": 55.0}},
		{VersionID: vid, Name: "Ala2-HN", Residue: "A2", Shifts: map[string]float64{"H": 8.00, "N": 120.0, "C": 55.4}},
		{VersionID: vid, Name: "Ser3-HN", Residue: "S3", Shifts: map[string]float64{"H": 7.50, "N": 115.0, "C": 58.0}},
		{VersionID: vid, Name: "Thr4-HN", Residue: "T4", Shifts: map[string]float64{"H": 7.20, "N": 112.0, "C": 62.0}},
		{VersionID: vid, Name: "Val5-HN", Residue: "V5", Shifts: map[string]float64{"H": 7.28, "N": 112.5, "C": 62.5}},
		{VersionID: vid, Name: "Leu6-HN", Residue: "L6", Shifts: map[string]float64{"H": 7.21, "N": 111.8, "C": 60.0}},
	}
	for i := range atoms {
		if err := s.AddAtom(&atoms[i]); err != nil {
			return 0, err
		}
	}

	conns := []model.Connectivity{
		{VersionID: vid, AtomA: atoms[1].ID, AtomB: atoms[2].ID, Kind: "sequential", Weight: 0.3},
		{VersionID: vid, AtomA: atoms[2].ID, AtomB: atoms[3].ID, Kind: "sequential", Weight: 0.3},
		{VersionID: vid, AtomA: atoms[3].ID, AtomB: atoms[4].ID, Kind: "sequential", Weight: 0.3},
	}
	for i := range conns {
		if err := s.AddConnectivity(&conns[i]); err != nil {
			return 0, err
		}
	}

	// Cross-experiment correspondence: validated separately from the
	// single-experiment exclusivity constraint.
	corrs := []model.Correspondence{
		{VersionID: vid, PeakA: peaks[0].ID, PeakB: peaks[5].ID}, // hsqc-1 <-> hnca-1
		{VersionID: vid, PeakA: peaks[2].ID, PeakB: peaks[6].ID}, // hsqc-3 <-> hnca-2
	}
	for i := range corrs {
		if err := s.AddCorrespondence(&corrs[i]); err != nil {
			return 0, err
		}
	}
	return vid, nil
}
