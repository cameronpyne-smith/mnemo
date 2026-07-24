package embedder

import (
	"context"
	"log/slog"
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
}

// Backlog reports how many notes currently lack a vector — nonzero means
// searches fall back to FTS-only for those notes.
func (w *Worker) Backlog() int {
	return 0
}

// LastError reports the most recent sync failure, empty after a clean pass.
func (w *Worker) LastError() string {
	return ""
}

// Sync reconciles once: lists notes/ and hubs/, builds each note's DocText
// and hash, applies cache hits directly, embeds misses through the client in
// batches of at most embedBatch, and removes index vectors whose files are
// gone. Ends by saving the cache pruned to the live hashes. On an embed error
// the cache hits are still applied and the cache still saved; the error is
// returned and Backlog counts the notes left unembedded.
func (w *Worker) Sync(ctx context.Context) error {
	return nil
}

// Run syncs once at startup, then again on every Wake and every syncInterval,
// until ctx ends. Sync failures are logged and surfaced via Backlog and
// LastError, never fatal.
func (w *Worker) Run(ctx context.Context) {
}
