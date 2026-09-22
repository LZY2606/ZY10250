// Package store persists the workshop state in SQLite.
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
	s := &Store{db: db}
	return s, s.migrate()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS experiments(
  id INTEGER PRIMARY KEY, name TEXT, dims TEXT, bias TEXT);
CREATE TABLE IF NOT EXISTS peaks(
  id INTEGER PRIMARY KEY, exp_id INTEGER, label TEXT, shifts TEXT,
  intensity REAL, overlap INTEGER, capacity INTEGER);
CREATE TABLE IF NOT EXISTS atoms(id INTEGER PRIMARY KEY, label TEXT);
CREATE TABLE IF NOT EXISTS atom_expected(
  atom_id INTEGER, exp_id INTEGER, shifts TEXT,
  PRIMARY KEY(atom_id, exp_id));
CREATE TABLE IF NOT EXISTS connectivities(
  id INTEGER PRIMARY KEY, from_id INTEGER, to_id INTEGER, kind TEXT, weight REAL);
CREATE TABLE IF NOT EXISTS runs(
  id INTEGER PRIMARY KEY, created_at TEXT, version TEXT, ops TEXT, result TEXT);
`)
	return err
}

// Wipe removes all domain and run data so a fixture can be re-imported.
func (s *Store) Wipe() error {
	_, err := s.db.Exec(`DELETE FROM experiments; DELETE FROM peaks;
DELETE FROM atoms; DELETE FROM atom_expected; DELETE FROM connectivities; DELETE FROM runs;`)
	return err
}

func (s *Store) SaveExperiment(e model.Experiment) error {
	dims, _ := json.Marshal(e.Dims)
	bias, _ := json.Marshal(e.ShiftBias)
	_, err := s.db.Exec(`INSERT INTO experiments(id,name,dims,bias) VALUES(?,?,?,?)`,
		e.ID, e.Name, string(dims), string(bias))
	return err
}

func (s *Store) SavePeak(p model.Peak) error {
	sh, _ := json.Marshal(p.Shifts)
	ov := 0
	if p.Overlap {
		ov = 1
	}
	_, err := s.db.Exec(`INSERT INTO peaks(id,exp_id,label,shifts,intensity,overlap,capacity)
VALUES(?,?,?,?,?,?,?)`, p.ID, p.ExpID, p.Label, string(sh), p.Intensity, ov, p.Capacity)
	return err
}

func (s *Store) SaveAtom(a model.AtomSite) error {
	_, err := s.db.Exec(`INSERT INTO atoms(id,label) VALUES(?,?)`, a.ID, a.Label)
	return err
}

func (s *Store) SaveExpected(e model.AtomExpected) error {
	sh, _ := json.Marshal(e.Shifts)
	_, err := s.db.Exec(`INSERT INTO atom_expected(atom_id,exp_id,shifts) VALUES(?,?,?)`,
		e.AtomID, e.ExpID, string(sh))
	return err
}

func (s *Store) SaveConnectivity(c model.Connectivity) error {
	_, err := s.db.Exec(`INSERT INTO connectivities(id,from_id,to_id,kind,weight) VALUES(?,?,?,?,?)`,
		c.ID, c.FromID, c.ToID, c.Kind, c.Weight)
	return err
}

// Dataset is the full solver input loaded from the database.
type Dataset struct {
	Experiments []model.Experiment
	Peaks       []model.Peak
	Atoms       []model.AtomSite
	Expected    []model.AtomExpected
	Conns       []model.Connectivity
}

func (s *Store) Load() (*Dataset, error) {
	d := &Dataset{}
	rows, err := s.db.Query(`SELECT id,name,dims,bias FROM experiments ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e model.Experiment
		var dims, bias string
		if err := rows.Scan(&e.ID, &e.Name, &dims, &bias); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(dims), &e.Dims)
		json.Unmarshal([]byte(bias), &e.ShiftBias)
		d.Experiments = append(d.Experiments, e)
	}
	rows2, err := s.db.Query(`SELECT id,exp_id,label,shifts,intensity,overlap,capacity FROM peaks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var p model.Peak
		var sh string
		var ov int
		if err := rows2.Scan(&p.ID, &p.ExpID, &p.Label, &sh, &p.Intensity, &ov, &p.Capacity); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(sh), &p.Shifts)
		p.Overlap = ov != 0
		d.Peaks = append(d.Peaks, p)
	}
	rows3, err := s.db.Query(`SELECT id,label FROM atoms ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	for rows3.Next() {
		var a model.AtomSite
		if err := rows3.Scan(&a.ID, &a.Label); err != nil {
			return nil, err
		}
		d.Atoms = append(d.Atoms, a)
	}
	rows4, err := s.db.Query(`SELECT atom_id,exp_id,shifts FROM atom_expected ORDER BY atom_id,exp_id`)
	if err != nil {
		return nil, err
	}
	defer rows4.Close()
	for rows4.Next() {
		var e model.AtomExpected
		var sh string
		if err := rows4.Scan(&e.AtomID, &e.ExpID, &sh); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(sh), &e.Shifts)
		d.Expected = append(d.Expected, e)
	}
	rows5, err := s.db.Query(`SELECT id,from_id,to_id,kind,weight FROM connectivities ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows5.Close()
	for rows5.Next() {
		var c model.Connectivity
		if err := rows5.Scan(&c.ID, &c.FromID, &c.ToID, &c.Kind, &c.Weight); err != nil {
			return nil, err
		}
		d.Conns = append(d.Conns, c)
	}
	return d, rows.Err()
}

// RunRecord is one logged solve run.
type RunRecord struct {
	ID        int64
	CreatedAt string
	Version   string
	Ops       model.Ops
	Result    model.SolveResult
}

func (s *Store) SaveRun(version string, ops model.Ops, res model.SolveResult) error {
	o, _ := json.Marshal(ops)
	r, _ := json.Marshal(res)
	_, err := s.db.Exec(`INSERT INTO runs(created_at,version,ops,result) VALUES(?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), version, string(o), string(r))
	return err
}

func (s *Store) ListRuns() ([]RunRecord, error) {
	rows, err := s.db.Query(`SELECT id,created_at,version,ops,result FROM runs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunRecord
	for rows.Next() {
		var rec RunRecord
		var o, r string
		if err := rows.Scan(&rec.ID, &rec.CreatedAt, &rec.Version, &o, &r); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(o), &rec.Ops)
		json.Unmarshal([]byte(r), &rec.Result)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) String() string {
	return fmt.Sprintf("store(%p)", s.db)
}
