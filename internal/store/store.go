// Package store persists workshop state and run logs in SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"resonance-workshop/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS experiments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  nuclei TEXT NOT NULL,
  tolerances TEXT NOT NULL,
  bias TEXT NOT NULL,
  window REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS peaks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  experiment_id INTEGER NOT NULL,
  label TEXT NOT NULL,
  shifts TEXT NOT NULL,
  intensity REAL NOT NULL,
  overlap INTEGER NOT NULL,
  capacity INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS atoms (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  residue TEXT NOT NULL,
  shifts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS connectivities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  atom_a INTEGER NOT NULL,
  atom_b INTEGER NOT NULL,
  kind TEXT NOT NULL,
  weight REAL NOT NULL,
  rejected INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS correspondences (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  peak_a INTEGER NOT NULL,
  peak_b INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS locks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  peak_id INTEGER NOT NULL,
  atom_id INTEGER NOT NULL,
  UNIQUE(version_id, peak_id, atom_id)
);
CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version_id INTEGER NOT NULL,
  action TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`)
	return err
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// CreateVersion inserts a new peak-list version and returns its ID.
func (s *Store) CreateVersion(name string) (int64, error) {
	r, err := s.db.Exec(`INSERT INTO versions(name, created_at) VALUES(?,?)`, name, now())
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) AddExperiment(e *model.Experiment) error {
	nuclei, _ := json.Marshal(e.Nuclei)
	tol, _ := json.Marshal(e.Tolerances)
	bias, _ := json.Marshal(e.Bias)
	r, err := s.db.Exec(`INSERT INTO experiments(version_id,name,nuclei,tolerances,bias,window) VALUES(?,?,?,?,?,?)`,
		e.VersionID, e.Name, string(nuclei), string(tol), string(bias), e.Window)
	if err != nil {
		return err
	}
	e.ID, _ = r.LastInsertId()
	return nil
}

func (s *Store) AddPeak(p *model.Peak) error {
	shifts, _ := json.Marshal(p.Shifts)
	ov := 0
	if p.Overlap {
		ov = 1
	}
	r, err := s.db.Exec(`INSERT INTO peaks(version_id,experiment_id,label,shifts,intensity,overlap,capacity) VALUES(?,?,?,?,?,?,?)`,
		p.VersionID, p.ExperimentID, p.Label, string(shifts), p.Intensity, ov, p.Capacity)
	if err != nil {
		return err
	}
	p.ID, _ = r.LastInsertId()
	return nil
}

func (s *Store) AddAtom(a *model.Atom) error {
	shifts, _ := json.Marshal(a.Shifts)
	r, err := s.db.Exec(`INSERT INTO atoms(version_id,name,residue,shifts) VALUES(?,?,?,?)`,
		a.VersionID, a.Name, a.Residue, string(shifts))
	if err != nil {
		return err
	}
	a.ID, _ = r.LastInsertId()
	return nil
}

func (s *Store) AddConnectivity(c *model.Connectivity) error {
	r, err := s.db.Exec(`INSERT INTO connectivities(version_id,atom_a,atom_b,kind,weight,rejected) VALUES(?,?,?,?,?,0)`,
		c.VersionID, c.AtomA, c.AtomB, c.Kind, c.Weight)
	if err != nil {
		return err
	}
	c.ID, _ = r.LastInsertId()
	return nil
}

func (s *Store) AddCorrespondence(c *model.Correspondence) error {
	r, err := s.db.Exec(`INSERT INTO correspondences(version_id,peak_a,peak_b) VALUES(?,?,?)`,
		c.VersionID, c.PeakA, c.PeakB)
	if err != nil {
		return err
	}
	c.ID, _ = r.LastInsertId()
	return nil
}

// LatestVersion returns the most recent version ID, or 0 if none.
func (s *Store) LatestVersion() (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM versions`).Scan(&id)
	return id, err
}

