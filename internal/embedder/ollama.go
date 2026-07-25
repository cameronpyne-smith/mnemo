package embedder

import (
	"context"
	"fmt"

	"github.com/cameronpyne-smith/mnemo/internal/ollama"
)

// Ollama adapts the ollama client to EmbedClient, pinning the model so every
// vector in one index comes from the same embedding space. The response is
// validated against the input length because callers pair vectors with texts
// by position.
type Ollama struct {
	client *ollama.Client
	model  string
}

func NewOllama(client *ollama.Client, model string) *Ollama {
	return &Ollama{client: client, model: model}
}

func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := o.client.Embed(ctx, ollama.EmbedRequest{Model: o.model, Input: texts})
	if err != nil {
		return nil, err
	}
	if len(resp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d inputs", len(resp.Embeddings), len(texts))
	}
	return resp.Embeddings, nil
}
