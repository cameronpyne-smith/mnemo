package embedder

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TextHash keys the cache by the sha256 of the exact text that was embedded,
// so any change to the embedding recipe re-embeds affected notes automatically.
func TextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Cache is the one sanctioned derived file: embeddings keyed by text hash,
// disposable and rebuilt from markdown at any time. Opening tolerates a
// missing, corrupt, or wrong-model file by starting empty — a bad cache must
// never fail the daemon, it just costs a re-embed.
type Cache struct {
	mu      sync.Mutex
	path    string
	model   string
	dims    int
	entries map[string][]float32
}

type cacheFile struct {
	Model   string
	Dims    int
	Entries map[string][]float32
}

func OpenCache(path, model string) *Cache {
	c := &Cache{path: path, model: model, entries: make(map[string][]float32)}
	f, err := os.Open(path)
	if err != nil {
		return c
	}
	defer f.Close()
	var file cacheFile
	if err := gob.NewDecoder(f).Decode(&file); err != nil || file.Model != model || file.Entries == nil {
		return c
	}
	c.dims = file.Dims
	c.entries = file.Entries
	return c
}

func (c *Cache) Get(hash string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	vec, ok := c.entries[hash]
	return vec, ok
}

func (c *Cache) Put(hash string, vec []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dims == 0 {
		c.dims = len(vec)
	}
	c.entries[hash] = vec
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *Cache) Dims() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dims
}

// Save writes the cache atomically, keeping only entries whose hash is in
// live — stale vectors for edited or deleted notes are dropped here rather
// than tracked incrementally.
func (c *Cache) Save(live map[string]bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for hash := range c.entries {
		if !live[hash] {
			delete(c.entries, hash)
		}
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("saving embedding cache: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), ".mnemo-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("saving embedding cache: %w", err)
	}
	defer os.Remove(tmp.Name())
	err = gob.NewEncoder(tmp).Encode(cacheFile{Model: c.model, Dims: c.dims, Entries: c.entries})
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("saving embedding cache: %w", err)
	}
	if err := os.Rename(tmp.Name(), c.path); err != nil {
		return fmt.Errorf("saving embedding cache: %w", err)
	}
	return nil
}
