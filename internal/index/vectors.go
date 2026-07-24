package index

type DocVector struct {
	Slug string
	Vec  []float32
}

// TopK ranks docs against query by dot product, descending, returning at most
// k results. All vectors must be L2-normalized (ollama's are), which makes dot
// product equal cosine similarity. Ties break by slug ascending so results are
// deterministic. Docs whose vector dimension differs from the query's are
// skipped: stale vectors from a previous embedding model must degrade to
// FTS-only coverage, not panic or pollute rankings.
func TopK(query []float32, docs []DocVector, k int) []Scored {
	if len(query) == 0 || k <= 0 {
		return nil
	}

	scored := make([]Scored, 0, len(docs))
	for _, doc := range docs {
		if len(doc.Vec) != len(query) {
			continue
		}
		scored = append(scored, Scored{Slug: doc.Slug, Score: dotProduct(query, doc.Vec)})
	}

	return orderByScoreDescending(scored, k)
}

// dotProduct requires len(a) == len(b): a mismatch would panic or silently
// mis-score, so callers must filter first, as TopK does.
func dotProduct(a []float32, b []float32) float64 {
	result := 0.0
	for i := range a {
		result += float64(a[i]) * float64(b[i])
	}

	return result
}
