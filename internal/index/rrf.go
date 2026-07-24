package index

import (
	"slices"
	"strings"
)

// FuseRRF merges ranked lists of slugs by reciprocal rank fusion:
// score(d) = sum over lists of 1/(k + rank(d)), with rank starting at 1.
// A doc absent from a list contributes nothing for that list and is kept.
// Scores are ignored by design — only rank order matters. Results are
// descending by fused score, ties broken by slug ascending, truncated to
// limit.
func FuseRRF(lists [][]string, k, limit int) []Scored {
	scores := make(map[string]float64)
	for _, list := range lists {
		for j, item := range list {
			scores[item] += (1.0 / float64(k+j+1))
		}
	}

	ranked := make([]Scored, 0, len(scores))
	for slug, score := range scores {
		ranked = append(ranked, Scored{Slug: slug, Score: score})
	}

	slices.SortFunc(ranked, func(a, b Scored) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		default:
			return strings.Compare(a.Slug, b.Slug)
		}
	})

	if len(ranked) > limit {
		return ranked[:limit]
	}
	return ranked
}
