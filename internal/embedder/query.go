package embedder

import (
	"context"
	"fmt"
)

// QueryEmbedder builds search-query vectors: the one code path that applies
// the instruction template before embedding, mirroring how Sync is the one
// code path that embeds documents raw. Mixed conventions fail silently —
// miscalibrated cosines, no error — so both sides stay funneled.
type QueryEmbedder struct {
	client      EmbedClient
	instruction string
}

func NewQueryEmbedder(client EmbedClient, instruction string) *QueryEmbedder {
	return &QueryEmbedder{client: client, instruction: instruction}
}

func (q *QueryEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	vecs, err := q.client.Embed(ctx, []string{QueryText(q.instruction, query)})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embedding query: got %d vectors for one input", len(vecs))
	}
	return vecs[0], nil
}
