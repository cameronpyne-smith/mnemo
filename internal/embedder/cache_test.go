package embedder

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func cachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".mnemo", "embeddings.gob")
}

func TestCacheRoundTrip(t *testing.T) {
	path := cachePath(t)
	c := OpenCache(path, "qwen3-embedding:8b")
	h := TextHash("some note text")
	c.Put(h, []float32{0.1, 0.2, 0.3})

	if err := c.Save(map[string]bool{h: true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reopened := OpenCache(path, "qwen3-embedding:8b")
	vec, ok := reopened.Get(h)
	if !ok || !reflect.DeepEqual(vec, []float32{0.1, 0.2, 0.3}) {
		t.Errorf("Get after reopen = %v, %v", vec, ok)
	}
	if reopened.Dims() != 3 {
		t.Errorf("Dims = %d, want 3", reopened.Dims())
	}
}

func TestCacheModelSwapDiscards(t *testing.T) {
	path := cachePath(t)
	c := OpenCache(path, "old-model")
	h := TextHash("text")
	c.Put(h, []float32{1})
	if err := c.Save(map[string]bool{h: true}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened := OpenCache(path, "new-model")
	if reopened.Len() != 0 {
		t.Errorf("Len after model swap = %d, want 0", reopened.Len())
	}
}

func TestCacheSaveDropsStaleEntries(t *testing.T) {
	path := cachePath(t)
	c := OpenCache(path, "m")
	live, stale := TextHash("live"), TextHash("stale")
	c.Put(live, []float32{1})
	c.Put(stale, []float32{2})

	if err := c.Save(map[string]bool{live: true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reopened := OpenCache(path, "m")
	if _, ok := reopened.Get(stale); ok {
		t.Error("stale entry survived Save")
	}
	if _, ok := reopened.Get(live); !ok {
		t.Error("live entry dropped by Save")
	}
}

func TestCacheToleratesCorruptFile(t *testing.T) {
	path := cachePath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a gob"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := OpenCache(path, "m")
	if c.Len() != 0 {
		t.Errorf("Len from corrupt file = %d, want 0", c.Len())
	}
	h := TextHash("text")
	c.Put(h, []float32{1})
	if err := c.Save(map[string]bool{h: true}); err != nil {
		t.Fatalf("Save over corrupt file: %v", err)
	}
	if _, ok := OpenCache(path, "m").Get(h); !ok {
		t.Error("entry lost after recovering from corrupt file")
	}
}
