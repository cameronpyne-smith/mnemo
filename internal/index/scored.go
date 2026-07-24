package index

import (
	"slices"
	"strings"
)

type Scored struct {
	Slug  string
	Score float64
}

func orderByScoreDescending(scored []Scored, limit int) []Scored {
	slices.SortFunc(scored, func(a, b Scored) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		default:
			return strings.Compare(a.Slug, b.Slug)
		}
	})

	if len(scored) > limit {
		return scored[:limit]
	}
	return scored
}
