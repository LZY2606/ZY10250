package store

import "resonance-workshop/internal/model"

// Fixture returns the fixed demo dataset: one overlapped peak (declared
// capacity 2), two equal-cost shortest assignment schemes (A1/A2 swap),
// and one experiment-level shift bias (EXP2 13C dimension).
func Fixture() *Dataset {
	return &Dataset{
		Experiments: []model.Experiment{
			{ID: 1, Name: "HSQC-15N",
				Dims: []model.DimSpec{
					{Nucleus: "1H", Tol: 0.03},
					{Nucleus: "15N", Tol: 0.30},
				},
				ShiftBias: []float64{0, 0}},
			{ID: 2, Name: "HNCO",
				Dims: []model.DimSpec{
					{Nucleus: "1H", Tol: 0.03},
					{Nucleus: "13C", Tol: 0.25},
				},
				ShiftBias: []float64{0, 0}},
		},
		Peaks: []model.Peak{
			{ID: 1, ExpID: 1, Label: "P1", Shifts: []float64{8.01, 120.05}, Intensity: 1.0, Capacity: 1},
			// P2 is the overlapped peak; capacity 2 is declared by input evidence.
			{ID: 2, ExpID: 1, Label: "P2(ovl)", Shifts: []float64{8.02, 120.10}, Intensity: 2.4, Overlap: true, Capacity: 2},
			{ID: 3, ExpID: 1, Label: "P3", Shifts: []float64{7.50, 118.00}, Intensity: 0.9, Capacity: 1},
			// EXP2 peaks carry a +0.2 ppm systematic 13C offset; adjusting the
			// experiment bias changes the candidate set (Q2 only matches with bias).
			{ID: 4, ExpID: 2, Label: "Q1", Shifts: []float64{8.01, 55.20}, Intensity: 1.1, Capacity: 1},
			{ID: 5, ExpID: 2, Label: "Q2", Shifts: []float64{7.50, 58.40}, Intensity: 0.8, Capacity: 1},
			{ID: 6, ExpID: 2, Label: "Q3", Shifts: []float64{8.60, 60.20}, Intensity: 1.0, Capacity: 1},
		},
		Atoms: []model.AtomSite{
			{ID: 1, Label: "A1"}, {ID: 2, Label: "A2"},
			{ID: 3, Label: "A3"}, {ID: 4, Label: "A4"},
		},
		Expected: []model.AtomExpected{
			// A1 and A2 have identical expected shifts -> two tied shortest schemes.
			{AtomID: 1, ExpID: 1, Shifts: []float64{8.00, 120.0}},
			{AtomID: 2, ExpID: 1, Shifts: []float64{8.00, 120.0}},
			{AtomID: 3, ExpID: 1, Shifts: []float64{7.50, 118.0}},
			{AtomID: 4, ExpID: 1, Shifts: []float64{8.60, 122.0}},
			{AtomID: 1, ExpID: 2, Shifts: []float64{8.00, 55.0}},
			{AtomID: 2, ExpID: 2, Shifts: []float64{8.00, 55.0}},
			{AtomID: 3, ExpID: 2, Shifts: []float64{7.50, 58.0}},
			{AtomID: 4, ExpID: 2, Shifts: []float64{8.60, 60.0}},
		},
		Conns: []model.Connectivity{
			{ID: 1, FromID: 3, ToID: 4, Kind: "bond", Weight: 1.0},
			{ID: 2, FromID: 1, ToID: 2, Kind: "NOE", Weight: 0.5},
		},
	}
}

// Seed writes the fixture into an (empty) database.
func (s *Store) Seed() error {
	d := Fixture()
	for _, e := range d.Experiments {
		if err := s.SaveExperiment(e); err != nil {
			return err
		}
	}
	for _, p := range d.Peaks {
		if err := s.SavePeak(p); err != nil {
			return err
		}
	}
	for _, a := range d.Atoms {
		if err := s.SaveAtom(a); err != nil {
			return err
		}
	}
	for _, e := range d.Expected {
		if err := s.SaveExpected(e); err != nil {
			return err
		}
	}
	for _, c := range d.Conns {
		if err := s.SaveConnectivity(c); err != nil {
			return err
		}
	}
	return nil
}
