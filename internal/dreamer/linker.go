package dreamer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type LLM interface {
	Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error)
}

type Linker struct {
	Store *store.Store
	LLM   LLM
	Model string
}

type verdict struct {
	Link   bool   `json:"link"`
	Reason string `json:"reason"`
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
// note↔note pairs, deduplicating the symmetric case where each note found the
// other. Hub hits are skipped: hub membership is the filing agent's job, not
// the linker's. It mutates linked, treating "already proposed" the same as
// "already linked".
func selectCandidates(similarNotes map[string][]index.Hit, linked map[[2]string]bool, budget int) []candidate {
	var candidates []candidate
	for source, hits := range similarNotes {
		for _, hit := range hits {
			if hit.Folder != vault.FolderNotes {
				continue
			}
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

const judgePrompt = `You judge proposed connections between notes in a personal knowledge vault. Wikilinks like [[some-slug]] are followed by an LLM assistant lazily deciding what else to read, so a link is only worth adding when reading one note would genuinely change what that reader does with the other. Judge whether the two notes below should be connected. Be very selective: the default answer is no. The question is not "are these notes related" — every pair you see was pre-selected as similar — but whether a reader of one note would really benefit from being pointed to the other. Give at most one sentence of reasoning.`

// judgeFormat is enforced by ollama structured outputs: the reply is
// guaranteed to unmarshal into verdict, so a parse failure means the
// model server misbehaved and the pair is skipped, never linked.
var judgeFormat = json.RawMessage(`{"type":"object","properties":{"link":{"type":"boolean"},"reason":{"type":"string"}},"required":["link","reason"]}`)

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
			Model:  l.Model,
			Format: judgeFormat,
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
		v, err := parseVerdict(resp.Message.Content)
		if err != nil {
			actions = append(actions, fmt.Sprintf("skip [[%s]] ↔ [[%s]] (%.2f): unparseable verdict: %s", c.a, c.b, c.score, resp.Message.Content))
			continue
		}
		outcome := "skip"
		if v.Link {
			outcome = "link"
		}
		actions = append(actions, fmt.Sprintf("%s [[%s]] ↔ [[%s]] (%.2f): %s", outcome, c.a, c.b, c.score, v.Reason))
	}
	return actions, nil
}

func parseVerdict(text string) (verdict, error) {
	var v verdict
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return verdict{}, err
	}
	return v, nil
}
