package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cameronpyne-smith/mnemo/internal/api"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func testServer(t *testing.T, token string) (*httptest.Server, *store.Store) {
	t.Helper()
	root := t.TempDir()
	if _, err := vault.Init(root); err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	srv := httptest.NewServer(New(st, nil, token))
	t.Cleanup(srv.Close)
	return srv, st
}

func request(t *testing.T, method, url, token string, body, out any) int {
	t.Helper()
	var reqBody *bytes.Reader = bytes.NewReader(nil)
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp.StatusCode
}

func TestCaptureSearchGetFlow(t *testing.T) {
	srv, st := testServer(t, "")

	var captured api.CaptureResponse
	code := request(t, http.MethodPost, srv.URL+"/capture", "", api.CaptureRequest{Content: "remember the milk", Source: "test"}, &captured)
	if code != http.StatusAccepted || !strings.HasPrefix(captured.Slug, "capture-") {
		t.Fatalf("capture: code=%d resp=%+v", code, captured)
	}

	if err := st.Save(&vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Plan for the birth."},
		Body:        "Details here.\n",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var search api.SearchResponse
	code = request(t, http.MethodGet, srv.URL+"/search?q=birth", "", nil, &search)
	if code != http.StatusOK || len(search.Results) != 1 || search.Results[0].Slug != "birth-plan" {
		t.Fatalf("search: code=%d resp=%+v", code, search)
	}

	var note api.Note
	code = request(t, http.MethodGet, srv.URL+"/notes/birth-plan", "", nil, &note)
	if code != http.StatusOK || note.Body != "Details here.\n" {
		t.Fatalf("get: code=%d resp=%+v", code, note)
	}

	code = request(t, http.MethodGet, srv.URL+"/notes/nope", "", nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("get missing: code=%d, want 404", code)
	}
}

func TestEditRenameStatusFlow(t *testing.T) {
	srv, st := testServer(t, "")
	if err := st.Save(&vault.Note{
		Slug: "n", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "old"}, Body: "body\n",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	desc := "new"
	var edited api.Note
	code := request(t, http.MethodPost, srv.URL+"/notes/n/edit", "", api.EditRequest{Description: &desc}, &edited)
	if code != http.StatusOK || edited.Description != "new" {
		t.Fatalf("edit: code=%d resp=%+v", code, edited)
	}

	var renamed api.Note
	code = request(t, http.MethodPost, srv.URL+"/notes/n/rename", "", api.RenameRequest{To: "m"}, &renamed)
	if code != http.StatusOK || renamed.Slug != "m" {
		t.Fatalf("rename: code=%d resp=%+v", code, renamed)
	}

	var status api.StatusResponse
	code = request(t, http.MethodGet, srv.URL+"/status", "", nil, &status)
	if code != http.StatusOK || status.Notes != 1 || status.Filing.Enabled {
		t.Fatalf("status: code=%d resp=%+v", code, status)
	}
}

func TestIndexEndpoint(t *testing.T) {
	srv, st := testServer(t, "")
	if err := st.Save(&vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "The plan."}, Body: "p\n",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := st.AddToHub("health", "birth-plan", "the plan"); err != nil {
		t.Fatalf("AddToHub: %v", err)
	}

	var idx api.IndexResponse
	code := request(t, http.MethodGet, srv.URL+"/index", "", nil, &idx)
	if code != http.StatusOK {
		t.Fatalf("index: code=%d", code)
	}
	if idx.Root.Slug != "root" || len(idx.Hubs) != 1 || idx.Hubs[0].Slug != "health" {
		t.Fatalf("index resp = %+v", idx)
	}
}

func TestAuth(t *testing.T) {
	srv, _ := testServer(t, "secret")
	if code := request(t, http.MethodGet, srv.URL+"/status", "", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("no token: code=%d, want 401", code)
	}
	if code := request(t, http.MethodGet, srv.URL+"/status", "wrong", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("wrong token: code=%d, want 401", code)
	}
	if code := request(t, http.MethodGet, srv.URL+"/status", "secret", nil, nil); code != http.StatusOK {
		t.Errorf("right token: code=%d, want 200", code)
	}
}
