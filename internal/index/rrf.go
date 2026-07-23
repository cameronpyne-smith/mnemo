package index

// FuseRRF merges ranked lists of slugs by reciprocal rank fusion:
// score(d) = sum over lists of 1/(k + rank(d)), with rank starting at 1.
// A doc absent from a list contributes nothing for that list and is kept.
// Scores are ignored by design — only rank order matters. Results are
// descending by fused score, ties broken by slug ascending, truncated to
// limit.
func FuseRRF(lists [][]string, k, limit int) []Scored {
	return nil
}
