package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/embedder"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type fakeEmbedder struct {
	fail bool
	vecs map[string][]float32
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.fail {
		return nil, errors.New("ollama down")
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec, ok := f.vecs[text]
		if !ok {
			return nil, fmt.Errorf("unexpected embed input %q", text)
		}
		out[i] = vec
	}
	return out, nil
}

func docTextOf(t *testing.T, s *Store, slug string) string {
	t.Helper()
	view, err := s.Get(slug)
	if err != nil {
		t.Fatalf("Get(%s): %v", slug, err)
	}
	return embedder.DocText(view.Note.Frontmatter.Description, view.Note.Body)
}

func enableTestEmbeddings(t *testing.T, s *Store, fake embedder.EmbedClient) *embedder.Worker {
	t.Helper()
	cache := embedder.OpenCache(filepath.Join(t.TempDir(), "embeddings.gob"), "test-model")
	return s.EnableEmbeddings(cache, fake, "", nil)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestHybridSearchFindsVectorOnlyMatch(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Plan for the birth."}, Body: "Details.\n",
	})
	mustSave(t, s, &vault.Note{
		Slug: "hypnobirthing", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Breathing techniques for labour."}, Body: "Relax.\n",
	})

	fake := &fakeEmbedder{vecs: map[string][]float32{
		docTextOf(t, s, "root"):          {0, 0},
		docTextOf(t, s, "birth-plan"):    {0, 1},
		docTextOf(t, s, "hypnobirthing"): {1, 0},
		"birth":                          {1, 0},
	}}
	w := enableTestEmbeddings(t, s, fake)
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	hits, err := s.Search(t.Context(), "birth", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) < 2 || hits[0].Slug != "birth-plan" || hits[1].Slug != "hypnobirthing" {
		t.Fatalf("hybrid hits = %+v, want birth-plan (both sides) then hypnobirthing (vector only)", hits)
	}
	if hits[1].Description == "" || hits[1].Folder == "" {
		t.Errorf("vector-only hit missing metadata: %+v", hits[1])
	}
}

func TestSearchFallsBackToFTSWhenEmbedFails(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Plan for the birth."}, Body: "Details.\n",
	})
	fake := &fakeEmbedder{vecs: map[string][]float32{
		docTextOf(t, s, "root"):       {0, 0},
		docTextOf(t, s, "birth-plan"): {0, 1},
	}}
	w := enableTestEmbeddings(t, s, fake)
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	fake.fail = true
	hits, err := s.Search(t.Context(), "birth", 10)
	if err != nil {
		t.Fatalf("Search with failing embedder: %v", err)
	}
	if len(hits) != 1 || hits[0].Slug != "birth-plan" {
		t.Errorf("fallback hits = %+v, want the FTS result", hits)
	}
}

func TestSimilar(t *testing.T) {
	s := testStore(t)
	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Plan for the birth."}, Body: "Details.\n",
	})
	mustSave(t, s, &vault.Note{
		Slug: "hypnobirthing", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Breathing techniques for labour."}, Body: "Relax.\n",
	})

	if _, err := s.Similar("birth-plan", 10); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Similar with embeddings disabled: err = %v, want ErrUnavailable", err)
	}

	fake := &fakeEmbedder{vecs: map[string][]float32{
		docTextOf(t, s, "root"):          {0, 0},
		docTextOf(t, s, "birth-plan"):    {1, 0},
		docTextOf(t, s, "hypnobirthing"): {1, 0},
	}}
	w := enableTestEmbeddings(t, s, fake)

	if _, err := s.Similar("birth-plan", 10); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Similar before first sync: err = %v, want ErrUnavailable", err)
	}

	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	hits, err := s.Similar("birth-plan", 10)
	if err != nil {
		t.Fatalf("Similar: %v", err)
	}
	if len(hits) == 0 || hits[0].Slug != "hypnobirthing" {
		t.Fatalf("Similar = %+v, want hypnobirthing first", hits)
	}
	for _, h := range hits {
		if h.Slug == "birth-plan" {
			t.Error("Similar returned the note itself")
		}
	}

	if _, err := s.Similar("nope", 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("Similar(nope): err = %v, want ErrNotFound", err)
	}
}

func TestSaveWakesEmbedWorker(t *testing.T) {
	s := testStore(t)
	fake := &fakeEmbedder{vecs: map[string][]float32{
		docTextOf(t, s, "root"): {0, 0},
		embedder.DocText("Plan for the birth.", "Details.\n"): {0, 1},
	}}
	w := enableTestEmbeddings(t, s, fake)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	embedded := func() int {
		st, err := s.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		return st.Embed.Embedded
	}
	waitFor(t, "startup sync", func() bool { return embedded() == 1 })

	mustSave(t, s, &vault.Note{
		Slug: "birth-plan", Folder: vault.FolderNotes,
		Frontmatter: vault.Frontmatter{Description: "Plan for the birth."}, Body: "Details.\n",
	})
	waitFor(t, "wake-triggered sync", func() bool { return embedded() == 2 })

	st, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Embed.Enabled || st.Embed.Backlog != 0 || st.Embed.LastError != "" {
		t.Errorf("Embed status = %+v", st.Embed)
	}
}
