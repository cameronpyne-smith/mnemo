package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/agent"
	"github.com/cameronpyne-smith/mnemo/internal/api"
	"github.com/cameronpyne-smith/mnemo/internal/gitsync"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type Server struct {
	store  *store.Store
	worker *agent.Worker
	token  string
	sync   *gitsync.Syncer
}

func New(st *store.Store, worker *agent.Worker, token string, mcp http.Handler, sync *gitsync.Syncer) http.Handler {
	s := &Server{store: st, worker: worker, token: token, sync: sync}
	mux := http.NewServeMux()
	if mcp != nil {
		mux.Handle("/mcp", mcp)
	}
	mux.HandleFunc("GET /index", s.handleIndex)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /notes/{slug}", s.handleGet)
	mux.HandleFunc("GET /notes/{slug}/links", s.handleLinks)
	mux.HandleFunc("POST /notes/{slug}/edit", s.handleEdit)
	mux.HandleFunc("POST /notes/{slug}/rename", s.handleRename)
	mux.HandleFunc("POST /capture", s.handleCapture)
	mux.HandleFunc("GET /status", s.handleStatus)
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeError(w, http.StatusUnauthorized, errors.New("invalid or missing bearer token"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	root, hubs, err := s.store.Hubs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := api.IndexResponse{Root: toAPINote(root, nil), Hubs: make([]api.Note, 0, len(hubs))}
	for _, h := range hubs {
		resp.Hubs = append(resp.Hubs, toAPINote(h, nil))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := s.store.Search(r.URL.Query().Get("q"), limit)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, api.FromHits(hits))
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	view, err := s.store.Get(r.PathValue("slug"))
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, toAPINote(view.Note, view))
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	view, err := s.store.Get(r.PathValue("slug"))
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, api.LinksResponse{
		Slug: view.Note.Slug, Links: view.Links, Backlinks: view.Backlinks,
	})
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	var req api.EditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slug := r.PathValue("slug")
	err := s.store.EditNote(store.ActorAPI, slug, store.Edit{
		Description: req.Description, Tags: req.Tags, Body: req.Body, Append: req.Append,
	})
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	view, err := s.store.Get(slug)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, toAPINote(view.Note, view))
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	var req api.RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.Rename(store.ActorAPI, r.PathValue("slug"), req.To); err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	view, err := s.store.Get(req.To)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, toAPINote(view.Note, view))
}

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	var req api.CaptureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	slug, err := s.store.Capture(store.ActorAPI, req.Content, req.Source)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if s.worker != nil {
		s.worker.Wake()
	}
	writeJSON(w, http.StatusAccepted, api.CaptureResponse{Slug: slug})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Status()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := api.StatusResponse{
		Notes: st.Notes, Hubs: st.Hubs, Inbox: st.Inbox, Archived: st.Archived,
	}
	if s.worker != nil {
		processed, failed, inflight := s.worker.Stats()
		resp.Filing = api.FilingStatus{Enabled: true, Processed: processed, Failed: failed, InFlight: inflight}
	}
	if s.sync != nil {
		gs := s.sync.Stats(r.Context())
		resp.Git = api.GitStatus{Enabled: true, Commits: gs.Commits, LastError: gs.LastError}
		for _, rm := range gs.Remotes {
			lastPush := ""
			if !rm.LastPush.IsZero() {
				lastPush = rm.LastPush.Format(time.RFC3339)
			}
			resp.Git.Remotes = append(resp.Git.Remotes, api.GitRemoteStatus{
				Name: rm.Name, Lag: rm.Lag, LastPush: lastPush, LastError: rm.LastError,
			})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func toAPINote(n *vault.Note, view *store.NoteView) api.Note {
	if view == nil {
		return api.FromNote(n, nil, nil)
	}
	return api.FromNote(n, view.Links, view.Backlinks)
}

func statusFor(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, store.ErrInvalid) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, api.ErrorResponse{Error: err.Error()})
}
