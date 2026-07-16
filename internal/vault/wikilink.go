package vault

import (
	"regexp"
	"strings"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

func ExtractWikilinks(body string) []string {
	var links []string
	matches := wikilinkRe.FindAllStringSubmatch(body, -1)
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
