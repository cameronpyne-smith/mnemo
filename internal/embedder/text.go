package embedder

// QueryText builds the exact string embedded for a search query — the single
// code path that applies the instruction template, so prefixed and raw
// embeddings can never mix in one space. Empty instruction means no wrapping
// (for models that are not instruction-aware). Template per Qwen3-Embedding:
// "Instruct: {instruction}\nQuery:{query}" with no space after "Query:".
func QueryText(instruction, query string) string {
	return ""
}

// DocText builds the exact string embedded for a note. Documents are embedded
// raw — never instruction-wrapped. Non-empty parts joined by a blank line.
func DocText(description, body string) string {
	return ""
}
