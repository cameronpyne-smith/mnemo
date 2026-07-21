package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/store"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

type scriptedLLM struct {
	turns []ollama.Message
	calls int
}

func (s *scriptedLLM) Chat(ctx context.Context, req ollama.ChatRequest) (*ollama.ChatResponse, error) {
	msg := s.turns[s.calls]
	s.calls++
	return &ollama.ChatResponse{Message: msg, Done: true}, nil
}

func call(name string, args map[string]any) ollama.ToolCall {
	return ollama.ToolCall{Function: ollama.ToolCallFunction{Name: name, Arguments: args}}
}

func testFiler(t *testing.T, llm LLM) (*Filer, *store.Store) {
	t.Helper()
	root := t.TempDir()
	if _, err := vault.Init(root); err != nil {
		t.Fatalf("vault.Init: %v", err)
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return &Filer{Store: st, LLM: llm, Model: "fake"}, st
}

func TestFileHappyPath(t *testing.T) {
	llm := &scriptedLLM{turns: []ollama.Message{
		{Role: ollama.RoleAssistant, ToolCalls: []ollama.ToolCall{
			call("search_notes", map[string]any{"query": "protein powder"}),
		}},
		{Role: ollama.RoleAssistant, ToolCalls: []ollama.ToolCall{
			call("write_note", map[string]any{
				"slug":        "shopping-list",
				"description": "Things to buy.",
				"tags":        []any{"shopping"},
				"body":        "- Vanilla protein powder (MyProtein)\n",
			}),
			call("add_to_hub", map[string]any{
				"hub": "household", "note": "shopping-list", "description": "things to buy",
			}),
		}},
		{Role: ollama.RoleAssistant, ToolCalls: []ollama.ToolCall{
			call("finish", map[string]any{"notes": []any{"shopping-list"}, "summary": "Filed shopping item."}),
		}},
	}}
	f, st := testFiler(t, llm)

	slug, err := st.Capture("test", "Buy vanilla protein powder from MyProtein", "test")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	result, err := f.File(context.Background(), slug)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if len(result.FiledInto) != 1 || result.FiledInto[0] != "shopping-list" {
		t.Errorf("FiledInto = %v", result.FiledInto)
	}

	view, err := st.Get("shopping-list")
	if err != nil {
		t.Fatalf("note not written: %v", err)
	}
	if !strings.Contains(view.Note.Body, "protein powder") {
		t.Errorf("body = %q", view.Note.Body)
	}
	if len(view.Backlinks) != 1 || view.Backlinks[0] != "household" {
		t.Errorf("backlinks = %v, want [household]", view.Backlinks)
	}

	inbox, err := st.ListInbox()
	if err != nil || len(inbox) != 0 {
		t.Errorf("inbox not drained: %v %v", inbox, err)
	}
	archived, err := st.Get(slug)
	if err != nil || archived.Note.Folder != vault.FolderArchive {
		t.Errorf("capture not archived: %v %v", archived, err)
	}
}

func TestFileToolErrorsAreReported(t *testing.T) {
	llm := &scriptedLLM{turns: []ollama.Message{
		{Role: ollama.RoleAssistant, ToolCalls: []ollama.ToolCall{
			call("write_note", map[string]any{"slug": "Bad Slug", "description": "d", "body": "b"}),
		}},
		{Role: ollama.RoleAssistant, ToolCalls: []ollama.ToolCall{
			call("write_note", map[string]any{"slug": "good-slug", "description": "d.", "body": "b\n"}),
			call("finish", map[string]any{"notes": []any{"good-slug"}, "summary": "done"}),
		}},
	}}
	f, st := testFiler(t, llm)

	slug, err := st.Capture("test", "something", "test")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, err := f.File(context.Background(), slug); err != nil {
		t.Fatalf("File: %v", err)
	}
	if _, err := st.Get("good-slug"); err != nil {
		t.Errorf("recovery write missing: %v", err)
	}
}

func TestFileNudgesThenAborts(t *testing.T) {
	llm := &scriptedLLM{turns: []ollama.Message{
		{Role: ollama.RoleAssistant, Content: "I think this is about shopping."},
		{Role: ollama.RoleAssistant, Content: "Still just chatting."},
	}}
	f, st := testFiler(t, llm)

	slug, err := st.Capture("test", "something", "test")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, err := f.File(context.Background(), slug); err == nil {
		t.Fatal("File succeeded with a model that never used tools")
	}
	inbox, err := st.ListInbox()
	if err != nil || len(inbox) != 1 {
		t.Errorf("capture should remain in inbox on failure: %v %v", inbox, err)
	}
	if llm.calls != 2 {
		t.Errorf("calls = %d, want 2 (initial + nudge)", llm.calls)
	}
}

func TestWriteNoteCannotOverwriteHub(t *testing.T) {
	f, st := testFiler(t, &scriptedLLM{})
	if err := st.AddToHub("test", "health", "x", "d"); err == nil {
		t.Log("hub add failed as expected (note missing), creating hub directly")
	}
	out := f.execute("write_note", map[string]any{"slug": "root", "description": "d", "body": "b"})
	if !strings.Contains(out, "error") {
		t.Errorf("overwriting root hub allowed: %q", out)
	}
}
