package vault

import (
	"regexp"
	"strings"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

func ExtractWikilinks(body string) []string {
	matches := wikilinkRe.FindAllStringSubmatch(body, -1)
	if matches == nil {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var targets []string
	for _, m := range matches {
		target := m[1]
		if i := strings.IndexAny(target, "|#"); i >= 0 {
			target = target[:i]
		}
		target = strings.TrimSpace(target)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
	}
	return targets
}
