package dreamer

import (
	"context"
	"fmt"
	"sort"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
)

type LLM interface {
	Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error)
}

type Linker struct {
	Store *store.Store
	LLM   LLM
	Model string
}

func NewLinker(st *store.Store, llm LLM, model string) *Linker {
	return &Linker{Store: st, LLM: llm, Model: model}
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
	return l.judgeCandidates(ctx, selectCandidates(similarNotes, linked, budget))
}

type candidate struct {
	a, b  string
	score float64
}

// selectCandidates filters similar-note hits down to the budget best unlinked
// pairs, deduplicating the symmetric case where each note found the other.
// It mutates linked, treating "already proposed" the same as "already linked".
func selectCandidates(similarNotes map[string][]index.Hit, linked map[[2]string]bool, budget int) []candidate {
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
	return candidates
}

func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

const judgePrompt = `Judge if the following two notes should contain links to each other. Be very selective when saying yes. The question is not "are these notes related" — the question is whether a reader of one note would really benefit from being pointed to the other.`

func (l *Linker) judgeCandidates(ctx context.Context, candidates []candidate) ([]string, error) {
	actions := make([]string, 0, len(candidates))
	for _, c := range candidates {
		av, err := l.Store.Get(c.a)
		if err != nil {
			return actions, fmt.Errorf("linker: reading %s: %w", c.a, err)
		}
		bv, err := l.Store.Get(c.b)
		if err != nil {
			return actions, fmt.Errorf("linker: reading %s: %w", c.b, err)
		}
		resp, err := l.LLM.Chat(ctx, ollama.ChatRequest{
			Model: l.Model,
			Messages: []ollama.Message{
				{Role: ollama.RoleSystem, Content: judgePrompt},
				{Role: ollama.RoleUser, Content: fmt.Sprintf(
					"Note [[%s]] — %s\n\n%s\n\n---\n\nNote [[%s]] — %s\n\n%s",
					c.a, av.Note.Frontmatter.Description, av.Note.Body,
					c.b, bv.Note.Frontmatter.Description, bv.Note.Body,
				)},
			},
		})
		if err != nil {
			return actions, fmt.Errorf("linker: judging [[%s]] ↔ [[%s]]: %w", c.a, c.b, err)
		}
		actions = append(actions, fmt.Sprintf("judged [[%s]] ↔ [[%s]] (%.2f): %s", c.a, c.b, c.score, resp.Message.Content))
	}
	return actions, nil
}
