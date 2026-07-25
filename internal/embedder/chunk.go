package embedder

import (
	"strings"
	"unicode/utf8"
)

// maxEmbedChars caps one embedding input at roughly 8k tokens. Measured in
// bytes, not runes — a deliberate underestimate for multibyte text, keeping
// the daemon free of a tokenizer dependency.
const maxEmbedChars = 32_000

// Chunks splits text for embedding. Almost every note fits the cap and comes
// back as a single chunk. Oversized text splits at heading lines, then blank
// lines, then (for one monstrous paragraph) rune boundaries, packed greedily
// so chunks stay few and large. The chunks always concatenate back to the
// exact input, so per-chunk hashes cover every byte of the note. Results stay
// note-level: callers score a note by its best chunk.
func Chunks(text string) []string {
	if len(text) <= maxEmbedChars {
		return []string{text}
	}
	var pieces []string
	for _, section := range splitAtHeadings(text) {
		if len(section) <= maxEmbedChars {
			pieces = append(pieces, section)
			continue
		}
		for _, para := range strings.SplitAfter(section, "\n\n") {
			if len(para) <= maxEmbedChars {
				pieces = append(pieces, para)
			} else {
				pieces = append(pieces, hardSplit(para)...)
			}
		}
	}
	return pack(pieces)
}

func splitAtHeadings(text string) []string {
	var parts []string
	start, offset := 0, 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if offset > start && isHeading(line) {
			parts = append(parts, text[start:offset])
			start = offset
		}
		offset += len(line)
	}
	return append(parts, text[start:])
}

func isHeading(line string) bool {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return false
	}
	rest := line[level:]
	return rest == "" || rest == "\n" || rest[0] == ' ' || rest[0] == '\t'
}

// hardSplit cuts at the last rune boundary at or before the cap, never inside
// a rune, so every chunk stays valid UTF-8.
func hardSplit(s string) []string {
	var parts []string
	for len(s) > maxEmbedChars {
		cut := maxEmbedChars
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		if cut == 0 {
			cut = maxEmbedChars
		}
		parts = append(parts, s[:cut])
		s = s[cut:]
	}
	return append(parts, s)
}

func pack(pieces []string) []string {
	var chunks []string
	var cur strings.Builder
	for _, piece := range pieces {
		if cur.Len() > 0 && cur.Len()+len(piece) > maxEmbedChars {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		cur.WriteString(piece)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}
