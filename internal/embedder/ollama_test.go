package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/cameronpyne-smith/mnemo/internal/ollama"
)

func TestOllamaAdapter(t *testing.T) {
	var got struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	short := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %s, want /api/embed", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		embeddings := [][]float32{{1, 0}, {0, 1}}
		if short {
			embeddings = embeddings[:1]
		}
		json.NewEncoder(w).Encode(map[string]any{"model": got.Model, "embeddings": embeddings})
	}))
	defer srv.Close()

	client := NewOllama(ollama.New(srv.URL), "test-model")
	vecs, err := client.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got.Model != "test-model" || !reflect.DeepEqual(got.Input, []string{"a", "b"}) {
		t.Errorf("request = %+v, want model test-model and both inputs", got)
	}
	if !reflect.DeepEqual(vecs, [][]float32{{1, 0}, {0, 1}}) {
		t.Errorf("vecs = %v", vecs)
	}

	short = true
	if _, err := client.Embed(context.Background(), []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "1 vectors for 2 inputs") {
		t.Errorf("short response err = %v, want length mismatch", err)
	}
}
