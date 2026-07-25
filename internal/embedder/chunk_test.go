package embedder

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func section(heading string, size int) string {
	return heading + "\n" + strings.Repeat("lorem ipsum dolor sit amet ", size/27+1)[:size] + "\n"
}

func TestChunks(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantChunks int
		wantStarts []string
	}{
		{
			name:       "small note is a single untouched chunk",
			text:       "description\n\n# Heading\n\nbody",
			wantChunks: 1,
		},
		{
			name:       "exactly at the cap stays whole",
			text:       strings.Repeat("a", maxEmbedChars),
			wantChunks: 1,
		},
		{
			name:       "oversized note splits at heading boundaries",
			text:       section("# One", 20_000) + section("## Two", 20_000) + section("## Three", 20_000),
			wantChunks: 3,
			wantStarts: []string{"# One", "## Two", "## Three"},
		},
		{
			name:       "small sections pack together up to the cap",
			text:       section("# A", 10_000) + section("# B", 10_000) + section("# C", 10_000) + section("# D", 10_000),
			wantChunks: 2,
			wantStarts: []string{"# A", "# D"},
		},
		{
			name:       "headingless text falls back to paragraph splits",
			text:       strings.Repeat(strings.Repeat("word ", 400)+"\n\n", 20),
			wantChunks: 2,
		},
		{
			name:       "one giant paragraph hard-splits",
			text:       strings.Repeat("x", 3*maxEmbedChars),
			wantChunks: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Chunks(tt.text)
			if len(got) != tt.wantChunks {
				t.Fatalf("got %d chunks, want %d (sizes: %v)", len(got), tt.wantChunks, chunkSizes(got))
			}
			if joined := strings.Join(got, ""); joined != tt.text {
				t.Error("chunks do not concatenate back to the input")
			}
			for i, c := range got {
				if len(c) > maxEmbedChars {
					t.Errorf("chunk %d is %d bytes, over the cap", i, len(c))
				}
			}
			for i, prefix := range tt.wantStarts {
				if !strings.HasPrefix(got[i], prefix) {
					t.Errorf("chunk %d starts %q, want prefix %q", i, got[i][:20], prefix)
				}
			}
		})
	}
}

func TestChunksKeepRunesIntact(t *testing.T) {
	text := strings.Repeat("€", maxEmbedChars)
	for i, c := range Chunks(text) {
		if !utf8.ValidString(c) {
			t.Errorf("chunk %d split inside a rune", i)
		}
	}
}

func TestChunksIgnoreFakeHeadings(t *testing.T) {
	text := strings.Repeat("#nohashtag heading here padding text\n", 2000)
	got := Chunks(text)
	for i, c := range got[:len(got)-1] {
		if len(c) < maxEmbedChars/2 {
			t.Errorf("chunk %d suspiciously small (%d bytes): split on a non-heading?", i, len(c))
		}
	}
}

func chunkSizes(chunks []string) []int {
	sizes := make([]int, len(chunks))
	for i, c := range chunks {
		sizes[i] = len(c)
	}
	return sizes
}
