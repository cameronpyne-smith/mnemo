package dreamer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cameronpyne-smith/mnemo/internal/index"
	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

// minCandidateScore drops pairs the embedding space already rules out:
// in observed runs everything below ~0.3 was judged skip, so such pairs
// are not worth a model call.
const minCandidateScore = 0.3

type LLM interface {
	Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error)
}

type Linker struct {
	Store *store.Store
	LLM   LLM
	Model string

	// judged remembers pairs already put to the model this daemon run, so
	// skip verdicts are not re-judged every cycle. In-memory on purpose: a
	// restart re-judges, which lets stale skips heal as notes grow.
	judged map[[2]string]bool
}

type verdict struct {
	Link   bool   `json:"link"`
	Reason string `json:"reason"`
	Into   string `json:"into"`
}

func NewLinker(st *store.Store, llm LLM, model string) *Linker {
	return &Linker{Store: st, LLM: llm, Model: model, judged: make(map[[2]string]bool)}
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
	for pair := range l.judged {
		linked[pair] = true
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
			if hit.Folder != vault.FolderNotes || hit.Score < minCandidateScore {
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

const judgePrompt = `You judge proposed connections between notes in a personal knowledge vault. Wikilinks like [[some-slug]] are followed by an LLM assistant lazily deciding what else to read, so a link is only worth adding when reading one note would genuinely change what that reader does with the other. Judge whether the two notes below should be connected. Be very selective: the default answer is no. The question is not "are these notes related" — every pair you see was pre-selected as similar — but whether a reader of one note would really benefit from being pointed to the other. Give at most one sentence of reasoning; do not restate the verdict in the reason. Set into to the slug of the note whose reader benefits most from the pointer.`

// judgeFormat is enforced by ollama structured outputs: the reply is
// guaranteed to unmarshal into verdict, so a parse failure means the
// model server misbehaved and the pair is skipped, never linked. The
// enum on into means the model cannot place a link anywhere but in the
// judged pair.
func judgeFormat(a, b string) json.RawMessage {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"link":   map[string]any{"type": "boolean"},
			"reason": map[string]any{"type": "string"},
			"into":   map[string]any{"type": "string", "enum": []string{a, b}},
		},
		"required": []string{"link", "reason", "into"},
	})
	return schema
}

func (l *Linker) judgeCandidates(ctx context.Context, candidates []candidate) ([]string, error) {
	actions := make([]string, 0, len(candidates))
	for _, c := range candidates {
		key := pairKey(c.a, c.b)
		if l.judged[key] {
			continue
		}
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
			Format: judgeFormat(c.a, c.b),
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
		reason := strings.Join(strings.Fields(v.Reason), " ")
		if !v.Link {
			l.judged[key] = true
			actions = append(actions, fmt.Sprintf("skip [[%s]] ↔ [[%s]] (%.2f): %s", c.a, c.b, c.score, reason))
			continue
		}
		into, other := c.a, c.b
		if v.Into == c.b {
			into, other = c.b, c.a
		}
		if err := l.Store.Link(store.ActorDreamer, into, other, reason); err != nil {
			return actions, fmt.Errorf("linker: linking [[%s]] -> [[%s]]: %w", into, other, err)
		}
		l.judged[key] = true
		actions = append(actions, fmt.Sprintf("linked [[%s]] → [[%s]] (%.2f): %s", into, other, c.score, reason))
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
