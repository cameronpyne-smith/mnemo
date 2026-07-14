package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s, want /api/chat", r.URL.Path)
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req.Stream {
			t.Error("stream = true, want false")
		}
		if req.Model != "test-model" || len(req.Messages) != 1 || len(req.Tools) != 1 {
			t.Errorf("unexpected request: %+v", req)
		}
		json.NewEncoder(w).Encode(ChatResponse{
			Model: req.Model,
			Done:  true,
			Message: Message{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{{Function: ToolCallFunction{
					Name:      "vault_search",
					Arguments: map[string]any{"query": "birth plan"},
				}}},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: RoleUser, Content: "file this"}},
		Tools:    []Tool{NewTool("vault_search", "Search the vault", json.RawMessage(`{"type":"object"}`))},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0].Function.Name != "vault_search" {
		t.Errorf("tool calls = %+v, want one vault_search call", resp.Message.ToolCalls)
	}
}

func TestEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %s, want /api/embed", r.URL.Path)
		}
		var req EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if len(req.Input) != 2 {
			t.Errorf("input length = %d, want 2", len(req.Input))
		}
		json.NewEncoder(w).Encode(EmbedResponse{
			Model:      req.Model,
			Embeddings: [][]float32{{0.1, 0.2}, {0.3, 0.4}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.Embed(context.Background(), EmbedRequest{Model: "embed-model", Input: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Embeddings) != 2 || len(resp.Embeddings[0]) != 2 {
		t.Errorf("embeddings = %v, want 2x2", resp.Embeddings)
	}
}

func TestAPIErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "model not found"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Chat(context.Background(), ChatRequest{Model: "missing"})
	if err == nil {
		t.Fatal("Chat succeeded on API error")
	}
	if got := err.Error(); !strings.Contains(got, "model not found") {
		t.Errorf("error %q does not contain API message", got)
	}
}
