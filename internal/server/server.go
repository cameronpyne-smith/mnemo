package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cameronpyne-smith/mnemo/internal/agent"
	"github.com/cameronpyne-smith/mnemo/internal/api"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type Server struct {
	store  *store.Store
	worker *agent.Worker
	token  string
}

func New(st *store.Store, worker *agent.Worker, token string) http.Handler {
	s := &Server{store: st, worker: worker, token: token}
	mux := http.NewServeMux()
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
	q := r.URL.Query().Get("q")
	if strings.TrimSpace(q) == "" {
		writeError(w, http.StatusBadRequest, errors.New("q parameter is required"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := s.store.Search(q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := api.SearchResponse{Results: make([]api.SearchResult, 0, len(hits))}
	for _, h := range hits {
		resp.Results = append(resp.Results, api.SearchResult{
			Slug: h.Slug, Folder: h.Folder, Description: h.Description, Score: h.Score,
		})
	}
	writeJSON(w, http.StatusOK, resp)
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
	err := s.store.EditNote(slug, store.Edit{
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
	if err := s.store.Rename(r.PathValue("slug"), req.To); err != nil {
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
	slug, err := s.store.Capture(req.Content, req.Source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	writeJSON(w, http.StatusOK, resp)
}

func toAPINote(n *vault.Note, view *store.NoteView) api.Note {
	out := api.Note{
		Slug:        n.Slug,
		Folder:      n.Folder,
		Description: n.Frontmatter.Description,
		Tags:        n.Frontmatter.Tags,
		Type:        n.Type(),
		Created:     n.Frontmatter.Created,
		Updated:     n.Frontmatter.Updated,
		Body:        n.Body,
	}
	if view != nil {
		out.Links = view.Links
		out.Backlinks = view.Backlinks
	}
	return out
}

func statusFor(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
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
