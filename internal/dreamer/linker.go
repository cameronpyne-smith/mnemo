package dreamer

import (
	"context"
	"fmt"
	"sort"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/store"
)

type Linker struct {
	Store *store.Store
}

func NewLinker(st *store.Store) *Linker {
	return &Linker{Store: st}
}

func (l *Linker) Name() string { return "linker" }

func (l *Linker) Run(ctx context.Context, budget int) ([]string, error) {
	notes, err := l.Store.ListNotes()
	if err != nil {
		return nil, fmt.Errorf("linker: listing notes: %w", err)
	}
	similarNotes := make(map[string][]index.Hit)
	linked := make(map[[2]string]bool)
	for _, note := range notes {
		for _, target := range note.Links() {
			linked[pairKey(note.Slug, target)] = true
		}
		similar, err := l.Store.Similar(note.Slug, 5)
		if err != nil {
			return nil, fmt.Errorf("linker: similar for %s: %w", note.Slug, err)
		}
		similarNotes[note.Slug] = similar
	}
	return selectCandidates(similarNotes, linked, budget), nil
}

type candidate struct {
	a, b  string
	score float64
}

// selectCandidates filters similar-note hits down to the budget best unlinked
// pairs, deduplicating the symmetric case where each note found the other.
// It mutates linked, treating "already proposed" the same as "already linked".
func selectCandidates(similarNotes map[string][]index.Hit, linked map[[2]string]bool, budget int) []string {
	var candidates []candidate
	for source, hits := range similarNotes {
		for _, hit := range hits {
			key := pairKey(source, hit.Slug)
			if linked[key] {
				continue
			}
			linked[key] = true
			candidates = append(candidates, candidate{a: key[0], b: key[1], score: hit.Score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > budget {
		candidates = candidates[:budget]
	}

	actions := make([]string, 0, len(candidates))
	for _, c := range candidates {
		actions = append(actions, fmt.Sprintf("candidate: [[%s]] ↔ [[%s]] (%.2f)", c.a, c.b, c.score))
	}
	return actions
}

func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}
