package index

const (
	hybridPool = 50
	rrfK       = 60
)

// SearchHybrid fuses full-text and vector rankings with reciprocal rank
// fusion: the top hybridPool candidates from each side, k=rrfK, truncated to
// limit. Hit scores are RRF scores, comparable only within one result set. A
// note found by only one side keeps that side's contribution alone.
func (idx *Index) SearchHybrid(q string, queryVec []float32, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 20
	}
	ftsHits, err := idx.Search(q, hybridPool)
	if err != nil {
		return nil, err
	}
	ftsSlugs := make([]string, len(ftsHits))
	for i, h := range ftsHits {
		ftsSlugs[i] = h.Slug
	}
	fused := FuseRRF([][]string{ftsSlugs, idx.vectorRank(queryVec, hybridPool)}, rrfK, limit)
	return idx.hits(fused), nil
}

// vectorRank returns up to limit slugs by cosine against queryVec, best
// first; a note ranks by its best chunk.
func (idx *Index) vectorRank(queryVec []float32, limit int) []string {
	docs := idx.Vectors()
	seen := make(map[string]bool)
	slugs := make([]string, 0, limit)
	for _, sc := range TopK(queryVec, docs, len(docs)) {
		if seen[sc.Slug] {
			continue
		}
		seen[sc.Slug] = true
		slugs = append(slugs, sc.Slug)
		if len(slugs) == limit {
			break
		}
	}
	return slugs
}

// Similar ranks every other embedded note against slug's vectors by cosine,
// scoring each note pair by its best chunk pair. ok is false when slug has no
// vectors — not embedded yet, or not a note.
func (idx *Index) Similar(slug string, limit int) (hits []Hit, ok bool) {
	idx.mu.RLock()
	queryVecs := idx.vectors[slug]
	idx.mu.RUnlock()
	if len(queryVecs) == 0 {
		return nil, false
	}
	if limit <= 0 {
		limit = 10
	}
	docs := idx.Vectors()
	best := make(map[string]float64)
	for _, qv := range queryVecs {
		for _, sc := range TopK(qv, docs, len(docs)) {
			if sc.Slug == slug {
				continue
			}
			if cur, found := best[sc.Slug]; !found || sc.Score > cur {
				best[sc.Slug] = sc.Score
			}
		}
	}
	ranked := make([]Scored, 0, len(best))
	for s, score := range best {
		ranked = append(ranked, Scored{Slug: s, Score: score})
	}
	return idx.hits(orderByScoreDescending(ranked, limit)), true
}

// hits resolves scored slugs to Hits with folder and description; a slug
// removed since scoring resolves to empty fields rather than an error.
func (idx *Index) hits(scored []Scored) []Hit {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]Hit, 0, len(scored))
	for _, sc := range scored {
		out = append(out, Hit{
			Slug:        sc.Slug,
			Folder:      idx.folders[sc.Slug],
			Description: idx.descriptions[sc.Slug],
			Score:       sc.Score,
		})
	}
	return out
}
