package embedder

import (
	"context"
	"errors"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type fakeEmbedder struct {
	mu    sync.Mutex
	calls [][]string
	fail  bool
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("ollama down")
	}
	f.calls = append(f.calls, texts)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = fakeVec(t)
	}
	return out, nil
}

func (f *fakeEmbedder) inputs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []string
	for _, call := range f.calls {
		all = append(all, call...)
	}
	return all
}

func (f *fakeEmbedder) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func fakeVec(text string) []float32 {
	h := fnv.New32a()
	h.Write([]byte(text))
	return []float32{float32(h.Sum32()%1000) / 1000, 1}
}

type workerFixture struct {
	vault     *vault.Vault
	idx       *index.Index
	cache     *Cache
	fake      *fakeEmbedder
	worker    *Worker
	cachePath string
}

func newWorkerFixture(t *testing.T) *workerFixture {
	t.Helper()
	root := t.TempDir()
	folders := []string{
		vault.FolderNotes, vault.FolderHubs, vault.FolderInbox,
		vault.FolderAttachments, vault.FolderArchive,
	}
	for _, folder := range folders {
		if err := os.MkdirAll(filepath.Join(root, folder), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	idx, err := index.New()
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	path := filepath.Join(root, ".mnemo", "embeddings.gob")
	cache := OpenCache(path, "test-model")
	fake := &fakeEmbedder{}
	return &workerFixture{
		vault:     v,
		idx:       idx,
		cache:     cache,
		fake:      fake,
		worker:    NewWorker(v, idx, cache, fake, nil),
		cachePath: path,
	}
}

func writeNote(t *testing.T, v *vault.Vault, folder, slug, description, body string) {
	t.Helper()
	n := &vault.Note{
		Slug:        slug,
		Folder:      folder,
		Frontmatter: vault.Frontmatter{Description: description},
		Body:        body,
	}
	if err := v.Write(n); err != nil {
		t.Fatalf("Write(%s/%s): %v", folder, slug, err)
	}
}

func noteText(t *testing.T, v *vault.Vault, folder, slug string) string {
	t.Helper()
	n, err := v.Read(folder, slug)
	if err != nil {
		t.Fatalf("Read(%s/%s): %v", folder, slug, err)
	}
	return DocText(n.Frontmatter.Description, n.Body)
}

func vectorSlugs(idx *index.Index) []string {
	var slugs []string
	for _, dv := range idx.Vectors() {
		slugs = append(slugs, dv.Slug)
	}
	return slugs
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

func TestSyncEmbedsNotesAndHubsOnly(t *testing.T) {
	fx := newWorkerFixture(t)
	writeNote(t, fx.vault, vault.FolderNotes, "birth-plan", "Plan for the hospital birth.", "Midwife notes.\n")
	writeNote(t, fx.vault, vault.FolderNotes, "sourdough-starter", "Feeding schedule.", "Feed daily at 8am.\n")
	writeNote(t, fx.vault, vault.FolderHubs, "health", "Hub for health notes.", "- [[birth-plan]]\n")
	writeNote(t, fx.vault, vault.FolderInbox, "dump-1", "", "raw capture, not yet filed\n")

	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	wantSlugs := []string{"birth-plan", "health", "sourdough-starter"}
	if got := vectorSlugs(fx.idx); !reflect.DeepEqual(got, wantSlugs) {
		t.Fatalf("vector slugs = %v, want %v", got, wantSlugs)
	}
	for _, dv := range fx.idx.Vectors() {
		folder := vault.FolderNotes
		if dv.Slug == "health" {
			folder = vault.FolderHubs
		}
		want := fakeVec(noteText(t, fx.vault, folder, dv.Slug))
		if !reflect.DeepEqual(dv.Vec, want) {
			t.Errorf("vector for %s = %v, want %v", dv.Slug, dv.Vec, want)
		}
	}
	if got := fx.worker.Backlog(); got != 0 {
		t.Errorf("Backlog = %d, want 0", got)
	}
	if reopened := OpenCache(fx.cachePath, "test-model"); reopened.Len() != 3 {
		t.Errorf("cache entries after Sync = %d, want 3", reopened.Len())
	}
}

func TestSyncUsesCacheWithoutEmbedding(t *testing.T) {
	fx := newWorkerFixture(t)
	writeNote(t, fx.vault, vault.FolderNotes, "birth-plan", "Plan for the hospital birth.", "Midwife notes.\n")
	cached := []float32{0.5, 0.5}
	fx.cache.Put(TextHash(noteText(t, fx.vault, vault.FolderNotes, "birth-plan")), cached)

	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := fx.fake.inputs(); len(got) != 0 {
		t.Errorf("embedded %v, want no embed calls on full cache hit", got)
	}
	vecs := fx.idx.Vectors()
	if len(vecs) != 1 || !reflect.DeepEqual(vecs[0].Vec, cached) {
		t.Errorf("Vectors = %v, want cached vector applied to index", vecs)
	}
}

func TestSyncReembedsOnlyChangedNote(t *testing.T) {
	fx := newWorkerFixture(t)
	writeNote(t, fx.vault, vault.FolderNotes, "one", "First.", "alpha\n")
	writeNote(t, fx.vault, vault.FolderNotes, "two", "Second.", "beta\n")
	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	fx.fake.reset()

	writeNote(t, fx.vault, vault.FolderNotes, "two", "Second.", "beta edited\n")
	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	want := []string{noteText(t, fx.vault, vault.FolderNotes, "two")}
	if got := fx.fake.inputs(); !reflect.DeepEqual(got, want) {
		t.Errorf("second sync embedded %v, want only the edited note", got)
	}
	for _, dv := range fx.idx.Vectors() {
		if dv.Slug == "two" && !reflect.DeepEqual(dv.Vec, fakeVec(want[0])) {
			t.Errorf("vector for edited note not replaced")
		}
	}
}

func TestSyncDropsDeletedNote(t *testing.T) {
	fx := newWorkerFixture(t)
	writeNote(t, fx.vault, vault.FolderNotes, "keep", "Kept.", "stays\n")
	writeNote(t, fx.vault, vault.FolderNotes, "gone", "Doomed.", "goes\n")
	goneHash := TextHash(noteText(t, fx.vault, vault.FolderNotes, "gone"))
	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	if err := os.Remove(fx.vault.NotePath(vault.FolderNotes, "gone")); err != nil {
		t.Fatal(err)
	}
	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if got := vectorSlugs(fx.idx); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Errorf("vector slugs after delete = %v, want [keep]", got)
	}
	if _, ok := OpenCache(fx.cachePath, "test-model").Get(goneHash); ok {
		t.Error("deleted note's embedding survived in saved cache")
	}
}

func TestSyncEmbedFailureFallsBackAndRecovers(t *testing.T) {
	fx := newWorkerFixture(t)
	fx.fake.fail = true
	writeNote(t, fx.vault, vault.FolderNotes, "one", "First.", "alpha\n")
	writeNote(t, fx.vault, vault.FolderNotes, "two", "Second.", "beta\n")

	if err := fx.worker.Sync(context.Background()); err == nil {
		t.Fatal("Sync with failing embedder returned nil error")
	}
	if got := fx.worker.Backlog(); got != 2 {
		t.Errorf("Backlog after failure = %d, want 2", got)
	}
	if fx.worker.LastError() == "" {
		t.Error("LastError empty after failed sync")
	}
	if got := fx.idx.Vectors(); len(got) != 0 {
		t.Errorf("Vectors after failed sync = %v, want none", got)
	}

	fx.fake.fail = false
	if err := fx.worker.Sync(context.Background()); err != nil {
		t.Fatalf("recovery Sync: %v", err)
	}
	if got := fx.worker.Backlog(); got != 0 {
		t.Errorf("Backlog after recovery = %d, want 0", got)
	}
	if fx.worker.LastError() != "" {
		t.Errorf("LastError after recovery = %q, want empty", fx.worker.LastError())
	}
	if got := vectorSlugs(fx.idx); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("vector slugs after recovery = %v", got)
	}
}

func TestRunSyncsOnStartWakeAndShutdown(t *testing.T) {
	fx := newWorkerFixture(t)
	writeNote(t, fx.vault, vault.FolderNotes, "first", "First.", "alpha\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		fx.worker.Run(ctx)
		close(done)
	}()

	waitFor(t, "startup sync", func() bool { return len(fx.idx.Vectors()) == 1 })

	writeNote(t, fx.vault, vault.FolderNotes, "second", "Second.", "beta\n")
	fx.worker.Wake()
	waitFor(t, "wake sync", func() bool { return len(fx.idx.Vectors()) == 2 })

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after ctx cancel")
	}
}
