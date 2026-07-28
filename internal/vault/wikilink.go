package vault

import (
	"regexp"
	"strings"
)

var (
	wikilinkRe   = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	inlineCodeRe = regexp.MustCompile("`[^`\n]*`")
)

// ExtractWikilinks ignores fenced code blocks and inline code spans, so
// [[...]] inside code (e.g. bash conditionals) is not mistaken for a link.
func ExtractWikilinks(body string) []string {
	var links []string
	matches := wikilinkRe.FindAllStringSubmatch(stripCode(body), -1)
	seen := make(map[string]bool, len(matches))
	for _, match := range matches {
		link := match[1]
		if i := strings.IndexAny(link, "|#"); i != -1 {
			link = link[:i]
		}
		link = strings.TrimSpace(link)
		if link == "" || seen[link] {
			continue
		}
		links = append(links, link)
		seen[link] = true
	}
	return links
}

func stripCode(body string) string {
	var b strings.Builder
	inFence := false
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if inFence {
			if strings.HasPrefix(trimmed, fence) {
				inFence = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			fence = trimmed[:3]
			continue
		}
		b.WriteString(inlineCodeRe.ReplaceAllString(line, ""))
		b.WriteByte('\n')
	}
	return b.String()
}
