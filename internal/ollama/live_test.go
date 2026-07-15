package ollama

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLiveChatToolCall(t *testing.T) {
	base := os.Getenv("MNEMO_OLLAMA_TESTS")
	if base == "" {
		t.Skip("set MNEMO_OLLAMA_TESTS=http://localhost:11434 to run live ollama tests")
	}
	model := os.Getenv("MNEMO_OLLAMA_MODEL")
	if model == "" {
		model = "qwen3.6:35b"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	c := New(base)
	resp, err := c.Chat(ctx, ChatRequest{
		Model: model,
		Messages: []Message{
			{Role: RoleSystem, Content: "You file notes. Always use the provided tools."},
			{Role: RoleUser, Content: "Search the vault for anything about birth plans."},
		},
		Tools: []Tool{NewTool("vault_search", "Search the user's notes vault.", json.RawMessage(`{
			"type": "object",
			"properties": {"query": {"type": "string", "description": "search query"}},
			"required": ["query"]
		}`))},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.Message.ToolCalls) == 0 {
		t.Fatalf("model made no tool calls; content: %q", resp.Message.Content)
	}
	call := resp.Message.ToolCalls[0].Function
	if call.Name != "vault_search" {
		t.Errorf("tool = %q, want vault_search", call.Name)
	}
	if _, ok := call.Arguments["query"]; !ok {
		t.Errorf("tool call missing query argument: %v", call.Arguments)
	}
}

func TestLiveEmbed(t *testing.T) {
	base := os.Getenv("MNEMO_OLLAMA_TESTS")
	if base == "" {
		t.Skip("set MNEMO_OLLAMA_TESTS=http://localhost:11434 to run live ollama tests")
	}
	model := os.Getenv("MNEMO_OLLAMA_EMBED_MODEL")
	if model == "" {
		model = "qwen3-embedding"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c := New(base)
	resp, err := c.Embed(ctx, EmbedRequest{Model: model, Input: []string{"birth plan", "hospital bag"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Embeddings) != 2 || len(resp.Embeddings[0]) == 0 {
		t.Fatalf("unexpected embeddings shape: %d x %d", len(resp.Embeddings), len(resp.Embeddings[0]))
	}
}
