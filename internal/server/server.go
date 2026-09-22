// Package server exposes the workshop HTTP API and the SVG operation page.
package server

import (
	"encoding/json"
	"net/http"

	"resonance-workshop/internal/fixture"
	"resonance-workshop/internal/solver"
	"resonance-workshop/internal/store"
)

type Server struct {
	st *store.Store
}

func New(st *store.Store) *Server { return &Server{st: st} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePage)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/solve", s.handleSolve)
	mux.HandleFunc("/api/lock", s.handleLock)
	mux.HandleFunc("/api/connectivity", s.handleConnectivity)
	mux.HandleFunc("/api/bias", s.handleBias)
	mux.HandleFunc("/api/reset", s.handleReset)
	mux.HandleFunc("/api/export", s.handleExport)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (s *Server) currentVersion() (int64, error) {
	vid, err := s.st.LatestVersion()
	if err != nil {
		return 0, err
	}
	if vid == 0 {
		vid, err = fixture.Seed(s.st)
		if err != nil {
			return 0, err
		}
	}
	return vid, nil
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	vid, err := s.currentVersion()
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.st.LoadState(vid)
	if err != nil {
		writeErr(w, err)
		return
	}
	res := solver.Solve(*st)
	writeJSON(w, http.StatusOK, map[string]any{"state": st, "result": res})
}

func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errMethod)
		return
	}
	vid, err := s.currentVersion()
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.st.LoadState(vid)
	if err != nil {
		writeErr(w, err)
		return
	}
	res := solver.Solve(*st)
	// Every conclusion is pinned to the peak-list version.
	_ = s.st.LogRun(vid, "solve", map[string]any{
		"version_id":       vid,
		"solutions":        len(res.Solutions),
		"unassigned_peaks": res.Unassigned,
		"conflict_locks":   res.ConflictSet,
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": st, "result": res})
}

var errMethod = errString("method not allowed")

type errString string

func (e errString) Error() string { return string(e) }

func decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errMethod)
		return
	}
	var req struct {
		PeakID int64 `json:"peak_id"`
		AtomID int64 `json:"atom_id"`
		Locked bool  `json:"locked"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	vid, err := s.currentVersion()
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.st.SetLock(vid, req.PeakID, req.AtomID, req.Locked); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.st.LogRun(vid, "lock", req)
	s.respondState(w, vid)
}

func (s *Server) handleConnectivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errMethod)
		return
	}
	var req struct {
		ID       int64 `json:"id"`
		Rejected bool  `json:"rejected"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	vid, err := s.currentVersion()
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.st.SetConnectivityRejected(req.ID, req.Rejected); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.st.LogRun(vid, "connectivity", req)
	s.respondState(w, vid)
}

func (s *Server) handleBias(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errMethod)
		return
	}
	var req struct {
		ExperimentID int64   `json:"experiment_id"`
		Nucleus      string  `json:"nucleus"`
		Value        float64 `json:"value"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	vid, err := s.currentVersion()
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.st.SetBias(req.ExperimentID, req.Nucleus, req.Value); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.st.LogRun(vid, "bias", req)
	s.respondState(w, vid)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errMethod)
		return
	}
	if err := s.st.Reset(); err != nil {
		writeErr(w, err)
		return
	}
	vid, err := fixture.Seed(s.st)
	if err != nil {
		writeErr(w, err)
		return
	}
	_ = s.st.LogRun(vid, "reset+import", map[string]any{"version_id": vid})
	s.respondState(w, vid)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	log, err := s.st.RunLog()
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=run-log.json")
	_ = json.NewEncoder(w).Encode(log)
}

func (s *Server) respondState(w http.ResponseWriter, vid int64) {
	st, err := s.st.LoadState(vid)
	if err != nil {
		writeErr(w, err)
		return
	}
	res := solver.Solve(*st)
	writeJSON(w, http.StatusOK, map[string]any{"state": st, "result": res})
}
