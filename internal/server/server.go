// Package server exposes the workshop over HTTP with an SVG operation page.
package server

import (
	"encoding/json"
	"net/http"

	"resonance-workshop/internal/model"
	"resonance-workshop/internal/solver"
	"resonance-workshop/internal/store"
)

type Server struct {
	st  *store.Store
	mux *http.ServeMux
}

func New(st *store.Store) *Server {
	s := &Server{st: st, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /api/state", s.handleState)
	s.mux.HandleFunc("POST /api/solve", s.handleSolve)
	s.mux.HandleFunc("POST /api/reset", s.handleReset)
	s.mux.HandleFunc("GET /api/runs", s.handleRuns)
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	d, err := s.st.Load()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	res := solver.Solve(d.Experiments, d.Peaks, d.Atoms, d.Expected, d.Conns, model.Ops{})
	cands := solver.Candidates(d.Experiments, d.Peaks, d.Expected, nil)
	writeJSON(w, 200, map[string]any{
		"dataset":    d,
		"candidates": cands,
		"result":     res,
	})
}

func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	var ops model.Ops
	if err := json.NewDecoder(r.Body).Decode(&ops); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	d, err := s.st.Load()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	res := solver.Solve(d.Experiments, d.Peaks, d.Atoms, d.Expected, d.Conns, ops)
	cands := solver.Candidates(d.Experiments, d.Peaks, d.Expected, ops.BiasOverride)
	if err := s.st.SaveRun(res.Version, ops, res); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"result": res, "candidates": cands})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := s.st.Wipe(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := s.st.Seed(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "reseeded"})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.st.ListRuns()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"runs": runs})
}
