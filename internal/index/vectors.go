package index

type DocVector struct {
	Slug string
	Vec  []float32
}

// TopK ranks docs against query by dot product, descending, returning at most
// k results. All vectors must be L2-normalized (ollama's are), which makes dot
// product equal cosine similarity. Ties break by slug ascending so results are
// deterministic.
func TopK(query []float32, docs []DocVector, k int) []Scored {
	if len(query) == 0 {
		return nil
	}
	if k <= 0 {
		return nil
	}

	scored := make([]Scored, len(docs))
	for i, doc := range docs {
		scored[i] = Scored{Slug: doc.Slug, Score: dotProduct(query, doc.Vec)}
	}

	return orderByScoreDescending(scored, k)
}

func dotProduct(a []float32, b []float32) float64 {
	result := 0.0
	for i := range a {
		result += float64(a[i]) * float64(b[i])
	}

	return result
}