// LoadState loads the complete working set for a version.
func (s *Store) LoadState(versionID int64) (*model.State, error) {
	st := &model.State{}
	err := s.db.QueryRow(`SELECT id,name,created_at FROM versions WHERE id=?`, versionID).
		Scan(&st.Version.ID, &st.Version.Name, &st.Version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("version %d: %w", versionID, err)
	}
	rows, err := s.db.Query(`SELECT id,name,nuclei,tolerances,bias,window FROM experiments WHERE version_id=? ORDER BY id`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e model.Experiment
		var nuclei, tol, bias string
		if err := rows.Scan(&e.ID, &e.Name, &nuclei, &tol, &bias, &e.Window); err != nil {
			return nil, err
		}
		e.VersionID = versionID
		_ = json.Unmarshal([]byte(nuclei), &e.Nuclei)
		_ = json.Unmarshal([]byte(tol), &e.Tolerances)
		_ = json.Unmarshal([]byte(bias), &e.Bias)
		st.Experiments = append(st.Experiments, e)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,experiment_id,label,shifts,intensity,overlap,capacity FROM peaks WHERE version_id=? ORDER BY id`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p model.Peak
		var shifts string
		var ov int
		if err := rows.Scan(&p.ID, &p.ExperimentID, &p.Label, &shifts, &p.Intensity, &ov, &p.Capacity); err != nil {
			return nil, err
		}
		p.VersionID = versionID
		p.Overlap = ov != 0
		_ = json.Unmarshal([]byte(shifts), &p.Shifts)
		st.Peaks = append(st.Peaks, p)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,name,residue,shifts FROM atoms WHERE version_id=? ORDER BY id`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a model.Atom
		var shifts string
		if err := rows.Scan(&a.ID, &a.Name, &a.Residue, &shifts); err != nil {
			return nil, err
		}
		a.VersionID = versionID
		_ = json.Unmarshal([]byte(shifts), &a.Shifts)
		st.Atoms = append(st.Atoms, a)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,atom_a,atom_b,kind,weight,rejected FROM connectivities WHERE version_id=? ORDER BY id`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c model.Connectivity
		var rej int
		if err := rows.Scan(&c.ID, &c.AtomA, &c.AtomB, &c.Kind, &c.Weight, &rej); err != nil {
			return nil, err
		}
		c.VersionID = versionID
		c.Rejected = rej != 0
		st.Connectivities = append(st.Connectivities, c)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,peak_a,peak_b FROM correspondences WHERE version_id=? ORDER BY id`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c model.Correspondence
		if err := rows.Scan(&c.ID, &c.PeakA, &c.PeakB); err != nil {
			return nil, err
		}
		c.VersionID = versionID
		st.Correspondence = append(st.Correspondence, c)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id,peak_id,atom_id FROM locks WHERE version_id=? ORDER BY id`, versionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l model.Lock
		if err := rows.Scan(&l.ID, &l.PeakID, &l.AtomID); err != nil {
			return nil, err
		}
		l.VersionID = versionID
		st.Locks = append(st.Locks, l)
	}
	rows.Close()
	return st, nil
}

func (s *Store) SetLock(versionID, peakID, atomID int64, locked bool) error {
	if locked {
		_, err := s.db.Exec(`INSERT OR IGNORE INTO locks(version_id,peak_id,atom_id) VALUES(?,?,?)`, versionID, peakID, atomID)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM locks WHERE version_id=? AND peak_id=? AND atom_id=?`, versionID, peakID, atomID)
	return err
}

func (s *Store) SetConnectivityRejected(connID int64, rejected bool) error {
	r := 0
	if rejected {
		r = 1
	}
	_, err := s.db.Exec(`UPDATE connectivities SET rejected=? WHERE id=?`, r, connID)
	return err
}

func (s *Store) SetBias(experimentID int64, nucleus string, value float64) error {
	var raw string
	err := s.db.QueryRow(`SELECT bias FROM experiments WHERE id=?`, experimentID).Scan(&raw)
	if err != nil {
		return err
	}
	bias := map[string]float64{}
	_ = json.Unmarshal([]byte(raw), &bias)
	bias[nucleus] = value
	out, _ := json.Marshal(bias)
	_, err = s.db.Exec(`UPDATE experiments SET bias=? WHERE id=?`, string(out), experimentID)
	return err
}

// LogRun appends an entry to the exportable run log.
func (s *Store) LogRun(versionID int64, action string, detail any) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO runs(version_id,action,detail,created_at) VALUES(?,?,?,?)`,
		versionID, action, string(b), now())
	return err
}

// RunLogEntry is one exported run-log row.
type RunLogEntry struct {
	ID        int64           `json:"id"`
	VersionID int64           `json:"version_id"`
	Action    string          `json:"action"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt string          `json:"created_at"`
}

func (s *Store) RunLog() ([]RunLogEntry, error) {
	rows, err := s.db.Query(`SELECT id,version_id,action,detail,created_at FROM runs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunLogEntry
	for rows.Next() {
		var e RunLogEntry
		var detail string
		if err := rows.Scan(&e.ID, &e.VersionID, &e.Action, &detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Reset clears every table so the fixture can be re-imported cleanly.
func (s *Store) Reset() error {
	_, err := s.db.Exec(`
DELETE FROM locks; DELETE FROM correspondences; DELETE FROM connectivities;
DELETE FROM atoms; DELETE FROM peaks; DELETE FROM experiments;
DELETE FROM versions; DELETE FROM runs;
DELETE FROM sqlite_sequence;`)
	return err
}
