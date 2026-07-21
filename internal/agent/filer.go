package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

const systemPrompt = `You are mnemo's filing agent. You maintain a vault of markdown notes — the user's second brain. Your job: take one raw captured dump and file its information into the vault, then finish.

Vault conventions:
- Notes are identified by kebab-case slugs. Wikilinks look like [[some-slug]].
- Every note has a one-line description used by other agents to decide whether to open it. Write descriptions that say what is IN the note, concretely.
- Hubs are index notes listing related notes. Every filed note must be reachable from a hub.

Process, in order:
1. search_notes with 1-3 queries drawn from the dump to find related existing notes.
2. read_note on the most promising results.
3. Decide: extend an existing note (preferred when the dump clearly belongs to it) or create a new one. Never fragment one topic across many small notes.
4. write_note with the full updated content. Rewrite the dump into clear prose or lists — do not paste it raw. Preserve every fact, date, name, and number; add nothing you were not told. Add [[wikilinks]] to related notes you found in step 1-2.
5. add_to_hub so the note is reachable (skip only if the note was already in a hub). Reuse an existing hub when one fits; create a new one only for a genuinely new area of life.
6. finish with the slugs you filed into and a one-line summary.

Be conservative: when unsure whether two things are the same topic, keep them separate and link them. Never delete or contradict existing information; append or reorganise instead.`

type LLM interface {
	Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error)
}

type Filer struct {
	Store    *store.Store
	LLM      LLM
	Model    string
	MaxTurns int
	Log      *slog.Logger
}

type Result struct {
	FiledInto []string
	Summary   string
	Turns     int
}

func (f *Filer) File(ctx context.Context, captureSlug string) (*Result, error) {
	view, err := f.Store.Get(captureSlug)
	if err != nil {
		return nil, fmt.Errorf("filing %s: %w", captureSlug, err)
	}
	source := ""
	if s, ok := view.Note.Frontmatter.Extra["source"].(string); ok && s != "" {
		source = fmt.Sprintf(" (source: %s)", s)
	}

	maxTurns := f.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 12
	}
	messages := []ollama.Message{
		{Role: ollama.RoleSystem, Content: systemPrompt},
		{Role: ollama.RoleUser, Content: fmt.Sprintf("New capture%s, taken %s:\n\n%s", source, view.Note.Frontmatter.Created, view.Note.Body)},
	}

	nudged := false
	for turn := 1; turn <= maxTurns; turn++ {
		resp, err := f.LLM.Chat(ctx, ollama.ChatRequest{
			Model:    f.Model,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return nil, fmt.Errorf("filing %s (turn %d): %w", captureSlug, turn, err)
		}
		messages = append(messages, resp.Message)

		if len(resp.Message.ToolCalls) == 0 {
			if nudged {
				return nil, fmt.Errorf("filing %s: model stopped calling tools without finishing", captureSlug)
			}
			nudged = true
			messages = append(messages, ollama.Message{
				Role:    ollama.RoleUser,
				Content: "Continue using the tools. When the capture is fully filed, call finish.",
			})
			continue
		}

		for _, call := range resp.Message.ToolCalls {
			if call.Function.Name == "finish" {
				result := parseFinish(call.Function.Arguments)
				result.Turns = turn
				if err := f.Store.ArchiveCapture(store.ActorFiling, captureSlug, result.FiledInto); err != nil {
					return nil, fmt.Errorf("filing %s: archiving capture: %w", captureSlug, err)
				}
				return result, nil
			}
			output := f.execute(call.Function.Name, call.Function.Arguments)
			if f.Log != nil {
				f.Log.Debug("tool call", "capture", captureSlug, "tool", call.Function.Name, "args", call.Function.Arguments)
			}
			messages = append(messages, ollama.Message{
				Role:     ollama.RoleTool,
				ToolName: call.Function.Name,
				Content:  output,
			})
		}
	}
	return nil, fmt.Errorf("filing %s: no finish after %d turns", captureSlug, maxTurns)
}

func parseFinish(args map[string]any) *Result {
	r := &Result{}
	if raw, ok := args["notes"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				r.FiledInto = append(r.FiledInto, s)
			}
		}
	}
	if s, ok := args["summary"].(string); ok {
		r.Summary = s
	}
	return r
}

func (f *Filer) execute(name string, args map[string]any) string {
	out, err := f.executeErr(name, args)
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

func (f *Filer) executeErr(name string, args map[string]any) (string, error) {
	switch name {
	case "search_notes":
		q, _ := args["query"].(string)
		if strings.TrimSpace(q) == "" {
			return "", fmt.Errorf("query is required")
		}
		hits, err := f.Store.Search(q, 8)
		if err != nil {
			return "", err
		}
		if len(hits) == 0 {
			return "no matches", nil
		}
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "[[%s]] (%s) — %s\n", h.Slug, h.Folder, h.Description)
		}
		return b.String(), nil

	case "read_note":
		slug, _ := args["slug"].(string)
		view, err := f.Store.Get(slug)
		if err != nil {
			return "", err
		}
		content, err := json.Marshal(map[string]any{
			"slug":        view.Note.Slug,
			"description": view.Note.Frontmatter.Description,
			"tags":        view.Note.Frontmatter.Tags,
			"body":        view.Note.Body,
			"backlinks":   view.Backlinks,
		})
		if err != nil {
			return "", err
		}
		return string(content), nil

	case "write_note":
		slug, _ := args["slug"].(string)
		description, _ := args["description"].(string)
		body, _ := args["body"].(string)
		if !vault.ValidSlug(slug) {
			return "", fmt.Errorf("invalid slug %q: use kebab-case", slug)
		}
		if strings.TrimSpace(description) == "" || strings.TrimSpace(body) == "" {
			return "", fmt.Errorf("description and body are both required")
		}
		var tags []string
		if raw, ok := args["tags"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					tags = append(tags, s)
				}
			}
		}
		n := &vault.Note{Slug: slug, Folder: vault.FolderNotes}
		if existing, err := f.Store.Get(slug); err == nil {
			if existing.Note.Folder != vault.FolderNotes {
				return "", fmt.Errorf("%s is a %s note and cannot be overwritten", slug, existing.Note.Folder)
			}
			n = existing.Note
		}
		n.Frontmatter.Description = description
		n.Frontmatter.Tags = tags
		n.Body = body
		if err := f.Store.Save(store.ActorFiling, n); err != nil {
			return "", err
		}
		return fmt.Sprintf("wrote [[%s]]", slug), nil

	case "add_to_hub":
		hub, _ := args["hub"].(string)
		note, _ := args["note"].(string)
		description, _ := args["description"].(string)
		if _, err := f.Store.Get(note); err != nil {
			return "", fmt.Errorf("note %s does not exist; write it first", note)
		}
		if err := f.Store.AddToHub(store.ActorFiling, hub, note, description); err != nil {
			return "", err
		}
		return fmt.Sprintf("added [[%s]] to hub [[%s]]", note, hub), nil

	default:
		return "", fmt.Errorf("unknown tool %s", name)
	}
}
