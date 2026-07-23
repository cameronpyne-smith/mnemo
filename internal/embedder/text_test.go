package embedder

import "testing"

func TestQueryText(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		query       string
		want        string
	}{
		{
			name:        "wraps with template, no space after Query:",
			instruction: "Given a web search query, retrieve relevant passages that answer the query",
			query:       "how do I feed the sourdough starter",
			want:        "Instruct: Given a web search query, retrieve relevant passages that answer the query\nQuery:how do I feed the sourdough starter",
		},
		{
			name:        "empty instruction leaves query raw",
			instruction: "",
			query:       "how do I feed the sourdough starter",
			want:        "how do I feed the sourdough starter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QueryText(tt.instruction, tt.query); got != tt.want {
				t.Errorf("QueryText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDocText(t *testing.T) {
	tests := []struct {
		name        string
		description string
		body        string
		want        string
	}{
		{
			name:        "description then body",
			description: "Feeding schedule for the sourdough starter.",
			body:        "Feed daily at 8am.\n",
			want:        "Feeding schedule for the sourdough starter.\n\nFeed daily at 8am.\n",
		},
		{
			name:        "no description",
			description: "",
			body:        "Feed daily at 8am.\n",
			want:        "Feed daily at 8am.\n",
		},
		{
			name:        "no body",
			description: "Feeding schedule for the sourdough starter.",
			body:        "",
			want:        "Feeding schedule for the sourdough starter.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DocText(tt.description, tt.body); got != tt.want {
				t.Errorf("DocText = %q, want %q", got, tt.want)
			}
		})
	}
}
