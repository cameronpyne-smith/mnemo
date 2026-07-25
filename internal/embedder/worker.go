package embedder

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

const (
	syncInterval = 5 * time.Minute
	embedBatch   = 32
)

// EmbedClient is the worker's one seam to the model: texts in, one
// L2-normalized vector per text out, in the same order. The ollama adapter is
// the only real implementation; tests substitute a fake.
type EmbedClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Worker keeps the index's vectors in sync with the markdown files by
// reconciling rather than queueing: each pass lists notes/ and hubs/, embeds
// whatever the cache cannot supply, and drops vectors for files that are
// gone. Writers never block on it — they just Wake it — and while ollama is
// unreachable the daemon keeps serving FTS-only results.
type Worker struct {
	vault  *vault.Vault
	idx    *index.Index
	cache  *Cache
	client EmbedClient
	log    *slog.Logger
	wake   chan struct{}

	mu      sync.Mutex
	backlog int
	lastErr string
}

// NewWorker wires a worker; a nil log means silent, matching gitsync.
func NewWorker(v *vault.Vault, idx *index.Index, cache *Cache, client EmbedClient, log *slog.Logger) *Worker {
	return &Worker{
		vault:  v,
		idx:    idx,
		cache:  cache,
		client: client,
		log:    log,
		wake:   make(chan struct{}, 1),
	}
}

// Wake nudges the worker to reconcile soon. Never blocks: a wake while one is
// already pending is a no-op, so writers can call it on every save.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Backlog reports how many notes currently lack a vector — nonzero means
// searches fall back to FTS-only for those notes.
func (w *Worker) Backlog() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.backlog
}

// LastError reports the most recent sync failure, empty after a clean pass.
func (w *Worker) LastError() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}

// Sync reconciles once: lists notes/ and hubs/, splits each note's DocText
// into chunks, hashes each chunk, embeds whatever the cache cannot supply in
// batches of at most embedBatch, and removes index vectors whose files are
// gone. A note reaches the index only once every chunk has a vector — a
// half-embedded note keeps serving its previous vectors and counts toward
// Backlog. Ends by saving the cache pruned to the live hashes. On an embed
// error the cache hits are still applied and the cache still saved; the
// error is returned.
func (w *Worker) Sync(ctx context.Context) error {
	notes, err := w.vault.List(vault.FolderNotes, vault.FolderHubs)
	if err != nil {
		err = fmt.Errorf("listing vault: %w", err)
		w.mu.Lock()
		w.lastErr = err.Error()
		w.mu.Unlock()
		return err
	}
	liveSlugs := make(map[string]bool)
	liveHashes := make(map[string]bool)

	type embedJob struct {
		text, hash string
	}
	type pendingNote struct {
		slug   string
		hashes []string
	}

	var jobs []embedJob
	var pending []pendingNote
	for _, note := range notes {
		liveSlugs[note.Slug] = true
		chunks := Chunks(DocText(note.Frontmatter.Description, note.Body))
		hashes := make([]string, len(chunks))
		vectors := make([][]float32, len(chunks))
		complete := true
		for i, chunk := range chunks {
			hash := TextHash(chunk)
			hashes[i] = hash
			liveHashes[hash] = true
			vector, found := w.cache.Get(hash)
			if found {
				vectors[i] = vector
			} else {
				complete = false
				jobs = append(jobs, embedJob{text: chunk, hash: hash})
			}
		}
		if complete {
			w.idx.SetVectors(note.Slug, vectors)
		} else {
			pending = append(pending, pendingNote{slug: note.Slug, hashes: hashes})
		}
	}

	if len(pending) > 0 && w.log != nil {
		w.log.Info("embedding notes", "notes", len(pending), "chunks", len(jobs))
	}

	var embedErr error
	for batch := range slices.Chunk(jobs, embedBatch) {
		texts := make([]string, len(batch))
		for i, job := range batch {
			texts[i] = job.text
		}
		vectors, err := w.client.Embed(ctx, texts)
		if err != nil {
			embedErr = fmt.Errorf("embedding notes: %w", err)
			break
		}
		for i, vector := range vectors {
			w.cache.Put(batch[i].hash, vector)
		}
	}

	backlog := 0
	for _, note := range pending {
		vectors := make([][]float32, len(note.hashes))
		complete := true
		for i, hash := range note.hashes {
			vector, found := w.cache.Get(hash)
			if !found {
				complete = false
				break
			}
			vectors[i] = vector
		}
		if complete {
			w.idx.SetVectors(note.slug, vectors)
		} else {
			backlog++
		}
	}

	for _, doc := range w.idx.Vectors() {
		if !liveSlugs[doc.Slug] {
			w.idx.RemoveVectors(doc.Slug)
		}
	}

	saveErr := w.cache.Save(liveHashes)

	syncErr := embedErr
	if syncErr == nil {
		syncErr = saveErr
	}

	w.mu.Lock()
	w.backlog = backlog
	if syncErr != nil {
		w.lastErr = syncErr.Error()
	} else {
		w.lastErr = ""
	}
	w.mu.Unlock()

	return syncErr
}

// Run syncs once at startup, then again on every Wake and every syncInterval,
// until ctx ends. Sync failures are logged and surfaced via Backlog and
// LastError, never fatal.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		if err := w.Sync(ctx); err != nil && w.log != nil {
			w.log.Error("embedder sync failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
		}
	}
}
